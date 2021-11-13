package streamer

import (
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

type streamSession struct {
	vc *discordgo.VoiceConnection

	cancel chan bool
	done   chan error

	source     dca.OpusReader
	framesSent int

	streaming bool

	sync.RWMutex
}

func (s *streamSession) stream() {

	s.Lock()
	if s.streaming {
		return
	}
	s.streaming = true
	s.Unlock()

	for {
		select {
		case <-s.cancel:
			s.Lock()
			s.streaming = false
			s.Unlock()
			return
		default:
		}
		err := s.readNext()
		if err != nil {
			go func() {
				s.done <- err
			}()
			// if err != io.EOF {
			// }
			s.Lock()
			s.streaming = false
			s.Unlock()
			break
		}
	}

}

func (s *streamSession) readNext() error {
	opus, err := s.source.OpusFrame()

	if err != nil {
		return err
	}

	timeOut := time.After(1 * time.Second)

	select {
	case <-timeOut:
		return dca.ErrVoiceConnClosed
	case s.vc.OpusSend <- opus:
	}

	s.Lock()
	s.framesSent++
	s.Unlock()

	return nil
}

func (s *streamSession) start() {
	go s.stream()
}

func (s *streamSession) stop() bool {
	if !s.isStreaming() {
		return false
	}
	s.cancel <- true
	return true
}

func (s *streamSession) resume() bool {
	if s.isStreaming() {
		return false
	}
	s.start()
	return true
}

func (s *streamSession) isStreaming() bool {
	s.RLock()
	defer s.RUnlock()
	return s.streaming
}

func (s *streamSession) playbackPos() time.Duration {
	s.RLock()
	defer s.RUnlock()
	return time.Duration(s.framesSent) * s.source.FrameDuration()
}
