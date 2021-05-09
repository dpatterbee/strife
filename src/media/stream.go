package media

import (
	"sync"
	"time"

	dgo "github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

type streamSession struct {
	vc *dgo.VoiceConnection

	stop chan bool
	done chan error

	source     dca.OpusReader
	framesSent int

	streaming bool

	sync.RWMutex
}

func newStreamingSession(source dca.OpusReader, vc *dgo.VoiceConnection) *streamSession {

	session := &streamSession{
		vc:     vc,
		source: source,
		done:   make(chan error),
		stop:   make(chan bool),
	}

	return session
}

func (s *streamSession) Start() {
	go s.stream()
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
		case <-s.stop:
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

func (s *streamSession) Streaming() bool {
	s.RLock()
	defer s.RUnlock()
	return s.streaming
}

// Pause pauses the current streamSession.
// Due to the nature of the streamSession, the only difference between pausing and
// stopping a stream is that stopping is typically followed by the discarding of the
// streamSession itself. Hence Pause is simply an alias of Stop.
func (s *streamSession) Pause() bool {
	return s.Stop()
}

func (s *streamSession) Stop() bool {
	if !s.Streaming() {
		return false
	}
	s.stop <- true
	return true
}

func (s *streamSession) Resume() bool {
	if s.Streaming() {
		return false
	}

	s.Start()
	return true
}

func (s *streamSession) PlaybackPos() time.Duration {
	s.Lock()
	defer s.Unlock()
	return time.Duration(s.framesSent) * s.source.FrameDuration()
}
