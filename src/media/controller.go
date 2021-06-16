package media

import (
	"context"
	"fmt"
	"io"
	lg "log"
	"net/url"
	"strings"
	"sync"
	"time"

	dgo "github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/bpipe"
	"github.com/dpatterbee/strife/src/media/youtube"
	"github.com/jonas747/dca"
	"github.com/rs/zerolog/log"
)

const (
	play = iota
	pause
	resume
	skip
	disconnect
	inspect
)

const stdTimeout = time.Millisecond * 500

type mediaSession struct {
	download *downloadSession
	encode   *dca.EncodeSession
	stream   *streamSession
}

type playerCommand struct {
	commandType   Action
	returnChannel chan string
}

type downloadSession struct {
	cancel context.CancelFunc
	sync.Mutex
}

type songReq struct {
	URL        string
	returnChan chan string
}

// mediaControlRouter function runs perpetually, maintaining a pool of active media sessions.
// It routes commands to the correct channel, creating a new media session if one is required to
// fulfill the request.
func controller(session *dgo.Session, mediaCommandChannel chan Request) {

	type activeMC struct {
		songChannel    chan songReq
		controlChannel chan playerCommand
	}

	type dyingMC struct {
		blocking bool // True if this media channel is blocking another from starting
		waitChan chan bool
	}

	// activeMCs is a map of media channels which are currently serving song requests.
	// dyingMCs is a map of media channels which have been instructed to shut down or have
	// timed out, but have not yet completed their shutdown tasks.
	activeMCs := make(map[string]activeMC)
	dyingMCs := make(map[string]dyingMC)

	// These channels are used by guildSoundPlayer goroutines to inform this goroutine of their
	// shutdown status.
	// mediaReturnBegin is used to inform that it has begun shutting down and that no more song or
	// command requests should be forwarded to that goroutine.
	// mediaReturnEnd is used to inform that it has completed shutting down and that any waiting
	// goroutines can be released.
	mediaReturnBegin := make(chan string)
	mediaReturnEnd := make(chan string)

	// This loops for the lifetime of the program, responding to messages sent on each channel.
	for {

		select {
		case req := <-mediaCommandChannel:
			// play and disconnect are special cases of command, as they create and destroy channels
			// all other commands just get passed through to the respective server.
			switch req.CommandType {
			case play:

				ch, ok := activeMCs[req.GuildID]
				if !ok {
					activeMCs[req.GuildID] = activeMC{
						controlChannel: make(chan playerCommand, 5),
						songChannel:    make(chan songReq, 100),
					}

					ch = activeMCs[req.GuildID]

					// Checks if there exists a guildSoundPlayer goroutine which is currently dying,
					// and creates the channel required so we can wait for the old goroutine to shut
					// down.
					var waitChan chan bool = nil
					if _, ok := dyingMCs[req.GuildID]; ok {
						waitChan = make(chan bool)
						dyingMCs[req.GuildID] = dyingMC{blocking: true, waitChan: waitChan}
					}
					go guildSoundPlayer(
						session,
						req.GuildID,
						req.ChannelID,
						ch.controlChannel,
						ch.songChannel,
						mediaReturnBegin,
						mediaReturnEnd,
						waitChan,
					)
				}

				songReq := songReq{
					URL:        req.CommandData,
					returnChan: req.ReturnChan,
				}

				select {
				case ch.songChannel <- songReq:
				default:
					go trySend(req.ReturnChan, "Queue full, please try again later.", stdTimeout)
				}

			case disconnect:
				mediaChannel, ok := activeMCs[req.GuildID]

				if ok {
					// TODO: There's definitely a potential scenario where this never properly
					// sends the disconnect signal and we end up with a zombie goroutine holding
					// onto a voiceconnection for a while.
					go reqPassTimeoutClose(mediaChannel.controlChannel, req, 10*time.Minute)

					dyingMCs[req.GuildID] = dyingMC{blocking: false, waitChan: nil}
					delete(activeMCs, req.GuildID)
				}

			default:

				mc, ok := activeMCs[req.GuildID]
				if ok {
					go reqPass(mc.controlChannel, req)
				}
			}
		case guildID := <-mediaReturnBegin:

			// When a guildSoundPlayer goroutine informs us that they are beginning to shut down,
			// we close the communications channels and drain them,
			// then create an entry in our dyingMCs map and remove from activeMCs
			m, ok := activeMCs[guildID]
			close(m.controlChannel)
			if ok {
				dyingMCs[guildID] = dyingMC{}
				delete(activeMCs, guildID)
			}
		case guildID := <-mediaReturnEnd:

			// When a guildSoundPlayer goroutine informs us that it has completed shutting down,
			// we check if there is a waiting goroutine, and if so,
			// we signal it by closing the waitChan, then remove it from the map.
			// If there is no waiting goroutine, we just remove it from the map.
			if dyingChannel, ok := dyingMCs[guildID]; ok {
				if dyingChannel.blocking {
					go func(ch chan<- bool) {
						close(ch)
					}(dyingChannel.waitChan)
				}
				delete(dyingMCs, guildID)
			}

		}

	}

}

