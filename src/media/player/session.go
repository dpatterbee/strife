package player

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/strife/src/media"
	"github.com/dpatterbee/strife/src/media/queue"
	"github.com/dpatterbee/strife/src/media/streamer"
	"github.com/pkg/errors"
)

// ErrDisconnect signifies that the streamer.Session ended with a disconnect command
var ErrDisconnect = errors.New("disconnect")

type playerCommand struct {
	commandType media.Action
}

type Session struct {
	dgoSession *discordgo.Session

	guildID, channelID string

	commandChan <-chan playerCommand

	queue *queue.Queue

	vc *discordgo.VoiceConnection

	returner *returner
	waitChan chan struct{}
}

func (s Session) passRequest(request media.Request) {

}

func (s Session) run() {
	defer s.returner.Exit(s.guildID)
	// TODO: log
	if s.waitChan != nil {
		<-s.waitChan
	}

	if s.queue == nil {
		s.queue = queue.NewSongQueue(s.)
	}
	defer s.queue.Exit()

	err := s.queue.Wait()
	if err != nil {
		// TODO: log
		s.returner.Exit(s.guildID)
		return
	}

	vc, err := s.dgoSession.ChannelVoiceJoin(s.guildID, s.channelID, false, true)
	defer func(vc *discordgo.VoiceConnection) {
		err := vc.Disconnect()
		if err != nil {
			// TODO: log
		}
	}(vc)

	if err != nil {
		// TODO: log
		s.returner.Exit(s.guildID)
		return
	}

	timerDuration := 15 * time.Second
	disconnectTimer := time.NewTimer(timerDuration)
	defer func() {
		if !disconnectTimer.Stop() {
			<-disconnectTimer.C
		}
	}()
	for {
		if !disconnectTimer.Stop() {
			<-disconnectTimer.C
		}
		disconnectTimer.Reset(timerDuration)

		select {
		case cmd := <-s.commandChan:
			switch cmd.commandType {
			case media.DISCONNECT:
				// TODO: Leaving message
				return
			default:
				// TODO: NO MEDIA PLAYING
			}

		case song := <-s.queue.SongChannel:
			// TODO: Log
			err := s.handlePlayingSong(song)
			if err == ErrDisconnect {
				return
			} else if err != nil {
				// TODO: log
			}

		case <-disconnectTimer.C:
			return

		}
	}
}

func (s *Session) handlePlayingSong(song media.Streamable) error {
	stream, err := streamer.NewSession(s.vc, song)
	if err != nil {
		return err
	}
	stream = stream
	stream.Start()

	s.vc.Speaking(true)
	defer s.vc.Speaking(false)
	for {
		select {

		case err := <-stream.Done:
			// Ensure the song cleans up okay.
			stream.Stop()
			return err

		case control := <-s.commandChan:

			switch control.commandType {

			case media.PAUSE:
				ok := stream.Pause()
				if ok {
					// Send "Song paused."
				} else {
					// Send "Song already paused."
				}

			case media.RESUME:
				ok := stream.Resume()
				if ok {
					// Send "Song resumed."
				} else {
					// Send "Song already playing"
				}

			case media.SKIP:
				stream.Stop()
				// Send "Song skipped."
				return nil

			case media.DISCONNECT:
				stream.Stop()
				// Send "Goodbye."
				return ErrDisconnect
			}
		}
	}

}
