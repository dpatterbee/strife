package strife

import (
	"context"
	"errors"
	"fmt"
	"io"
	lg "log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/bpipe"
	"github.com/jonas747/dca"
	youtube "github.com/kkdai/youtube/v2"
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

var (
	reg *regexp.Regexp = regexp.MustCompile(`^(https?)://(-\.)?([^\s/?\.#-]+\.?)+(/[^\s]*)?$`)
)

type songURL struct {
	requester  string
	isURL      bool
	submission string
}

type mediaSession struct {
	download *downloadSession
	encode   *dca.EncodeSession
	stream   *streamSession
}

type mediaRequest struct {
	commandType   int
	guildID       string
	channelID     string
	commandData   string
	returnChannel chan string
}

type mediaCommand struct {
	commandType   int
	returnChannel chan string
}

type downloadSession struct {
	cncl context.CancelFunc
	sync.Mutex
}

type songRequest struct {
	URL        string
	returnChan chan string
}

// mediaControlRouter function runs perpetually, maintaining a pool of all currently active media sessions.
// It routes commands to the correct channel, creating a new media session if one is required to fulfill the request.
func mediaControlRouter(session *discordgo.Session, mediaCommandChannel chan mediaRequest) {

	type activeMediaChannel struct {
		songChannel    chan songRequest
		controlChannel chan mediaCommand
	}

	type dyingMediaChannel struct {
		dependedOn     bool
		dependencyChan chan bool
	}

	// activeMediaChannels is a map of channels which are currently serving song requests.
	// dyingMediaChannels is a map of channels which have been instructed to shut down or have timed out, but have not yet completed their shutdown tasks.
	activeMediaChannels := make(map[string]activeMediaChannel)
	dyingMediaChannels := make(map[string]dyingMediaChannel)

	// These channels are used by guildSoundPlayer goroutines to inform this goroutine of their shutdown status.
	// mediaReturnBegin is used to inform that it has begun shutting down and that no more song or command requests should be forwarded to that goroutine.
	// mediaReturnEnd is used to inform that it has completed shutting down and that any waiting goroutines can be released.
	mediaReturnBegin := make(chan string)
	mediaReturnEnd := make(chan string)

	// This loops for the lifetime of the program, responding to messages sent on each channel.
	for {

		select {
		case request := <-mediaCommandChannel:
			// play and disconnect are special cases of command, as they can create and destroy channels.
			// all other commands just get passed through to the respective server.
			switch request.commandType {
			case play:

				ch, ok := activeMediaChannels[request.guildID]
				if !ok {
					activeMediaChannels[request.guildID] = activeMediaChannel{
						controlChannel: make(chan mediaCommand, 5),
						songChannel:    make(chan songRequest, 100),
					}

					ch = activeMediaChannels[request.guildID]

					// Checks if there exists a guildSoundPlayer goroutine which is currently dying, and creates the channel required so we can wait for the old goroutine to shut down.
					var dependencyChan chan bool = nil
					if _, ok := dyingMediaChannels[request.guildID]; ok {
						dependencyChan = make(chan bool)
						dyingMediaChannels[request.guildID] = dyingMediaChannel{dependedOn: true, dependencyChan: dependencyChan}
					}
					go guildSoundPlayer(
						session,
						request.guildID,
						request.channelID,
						ch.controlChannel,
						ch.songChannel,
						mediaReturnBegin,
						mediaReturnEnd,
						dependencyChan,
					)
				}

				songReq := songRequest{
					URL:        request.commandData,
					returnChan: request.returnChannel,
				}

				select {
				case ch.songChannel <- songReq:
				default:
					go trySend(request.returnChannel, "Queue full, please try again later.", standardTimeout)
				}

			case disconnect:
				mediaChannel, ok := activeMediaChannels[request.guildID]

				if ok {
					//TODO: There's definitely a potential scenario where this never properly sends the disconnect signal and we end up with
					//a zombie goroutine holding onto a voiceconnection for a while.
					go passCommandThroughToGuildSoundPlayerWithTimeoutAndClose(mediaChannel.controlChannel, request, 10*time.Minute)

					dyingMediaChannels[request.guildID] = dyingMediaChannel{dependedOn: false, dependencyChan: nil}
					delete(activeMediaChannels, request.guildID)
				}

			default:

				mc, ok := activeMediaChannels[request.guildID]
				if ok {
					go passCommandThroughToGuildSoundPlayer(mc.controlChannel, request)
				}
			}
		case guildID := <-mediaReturnBegin:

			// When a guildSoundPlayer goroutine informs us that they are beginning to shut down, we close the communications channels and drain them,
			// then create an entry in our dyingMediaChannels map and remove from activeMediaChannels
			m, ok := activeMediaChannels[guildID]
			close(m.controlChannel)
			if ok {
				dyingMediaChannels[guildID] = dyingMediaChannel{}
				delete(activeMediaChannels, guildID)
			}
		case guildID := <-mediaReturnEnd:

			// When a guildSoundPlayer goroutine informs us that it has completed shutting down, if that shutdown has a dependency, we close the channel
			// to let the dependant know it can continue, and we remove it from the map. Otherwise we simply remove it from the map.
			if dyingChannel, ok := dyingMediaChannels[guildID]; ok {
				if dyingChannel.dependedOn {
					go func(ch chan<- bool) {
						close(ch)
					}(dyingChannel.dependencyChan)
				}
				delete(dyingMediaChannels, guildID)
			}

		}

	}

}
func passCommandThroughToGuildSoundPlayer(controlChan chan mediaCommand, elem mediaRequest) {
	passCommandThroughToGuildSoundPlayerWithTimeout(controlChan, elem, standardTimeout)
}
func passCommandThroughToGuildSoundPlayerWithTimeoutAndClose(controlChan chan mediaCommand, elem mediaRequest, timeout time.Duration) {
	passCommandThroughToGuildSoundPlayerWithTimeout(controlChan, elem, timeout)
	close(controlChan)
}

func passCommandThroughToGuildSoundPlayerWithTimeout(controlChan chan mediaCommand, elem mediaRequest, timeout time.Duration) {
	timer := time.NewTimer(timeout)
	select {
	case controlChan <- mediaCommand{commandType: elem.commandType, returnChannel: elem.returnChannel}:
		timer.Stop()
		return
	case <-timer.C:
		go trySend(elem.returnChannel, "Serber bisi", standardTimeout)
		return
	}
}

func parseURL(m *discordgo.MessageCreate, s string) (songURL, error) {
	var loc songURL

	bol := isURL(s)

	loc.isURL = bol
	loc.submission = s
	loc.requester = m.Author.ID

	if !bol {
		return loc, errors.New("Not valid url")
	}

	return loc, nil
}

func isURL(s string) bool {
	return reg.MatchString(s)
}

// streamSong uses youtube-dl to download the song and pipe the stream of data to w
func streamSong(writePipe io.WriteCloser, video *youtube.Video, d *downloadSession) {

	client := youtube.Client{}

	ctx, cancel := context.WithCancel(context.Background())
	resp, err := client.GetStreamContext(ctx, video, &video.Formats[0])
	if err != nil {
		log.Error().Err(err).Msg("")
		cancel()
		writePipe.Close()
		return
	}
	defer resp.Body.Close()
	d.cncl = cancel
	d.Unlock()
	if err != nil {
		log.Info().Msg(err.Error())
	}
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
	writePipe.Close()
}

func newSongSession(s *youtube.Video) (*dca.EncodeSession, *downloadSession, error) {

	bufPipe := bpipe.New()

	var d downloadSession

	d.Lock()
	go streamSong(bufPipe, s, &d)

	// Trick the dca module into using my logger with level=warn
	t := log.With().Str("level", "warn").Logger()
	dca.Logger = lg.New(t, "", 0)

	ss, err := dca.EncodeMem(bufPipe, dca.StdEncodeOptions)
	if err != nil {
		return nil, nil, err
	}

	return ss, &d, nil
}

func newMediaSession(s *youtube.Video, vc *discordgo.VoiceConnection) (*mediaSession, error) {
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

func getUserVoiceChannel(sess *discordgo.Session, userID, guildID string) (string, error) {

	guild, err := sess.State.Guild(guildID)
	if err != nil {
		return "", err
	}

	for _, v := range guild.VoiceStates {
		if v.UserID == userID {
			return v.ChannelID, nil
		}
	}

	return "", fmt.Errorf("User not in voice channel")

}

// guildSoundPlayer runs while a server has a queue of songs to be played.
// It loops over the queue of songs and plays them in order, exiting once it has drained the list
func guildSoundPlayer(
	discordSession *discordgo.Session,
	guildID, channelID string,
	controlChannel <-chan mediaCommand,
	songChannel <-chan songRequest,
	mediaReturnRequestChan, mediaReturnFinishChan chan<- string,
	previousInstanceWaitChan <-chan bool,
) {
	log.Info().Msg("Soundhandler not active, activating")

	if previousInstanceWaitChan != nil {
		<-previousInstanceWaitChan
	}

	nextSong, inspectSongQueue, queueShutDown, firstSongWait := newSongQueue(songChannel)

	if !<-firstSongWait {
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
				go trySend(control.returnChannel, "Goodbye.", standardTimeout)
				break mainLoop
			default:
				go trySend(control.returnChannel, "No media playing.", standardTimeout)
			}
		case song := <-nextSong:
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

			vc.Speaking(true)

			log.Info().
				Str("guildID", guildID).
				Str("channelID", channelID).
				Msg("Starting Audio Stream")

			mediaSession.stream.Start()

			// controlLoop should only be entered once it is possible to control the media ie. once the ffmpeg session is up and running
		controlLoop:
			for {
				select {

				case err := <-mediaSession.stream.done:
					vc.Speaking(false)
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
							go trySend(control.returnChannel, "Song paused.", standardTimeout)
						} else {
							go trySend(control.returnChannel, "Song already paused.", standardTimeout)
						}

					case resume:
						ok := mediaSession.resume()
						if ok {
							go trySend(control.returnChannel, "Song resumed.", standardTimeout)
						} else {
							go trySend(control.returnChannel, "Song already playing", standardTimeout)
						}

					case skip:
						if !mediaSession.encode.Running() {
							go trySend(control.returnChannel, "Not yet.", standardTimeout)
							continue

						}
						mediaSession.stop()

						go trySend(control.returnChannel, "Song skipped.", standardTimeout)

						break controlLoop

					case disconnect:
						if !mediaSession.encode.Running() {
							go trySend(control.returnChannel, "Not yet.", standardTimeout)
							continue

						}
						mediaSession.stop()

						go trySend(control.returnChannel, "Goodbye.", standardTimeout)

						break mainLoop

					case inspect:
						qch := make(chan []*youtube.Video)
						inspectSongQueue <- qch
						q := <-qch
						currentSongTimeRemaining := song.Duration - mediaSession.stream.PlaybackPos()
						go trySend(control.returnChannel, makePrettySongList(q, currentSongTimeRemaining), standardTimeout)
					}
				}
			}

		case <-disconnectTimer.C:
			mediaReturnRequestChan <- guildID
			break mainLoop

		}

	}

	// End queue goroutine and disconnect from voice channel before informing the coordinator that we have finished.
	// TODO: I have implemented the potential for returning the queue after a session ends. This could be recovered afterwards.
	remainingQ := make(chan []*youtube.Video)
	queueShutDown <- remainingQ
	<-remainingQ
	vc.Disconnect()
	mediaReturnFinishChan <- guildID

}