// reqPass passes a mediaRequest down a playerCommand channel, timing out after stdTimeout
func reqPass(ch chan playerCommand, req Request) {
	reqPassTimeout(ch, req, stdTimeout)
}

func reqPassTimeoutClose(ch chan playerCommand, req Request, timeout time.Duration) {
	reqPassTimeout(ch, req, timeout)
	close(ch)
}

func reqPassTimeout(ch chan playerCommand, req Request, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	select {
	case ch <- playerCommand{commandType: req.CommandType, returnChannel: req.ReturnChan}:
		timer.Stop()
		return
	case <-timer.C:
		go trySend(req.ReturnChan, "Server busy", stdTimeout)
		return
	}
}

// streamSong uses youtube-dl to download the song and pipe the stream of data to writePipe
func streamSong(writePipe io.WriteCloser, video streamable, d *downloadSession) {

	ctx, cancel := context.WithCancel(context.Background())
	stream, length, err := video.Stream(ctx)
	if err != nil {
		log.Error().Err(err).Msg("")
		cancel()
		err := writePipe.Close()
		if err != nil {
			log.Error().Err(err).Msg("")
		}
	}
	log.Info().Msg(fmt.Sprint(length))
	defer func(stream io.ReadCloser) {
		err := stream.Close()
		if err != nil {
			log.Error().Err(err).Msg("")
		}
	}(stream)
	if err != nil {
		log.Error().Err(err).Msg("")
		cancel()
		err := writePipe.Close()
		if err != nil {
			log.Error().Err(err).Msg("")
		}
		return
	}
	d.cancel = cancel
	d.Unlock()

	_, err = io.Copy(writePipe, stream)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

	if err != nil {
		switch err {
		case context.Canceled:
			log.Info().Msg("Download cancelled.")
		default:
			log.Error().Err(err).Msg("Data Copy Failed.")
		}
	} else {
		log.Info().Msg("Finished download.")
	}
	err = writePipe.Close()
	if err != nil {
		log.Error().Err(err).Msg("")
	}
}

var audioQualities = map[string]int{
	"AUDIO_QUALITY_LOW":    1,
	"AUDIO_QUALITY_MEDIUM": 2,
	"AUDIO_QUALITY_HIGH":   3,
}

func newMediaSession(s streamable, vc *dgo.VoiceConnection) (*mediaSession, error) {
	bufPipe := bpipe.New()

	var d downloadSession
	d.Lock()
	go streamSong(bufPipe, s, &d)

	// Trick the dca module into using my logger with level=warn
	t := log.With().Str("level", "warn").Logger()
	dca.Logger = lg.New(t, "", 0)

	encode, err := dca.EncodeMem(bufPipe, dca.StdEncodeOptions)
	if err != nil {
		return nil, err
	}

	stream := newStreamingSession(encode, vc)

	return &mediaSession{
		download: &d,
		encode:   encode,
		stream:   stream,
	}, nil

}

func (m *mediaSession) stop() {
	m.stream.Stop()
	m.download.safeCancel()
	m.encode.Cleanup()
}

func (m *mediaSession) pause() bool {
	return m.stream.Pause()
}

func (m *mediaSession) resume() bool {
	return m.stream.Resume()
}

func (d *downloadSession) safeCancel() {
	d.Lock()
	d.cancel()
	d.Unlock()
}

func prettySongList(yts []streamable, currentSongPos time.Duration) string {
	var sb strings.Builder
	durationUntilNow := currentSongPos

	for i, v := range yts {
		// Writes to strings.Builder cannot error
		_, _ = fmt.Fprintf(&sb, "%d. %s | Playing in: %v\n", i+1, v.Title(),
			durationUntilNow.Truncate(time.Second).String())
		durationUntilNow = durationUntilNow + v.Duration()
	}
	return sb.String()
}

type queueConfig struct {
	requestChan      <-chan songReq
	nextSong         chan streamable
	inspectSongQueue chan chan []streamable
	shutdown         chan chan []streamable
	firstSongWait    chan bool
}

func newSongQueue(requestChan <-chan songReq) queueConfig {
	s := queueConfig{
		requestChan:      requestChan,
		nextSong:         make(chan streamable),
		inspectSongQueue: make(chan chan []streamable),
		shutdown:         make(chan chan []streamable),
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

	var songQueue []streamable
	nullQ := make(chan streamable)
	var songChannel *chan streamable
	log.Info().Msg("Song queue ready")
	for {
		var nextSong streamable
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

			url, err := url.Parse(strings.TrimSpace(song.URL))
			if err != nil {
				log.Error().Err(err).Msg("")
				if first {
					return
				}
				break
			}

			var s streamable
			switch url.Hostname() {
			case "www.youtube.com", "youtu":
				s, err = youtube.NewVideo(url.String())
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
				config.firstSongWait <- success
				close(config.firstSongWait)
			}

		case *songChannel <- nextSong:
			songQueue = songQueue[1:]
		case ret := <-config.inspectSongQueue:
			// This is slightly confusing. We do this rather than just sending directly on the
			// channel so that we avoid data races and also only copy when required.
			q := make([]streamable, len(songQueue))
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
