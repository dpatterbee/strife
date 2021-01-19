package strife

import (
	"errors"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

type streamSession struct {
	vc *discordgo.VoiceConnection

	stop chan bool
	done chan error
	skip chan bool

	source dca.OpusReader

	streaming bool

	sync.RWMutex
}

func newStreamingSession(source dca.OpusReader, vc *discordgo.VoiceConnection) *streamSession {

	session := &streamSession{
		vc:     vc,
		source: source,
		done:   make(chan error),
		stop:   make(chan bool),
		skip:   make(chan bool),
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
		case <-s.skip:
			s.Lock()
			s.streaming = false
			s.Unlock()
			go func() {
				s.done <- errors.New("User Interrupt")
			}()
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

	return nil
}

func (s *streamSession) Pause() {
	s.RLock()
	if !s.streaming {
		return
	}
	s.RUnlock()

	timeout := time.After(5 * time.Second)

	select {
	case <-timeout:
	case s.stop <- true:
	}
}

func (s *streamSession) Resume() {

	s.RLock()
	if s.streaming {
		return
	}
	s.RUnlock()

	go s.stream()
}

func (s *streamSession) Skip() {
	s.RLock()
	if !s.streaming {
		return
	}
	s.RUnlock()

	timeout := time.After(5 * time.Second)

	select {
	case <-timeout:
	case s.skip <- true:
	}

}