func (m *mediaSession) stop() {
	m.stream.Stop()
	m.download.cancel()
	m.encode.Cleanup()
}

func (m *mediaSession) pause() bool {
	return m.stream.Pause()
}

func (m *mediaSession) resume() bool {
	return m.stream.Resume()
}

func (d *downloadSession) cancel() {
	d.Lock()
	d.cncl()
	d.Unlock()
}

func makePrettySongList(yts []*youtube.Video, currentSongPos time.Duration) string {
	var sb strings.Builder
	timeToNow := currentSongPos

	for i, v := range yts {
		fmt.Fprintf(&sb, "%d. %s | Playing in: %v\n", i+1, v.Title, timeToNow.Truncate(time.Second).String())
		timeToNow = timeToNow + v.Duration
	}
	return sb.String()
}

func newSongQueue(songChannel <-chan songRequest) (chan *youtube.Video, chan chan []*youtube.Video, chan chan []*youtube.Video, chan bool) {
	nextSong := make(chan *youtube.Video)
	inspectSongQueue := make(chan chan []*youtube.Video)
	queueShutDown := make(chan chan []*youtube.Video)
	firstSongWait := make(chan bool)
	go songQueue(songChannel, nextSong, inspectSongQueue, queueShutDown, firstSongWait)

	return nextSong, inspectSongQueue, queueShutDown, firstSongWait
}

