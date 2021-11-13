package queue

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dpatterbee/strife/src/media"
	"github.com/dpatterbee/strife/src/media/youtube"
	"github.com/rs/zerolog/log"
)

type Queue struct {
	SongChannel   chan media.Streamable
	firstSongWait chan struct{}
}

func NewSongQueue(sc chan media.Streamable) *Queue {
	return &Queue{
		SongChannel: sc,
	}
}

func (q *Queue) FirstSongWait() bool {

}

func (q *Queue) AddSong() bool {

}

func (q *Queue) GetSong() (media.Streamable, error) {

}

func (q *Queue) Wait() error {

}

func (q *Queue) Exit() {

}

type queueConfig struct {
	requestChan      <-chan songReq
	nextSong         chan media.Streamable
	inspectSongQueue chan chan []media.Streamable
	shutdown         chan chan []media.Streamable
	firstSongWait    chan bool
}

func newSongQueue(requestChan <-chan songReq) queueConfig {
	s := queueConfig{
		requestChan:      requestChan,
		nextSong:         make(chan media.Streamable),
		inspectSongQueue: make(chan chan []media.Streamable),
		shutdown:         make(chan chan []media.Streamable),
		firstSongWait:    make(chan bool),
	}
	go songQueue(s)

	return s
}

func songQueue(config queueConfig) {
	first := true
	success := false

	defer func(ch chan<- bool) {
		if first {
			ch <- false
			close(config.firstSongWait)
		}
	}(config.firstSongWait)

	var songQueue []media.Streamable
	nullQ := make(chan media.Streamable)
	var songChannel *chan media.Streamable
	log.Info().Msg("Song queue ready")
	for {
		var nextSong media.Streamable
		// This if statement prevents the sending of songs to the player routine if there are no
		// songs in the queue.
		// It sets the channel to a channel which blocks forever,
		// and the song to be sent is a nil pointer.
		if len(songQueue) == 0 {
			songChannel = &nullQ
		} else {
			nextSong = songQueue[0]
			songChannel = &config.nextSong
		}

		select {
		case song := <-config.requestChan:

			rui, err := url.Parse(strings.TrimSpace(song.URL))
			if err != nil {
				log.Error().Err(err).Msg("")
				if first {
					return
				}
				break
			}

			var s media.Streamable
			switch rui.Hostname() {
			case "www.youtube.com", "youtu":
				s, err = youtube.NewVideo(rui.String())
			case "":
				var ID string
				ID, err = searchClient.SearchTopID(song.URL)
				if err != nil {
					break
				}
				s, err = youtube.NewVideo(ID)
			default:
			}

			if err != nil {
				log.Error().Err(err).Msg("")
				go trySend(song.returnChan, "Song not found.", stdTimeout)
				if first {
					return
				}
				continue
			}

			if s != nil {
				if s.Duration() <= time.Hour {
					songQueue = append(songQueue, s)
					go trySend(song.returnChan, fmt.Sprintf("Song added to queue: %v", s.Title()), stdTimeout)
					success = true
				} else {
					log.Error().Msg(fmt.Sprintf("Song duration: %v", s.Duration()))
					go trySend(song.returnChan, "Song too long.", stdTimeout)
				}

			}

			if first {
				first = !first
				close(config.firstSongWait)
			}

		case *songChannel <- nextSong:
			songQueue = songQueue[1:]
		case ret := <-config.inspectSongQueue:
			// This is slightly confusing. We do this rather than just sending directly on the
			// channel so that we avoid data races and also only copy when required.
			q := make([]media.Streamable, len(songQueue))
			copy(q, songQueue)
			// This is a blocking send. The receiver must listen immediately or be put to death.
			ret <- q
		case sht := <-config.shutdown:
			sht <- songQueue
			return
		}
	}

}

// trySend attempts to send "data" on "channel", timing out after "timeoutDuration".
func trySend(channel chan string, data string, timeoutDuration time.Duration) {
	// this will sure lend itself to generics when the time comes.
	timeout := time.NewTimer(timeoutDuration)

	select {
	case channel <- data:
		timeout.Stop()
	case <-timeout.C:
		return
	}
}
