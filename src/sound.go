package strife

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
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
	stop     chan bool
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
	cancel context.CancelFunc
	sync.Mutex
}

// mediaControlRouter function runs perpetually, maintaining a pool of all currently active media sessions.
// It routes commands to the correct channel, creating a new media session if one is required to fulfill the request.
func mediaControlRouter(session *discordgo.Session, mediaCommandChannel chan mediaRequest) {

	type activeMediaChannel struct {
		songChannel    chan string
		controlChannel chan mediaCommand
	}

	type dyingMediaChannel struct {
		dependedOn bool
		dependency chan bool
	}

	// activeMediaChannels is a map of channels which are currently serving song requests.
	// dyingMediaChannels is a map of channels which have been instructed to shut down or have timed out, but have not yet completed their shutdown tasks.
	activeMediaChannels := make(map[string]activeMediaChannel)
	dyingMediaChannels := make(map[string]dyingMediaChannel)

	// These channels are used by guildSoundPlayer goroutines to inform this goroutine of their shutdown status.
	// mediaReturnChannel is used to inform that it has begun shutting down and that no more song or command requests should be forwarded to that goroutine.
	// mediaFiniChannel is used to inform that it has completed shutting down and that any waiting goroutines can be released.
	mediaReturnChannel := make(chan string)
	mediaFiniChannel := make(chan string)

	// This loops for the lifetime of the program, responding to messages sent on each channel.
	for {

		select {
		case elem := <-mediaCommandChannel:
			switch elem.commandType {
			case play:

				ch, ok := activeMediaChannels[elem.guildID]
				if !ok {
					controlChannel := make(chan mediaCommand)
					songChannel := make(chan string, 100)
					activeMediaChannels[elem.guildID] = activeMediaChannel{
						controlChannel: controlChannel,
						songChannel:    songChannel,
					}

					ch = activeMediaChannels[elem.guildID]
					// Checks if there exists a guildSoundPlayer goroutine which is currently dying, and creates the channel required so we can wait for the old goroutine to shut down.
					if _, ok := dyingMediaChannels[elem.guildID]; ok {
						dependencyChan := make(chan bool)
						dyingMediaChannels[elem.guildID] = dyingMediaChannel{dependedOn: true, dependency: dependencyChan}

						go guildSoundPlayer(
							session,
							elem.guildID,
							elem.channelID,
							controlChannel,
							songChannel,
							mediaReturnChannel,
							mediaFiniChannel,
							dependencyChan,
						)
					} else {
						go guildSoundPlayer(
							session,
							elem.guildID,
							elem.channelID,
							controlChannel,
							songChannel,
							mediaReturnChannel,
							mediaFiniChannel,
							nil,
						)
					}
				}

				result := "Song added to queue"

				select {
				case ch.songChannel <- elem.commandData:
				default:
					result = "Queue full, please try again later."
				}

				go trySend(elem.returnChannel, result, standardTimeout)

			case disconnect:
				mc, ok := activeMediaChannels[elem.guildID]

				if ok {
					//TODO: There's definitely a potential scenario where this never properly sends the disconnect signal and we end up with
					//a zombie goroutine holding onto a voiceconnection for a while.
					go func(mc activeMediaChannel) {
						timeout := time.NewTimer(10 * time.Minute)
						select {
						case mc.controlChannel <- mediaCommand{commandType: disconnect, returnChannel: elem.returnChannel}:
							timeout.Stop()
						case <-timeout.C:

						}
					}(mc)

					dyingMediaChannels[elem.guildID] = dyingMediaChannel{dependedOn: false, dependency: nil}
					delete(activeMediaChannels, elem.guildID)
				}

			default:
				mc, ok := activeMediaChannels[elem.guildID]
				go func() {
					timeout := time.NewTimer(standardTimeout)
					if ok {
						select {
						case mc.controlChannel <- mediaCommand{commandType: elem.commandType, returnChannel: elem.returnChannel}:
							timeout.Stop()
							return
						case <-timeout.C:
							go trySend(elem.returnChannel, "Serber bisi", standardTimeout)
							return
						}
					}

					go trySend(elem.returnChannel, "No media playing", standardTimeout)
				}()

			}
		case guildID := <-mediaReturnChannel:

			// When a guildSoundPlayer goroutine informs us that they are beginning to shut down, we close the communications channels and drain them,
			// then create an entry in our dyingMediaChannels map and remove from activeMediaChannels
			_, ok := activeMediaChannels[guildID]
			if ok {
				dyingMediaChannels[guildID] = dyingMediaChannel{}
				delete(activeMediaChannels, guildID)
			}
		case guildID := <-mediaFiniChannel:

			// When a guildSoundPlayer goroutine informs us that it has completed shutting down, if that shutdown has a dependency, we inform the dependent that
			// it is now free to go, and we remove it from the map and close the channel. Otherwise we simply remove it from the map.
			if pp, ok := dyingMediaChannels[guildID]; ok {
				if pp.dependedOn {
					go func(ch chan<- bool) {
						ch <- true
						close(ch)
					}(pp.dependency)
					delete(dyingMediaChannels, guildID)
				} else {
					delete(dyingMediaChannels, guildID)
				}
			}

		}

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
func streamSong(writePipe io.WriteCloser, s string, d *downloadSession) {

	client := youtube.Client{}
	video, err := client.GetVideo(s)
	if err != nil {
		log.Error().Err(err).Msg("")
		writePipe.Close()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	resp, err := client.GetStreamContext(ctx, video, &video.Formats[0])
	if err != nil {
		log.Error().Err(err).Msg("")
		cancel()
		writePipe.Close()
		return
	}
	defer resp.Body.Close()
	d.cancel = cancel
	d.Unlock()

	_, err = io.Copy(writePipe, resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("")
	}
	log.Info().Msg("Finished download.")
	writePipe.Close()
}

func makeSongSession(s string) (*dca.EncodeSession, *downloadSession, error) {

	bufPipe := bpipe.New()

	var d downloadSession

	d.Lock()
	go streamSong(bufPipe, s, &d)

	ss, err := dca.EncodeMem(bufPipe, dca.StdEncodeOptions)
	if err != nil {
		return nil, nil, err
	}

	return ss, &d, nil
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
	songChannel <-chan string,
	mediaReturnRequestChan, mediaReturnFinishChan chan<- string,
	previousInstanceWaitChan <-chan bool,
) {
	log.Info().Msg("Soundhandler not active, activating")

	if previousInstanceWaitChan != nil {
		<-previousInstanceWaitChan
	}

	nextSong := make(chan string)
	inspectSongQueue := make(chan chan []string)
	queueShutDown := make(chan chan []string)
	go songQueue(songChannel, nextSong, inspectSongQueue, queueShutDown)

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
		case song := <-nextSong:
			log.Info().
				Str("URL", song).
				Str("guildID", guildID).
				Msg("Playing Song")
			encode, download, err := makeSongSession(song)
			streamingSession := newStreamingSession(encode, vc)
			if err != nil {
				log.Error().Err(err).Msg("")
				return
			}

			vc.Speaking(true)

			log.Info().
				Str("guildID", guildID).
				Str("channelID", channelID).
				Msg("Starting Audio Stream")

			streamingSession.Start()

			// controlLoop should only be entered once it is possible to control the media ie. once the ffmpeg and youtube-dl sessions are up and running
		controlLoop:
			for {
				select {

				case err := <-streamingSession.done:
					vc.Speaking(false)
					if err == io.EOF {
						log.Info().Msg("Song Completed.")
					} else {
						log.Error().Err(err).Msg("Song Stopped")
					}
					break controlLoop

				case control := <-controlChannel:

					switch control.commandType {

					case pause:
						streamingSession.stop <- true
						go trySend(control.returnChannel, "Song paused.", standardTimeout)

					case resume:
						go streamingSession.stream()
						go trySend(control.returnChannel, "Song resumed.", standardTimeout)

					case skip:
						if !encode.Running() {
							go trySend(control.returnChannel, "Not yet.", standardTimeout)
							continue

						}
						go trySend(control.returnChannel, "Song skipped.", standardTimeout)
						streamingSession.skip <- true

						download.Lock()
						download.cancel()
						download.Unlock()
						encode.Cleanup()

						break controlLoop

					case disconnect:
						if !encode.Running() {
							go trySend(control.returnChannel, "Not yet.", standardTimeout)
							continue

						}
						go trySend(control.returnChannel, "Goodbye.", standardTimeout)
						streamingSession.stop <- true
						download.Lock()
						download.cancel()
						download.Unlock()
						encode.Cleanup()
						break mainLoop

					case inspect:
						qch := make(chan []string)
						inspectSongQueue <- qch
						q := <-qch
						go trySend(control.returnChannel, fmt.Sprint(q), standardTimeout)
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
	remainingQ := make(chan []string)
	queueShutDown <- remainingQ
	<-remainingQ
	vc.Disconnect()
	mediaReturnFinishChan <- guildID

}

func songQueue(ch <-chan string, cc chan<- string, inspector <-chan chan []string, shutdown <-chan chan []string) {

	var songQueue []string
	for {
		if len(songQueue) == 0 {
			select {
			case song := <-ch:
				songQueue = append(songQueue, song)
			case ret := <-inspector:
				// This is slightly confusing. We do this rather than just sending directly on the channel so that we avoid data races and also only copy when required.
				q := make([]string, len(songQueue))
				copy(q, songQueue)
				// This is a blocking send. The receiver must listen immediately or be put to death.
				ret <- q
			case sht := <-shutdown:
				sht <- songQueue
				return
			}
		} else {
			select {
			case song := <-ch:
				songQueue = append(songQueue, song)
			case cc <- songQueue[0]:
				songQueue = songQueue[1:]
			case ret := <-inspector:
				// This is slightly confusing. We do this rather than just sending directly on the channel so that we avoid data races and also only copy when required.
				q := make([]string, len(songQueue))
				copy(q, songQueue)
				// This is a blocking send. The receiver must listen immediately or be put to death.
				ret <- q
			case sht := <-shutdown:
				sht <- songQueue
				return
			}
		}
	}

}