func songQueue(ch <-chan songRequest, cc chan<- *youtube.Video, inspector <-chan chan []*youtube.Video, shutdown <-chan chan []*youtube.Video, firstSongWait chan<- bool) {
	client := youtube.Client{}
	first := true
	success := false

	var songQueue []*youtube.Video
	nullQ := make(chan<- *youtube.Video)
	var songChannel *chan<- *youtube.Video
	for {
		var nextSong *youtube.Video
		// This if statement nullifies the sending of songs to the player routine if there are no songs in the queue.
		// It sets the channel to a channel which blocks forever, and the song to be sent is nil.
		if len(songQueue) == 0 {
			songChannel = &nullQ
		} else {
			nextSong = songQueue[0]
			songChannel = &cc
		}

		select {
		case song := <-ch:
			func(url songRequest) {
				ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
				defer cancel()
				vid, err := client.GetVideoContext(ctx, song.URL)
				if err != nil {
					go trySend(song.returnChan, "Song not found.", standardTimeout)
					return
				}
				if vid.Duration <= time.Hour {
					songQueue = append(songQueue, vid)
					go trySend(song.returnChan, "Song added to queue.", standardTimeout)
					success = true
				} else {
					go trySend(song.returnChan, "Song too long.", standardTimeout)
				}
			}(song)
			if first {
				first = !first
				firstSongWait <- success
				close(firstSongWait)
			}
		case *songChannel <- nextSong:
			songQueue = songQueue[1:]
		case ret := <-inspector:
			// This is slightly confusing. We do this rather than just sending directly on the channel so that we avoid data races and also only copy when required.
			q := make([]*youtube.Video, len(songQueue))
			copy(q, songQueue)
			// This is a blocking send. The receiver must listen immediately or be put to death.
			ret <- q
		case sht := <-shutdown:
			sht <- songQueue
			return
		}
	}

}
