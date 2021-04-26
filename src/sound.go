package strife

import (
	"context"
	"fmt"
	"io"
	lg "log"
	"strings"
	"sync"
	"time"

	dgo "github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/bpipe"
	"github.com/jonas747/dca"
	yt "github.com/kkdai/youtube/v2"
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

type mediaSession struct {
	download *downloadSession
	encode   *dca.EncodeSession
	stream   *streamSession
}

type mediaRequest struct {
	commandType int
	guildID     string
	channelID   string
	commandData string
	returnChan  chan string
}

type playerCommand struct {
	commandType   int
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
func mediaControlRouter(session *dgo.Session, mediaCommandChannel chan mediaRequest) {

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
			switch req.commandType {
			case play:

				ch, ok := activeMCs[req.guildID]
				if !ok {
					activeMCs[req.guildID] = activeMC{
						controlChannel: make(chan playerCommand, 5),
						songChannel:    make(chan songReq, 100),
					}

					ch = activeMCs[req.guildID]

					// Checks if there exists a guildSoundPlayer goroutine which is currently dying,
					// and creates the channel required so we can wait for the old goroutine to shut
					// down.
					var waitChan chan bool = nil
					if _, ok := dyingMCs[req.guildID]; ok {
						waitChan = make(chan bool)
						dyingMCs[req.guildID] = dyingMC{blocking: true, waitChan: waitChan}
					}
					go guildSoundPlayer(
						session,
						req.guildID,
						req.channelID,
						ch.controlChannel,
						ch.songChannel,
						mediaReturnBegin,
						mediaReturnEnd,
						waitChan,
					)
				}

				songReq := songReq{
					URL:        req.commandData,
					returnChan: req.returnChan,
				}

				select {
				case ch.songChannel <- songReq:
				default:
					go trySend(req.returnChan, "Queue full, please try again later.", stdTimeout)
				}

			case disconnect:
				mediaChannel, ok := activeMCs[req.guildID]

				if ok {
					// TODO: There's definitely a potential scenario where this never properly
					// sends the disconnect signal and we end up with a zombie goroutine holding
					// onto a voiceconnection for a while.
					go reqPassTimeoutClose(mediaChannel.controlChannel, req, 10*time.Minute)

					dyingMCs[req.guildID] = dyingMC{blocking: false, waitChan: nil}
					delete(activeMCs, req.guildID)
				}

			default:

				mc, ok := activeMCs[req.guildID]
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
func reqPass(ch chan playerCommand, req mediaRequest) {
	reqPassTimeout(ch, req, stdTimeout)
}

func reqPassTimeoutClose(ch chan playerCommand, req mediaRequest, timeout time.Duration) {
	reqPassTimeout(ch, req, timeout)
	close(ch)
}

func reqPassTimeout(ch chan playerCommand, req mediaRequest, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	select {
	case ch <- playerCommand{commandType: req.commandType, returnChannel: req.returnChan}:
		timer.Stop()
		return
	case <-timer.C:
		go trySend(req.returnChan, "Server busy", stdTimeout)
		return
	}
}

// streamSong uses youtube-dl to download the song and pipe the stream of data to w
func streamSong(writePipe io.WriteCloser, video *yt.Video, d *downloadSession) {

	client := yt.Client{}

	ctx, cancel := context.WithCancel(context.Background())
	resp, err := client.GetStreamContext(ctx, video, &video.Formats[0])
	if err != nil {
		log.Error().Err(err).Msg("")
		cancel()
		err := writePipe.Close()
		if err != nil {
			log.Error().Err(err).Msg("")
		}
		return
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Error().Err(err).Msg("")
		}
	}(resp.Body)
	d.cancel = cancel
	d.Unlock()

	_, err = io.Copy(writePipe, resp.Body)

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

func newMediaSession(s *yt.Video, vc *dgo.VoiceConnection) (*mediaSession, error) {
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

func getUserVoiceChannel(sess *dgo.Session, userID, guildID string) (string, error) {

	guild, err := sess.State.Guild(guildID)
	if err != nil {
		return "", err
	}

	for _, v := range guild.VoiceStates {
		if v.UserID == userID {
			return v.ChannelID, nil
		}
	}

	return "", fmt.Errorf("error: user not in voice channel")

}

// guildSoundPlayer runs while a server has a queue of songs to be played.
// It loops over the queue of songs and plays them in order, exiting once it has drained the list
func guildSoundPlayer(
	discordSession *dgo.Session,
	guildID, channelID string,
	controlChannel <-chan playerCommand,
	songChannel <-chan songReq,
	mediaReturnRequestChan, mediaReturnFinishChan chan<- string,
	previousInstanceWaitChan <-chan bool,
) {
	log.Info().Msg("Sound handler not active, activating")

	if previousInstanceWaitChan != nil {
		<-previousInstanceWaitChan
	}

	queue := newSongQueue(songChannel)

	if !<-queue.firstSongWait {
		log.Info().Msg("Initial song request too long, shutting down.")
		mediaReturnRequestChan <- guildID
		mediaReturnFinishChan <- guildID
		return
	}

	// Set up voiceconnection
	vc, err := discordSession.ChannelVoiceJoin(guildID, channelID, false, true)

	if err != nil {
		mediaReturnRequestChan <- guildID
		log.Error().Err(err).Msg("Couldn't initialise voice connection")
		return
	}

	disconnectTimer := time.NewTimer(5 * time.Second)

mainLoop:
	for {
		if !disconnectTimer.Stop() {
			<-disconnectTimer.C
		}
		disconnectTimer.Reset(5 * time.Second)
		select {
		case control := <-controlChannel:
			switch control.commandType {
			case disconnect:
				go trySend(control.returnChannel, "Goodbye.", stdTimeout)
				break mainLoop
			default:
				go trySend(control.returnChannel, "No media playing.", stdTimeout)
			}
		case song := <-queue.nextSong:
			log.Info().
				Str("URL", song.ID).
				Str("Title", song.Title).
				Str("guildID", guildID).
				Msg("Playing Song")
			//encode, download, err := newSongSession(song)
			//streamingSession := newStreamingSession(encode, vc)
			mediaSession, err := newMediaSession(song, vc)
			if err != nil {
				log.Error().Err(err).Msg("")
				return
			}

			err = vc.Speaking(true)
			if err != nil {
				log.Error().Err(err).Msg("")
			}

			log.Info().
				Str("guildID", guildID).
				Str("channelID", channelID).
				Msg("Starting Audio Stream")

			mediaSession.stream.Start()

			// controlLoop should only be entered once it is possible to control the media ie. once
			// the ffmpeg session is up and running
		controlLoop:
			for {
				select {

				case err := <-mediaSession.stream.done:
					if err := vc.Speaking(false); err != nil {
						log.Error().Err(err).Msg("")
					}
					mediaSession.stop() // Ensure the song cleans up okay.
					if err == io.EOF {
						log.Info().Msg("Song Completed.")
					} else {
						log.Error().Err(err).Msg("Song Stopped.")
					}
					break controlLoop

				case control := <-controlChannel:

					switch control.commandType {

					case pause:
						ok := mediaSession.pause()
						if ok {
							go trySend(control.returnChannel, "Song paused.", stdTimeout)
						} else {
							go trySend(control.returnChannel, "Song already paused.", stdTimeout)
						}

					case resume:
						ok := mediaSession.resume()
						if ok {
							go trySend(control.returnChannel, "Song resumed.", stdTimeout)
						} else {
							go trySend(control.returnChannel, "Song already playing", stdTimeout)
						}

					case skip:
						if !mediaSession.encode.Running() {
							go trySend(control.returnChannel, "Not yet.", stdTimeout)
							continue

						}
						mediaSession.stop()

						go trySend(control.returnChannel, "Song skipped.", stdTimeout)

						break controlLoop

					case disconnect:
						if !mediaSession.encode.Running() {
							go trySend(control.returnChannel, "Not yet.", stdTimeout)
							continue

						}
						mediaSession.stop()

						go trySend(control.returnChannel, "Goodbye.", stdTimeout)

						break mainLoop

					case inspect:
						qch := make(chan []*yt.Video)
						queue.inspectSongQueue <- qch
						q := <-qch
						songTimeRemaining := song.Duration - mediaSession.stream.PlaybackPos()
						go trySend(control.returnChannel, prettySongList(q, songTimeRemaining), stdTimeout)
					}
				}
			}

		case <-disconnectTimer.C:
			mediaReturnRequestChan <- guildID
			break mainLoop

		}

	}

	// End queue goroutine and disconnect from voice channel before informing the coordinator
	// that we have finished.
	// TODO: I have implemented the potential for returning the queue after a session ends. This
	//  could be recovered afterwards.
	remainingQ := make(chan []*yt.Video)
	queue.shutdown <- remainingQ
	<-remainingQ
	err = vc.Disconnect()
	if err != nil {
		log.Error().Err(err).Msg("")
	}
	mediaReturnFinishChan <- guildID

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

func prettySongList(yts []*yt.Video, currentSongPos time.Duration) string {
	var sb strings.Builder
	durationUntilNow := currentSongPos

	for i, v := range yts {
		// Writes to strings.Builder cannot error
		_, _ = fmt.Fprintf(&sb, "%d. %s | Playing in: %v\n", i+1, v.Title,
			durationUntilNow.Truncate(time.Second).String())
		durationUntilNow = durationUntilNow + v.Duration
	}
	return sb.String()
}

type sq struct {
	requestChan      <-chan songReq
	nextSong         chan *yt.Video
	inspectSongQueue chan chan []*yt.Video
	shutdown         chan chan []*yt.Video
	firstSongWait    chan bool
}

func newSongQueue(requestChan <-chan songReq) sq {
	s := sq{
		requestChan:      requestChan,
		nextSong:         make(chan *yt.Video),
		inspectSongQueue: make(chan chan []*yt.Video),
		shutdown:         make(chan chan []*yt.Video),
		firstSongWait:    make(chan bool),
	}
	go songQueue(s)

	return s
}

func songQueue(config sq) {
	client := yt.Client{}
	first := true
	success := false

	var songQueue []*yt.Video
	nullQ := make(chan *yt.Video)
	var songChannel *chan *yt.Video
	for {
		var nextSong *yt.Video
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
			func(url songReq) {
				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				defer cancel()
				vid, err := client.GetVideoContext(ctx, song.URL)
				if err != nil {
					go trySend(song.returnChan, "Song not found.", stdTimeout)
					return
				}
				if vid.Duration <= time.Hour {
					songQueue = append(songQueue, vid)
					go trySend(song.returnChan, "Song added to queue.", stdTimeout)
					success = true
				} else {
					go trySend(song.returnChan, "Song too long.", stdTimeout)
				}
			}(song)
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
			q := make([]*yt.Video, len(songQueue))
			copy(q, songQueue)
			// This is a blocking send. The receiver must listen immediately or be put to death.
			ret <- q
		case sht := <-config.shutdown:
			sht <- songQueue
			return
		}
	}

}
