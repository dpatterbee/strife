package strife

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

const (
	play = iota
	pause
	resume
	skip
	disconnect
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
	process *os.Process
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
					songChannel := make(chan string, 100) // This serves as a song queue. Which in hindsight is pretty terrible as I do not believe it can be inspected. TODO: Fix this.
					activeMediaChannels[elem.guildID] = activeMediaChannel{
						controlChannel: controlChannel,
						songChannel:    songChannel,
					}

					ch = activeMediaChannels[elem.guildID]
					// Checks if there exists a guildSoundPlayer goroutine which is currently dying, and creates the channel required so we can wait for the old goroutine to shut down.
					if j, ok := dyingMediaChannels[elem.guildID]; ok {
						dependencyChan := make(chan bool)
						j.dependedOn = true
						j.dependency = dependencyChan

						go guildSoundPlayer(session, elem.guildID, elem.channelID, controlChannel, songChannel, mediaReturnChannel, mediaFiniChannel, dependencyChan)
					} else {
						go guildSoundPlayer(session, elem.guildID, elem.channelID, controlChannel, songChannel, mediaReturnChannel, mediaFiniChannel, nil)
					}
				}

				result := "Song added to queue"

				// Perform operations in separate goroutine to avoid blocking
				select {
				case ch.songChannel <- elem.commandData:
				default:
					result = "Queue full, please try again later."
				}

				go trySend(elem.returnChannel, result, standardTimeout)

			case disconnect:
				mc, ok := activeMediaChannels[elem.guildID]

				if ok {
					go func(mc activeMediaChannel) {
						timeout := time.NewTimer(10 * time.Minute)
						select {
						case mc.controlChannel <- mediaCommand{commandType: disconnect, returnChannel: elem.returnChannel}:
							timeout.Stop()
						case <-timeout.C:

						}
					}(mc)

					dyingMediaChannels[elem.guildID] = dyingMediaChannel{}
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
func streamSong(writePipe io.Writer, s string, d *downloadSession) {
	ytdlArgs := []string{
		"-o", "-",
	}

	ytdlArgs = append(ytdlArgs, s)

	ytdl := exec.Command("youtube-dl", ytdlArgs...)

	ytdl.Stdout = writePipe

	err := ytdl.Start()
	if err != nil {
		log.Println("ytdl start", err)
		return
	}
	d.process = ytdl.Process
	d.Unlock()

	err = ytdl.Wait()
	if err != nil {
		log.Println("ytdl wait", err)
	}
}

func makeSongSession(s string) (*dca.EncodeSession, *downloadSession, error) {

	readPipe, writePipe := io.Pipe()

	bufferedReadPipe := bufio.NewReader(readPipe)
	bufferedWritePipe := bufio.NewWriter(writePipe)

	var d downloadSession

	d.Lock()
	go streamSong(bufferedWritePipe, s, &d)

	ss, err := dca.EncodeMem(bufferedReadPipe, dca.StdEncodeOptions)
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
	s *discordgo.Session,
	guildID, channelID string,
	controlChannel <-chan mediaCommand,
	songChannel <-chan string,
	mediaReturnRequestChan, mediaReturnFinishChan chan<- string,
	previousrunningthingchan <-chan bool,
) {

	if previousrunningthingchan != nil {
		<-previousrunningthingchan
	}

	songQueue := struct {
		songQ []string
		sync.Mutex
	}{
		songQ: make([]string, 100),
	}

	log.Println("Soundhandler not active, activating")

	// Set up voiceconnection
	// If a previous connection is active, this will error, and we will retry every 200ms 20 times to attempt to join.
	//TODO: Doesn't work
	vc, err := s.ChannelVoiceJoin(guildID, channelID, false, true)
	for i := 0; err != nil && i < 20; i++ {
		time.Sleep(200 * time.Millisecond)
		vc, err = s.ChannelVoiceJoin(guildID, channelID, false, true)
	}

	// If after 20 retries we still have not achieved a connection we will close this goroutine and tell the router that we are closed.
	if err != nil {
		mediaReturnRequestChan <- guildID
		log.Println("Couldn't initialise voice connection")
		return
	}

	disconnectTimer := time.NewTimer(5 * time.Second)

	for {
		if !disconnectTimer.Stop() {
			<-disconnectTimer.C
		}
		disconnectTimer.Reset(5 * time.Second)
		select {
		case song := <-songChannel:
			log.Println("song link:", song)
			encode, download, err := makeSongSession(song)
			streamingSession := newStreamingSession(encode, vc)
			if err != nil {
				log.Println("streamingSession", err)
				return
			}

			vc.Speaking(true)

			log.Println("started stream")

			streamingSession.Start()

			// controlLoop should only be entered once it is possible to control the media ie. once the ffmpeg and youtube-dl sessions are up and running
		controlLoop:
			for {
				select {

				case err := <-streamingSession.done:
					vc.Speaking(false)
					log.Println("Finished Song; reason: ", err)
					break

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
						download.process.Kill()
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
						download.process.Kill()
						download.Unlock()
						encode.Cleanup()

						vc.Disconnect()
						mediaReturnFinishChan <- guildID
						return
					}
				}
			}

		case <-disconnectTimer.C:
			select {
			// catch any song coming in between the timer expiring and the media return channel catching the controlling loop
			case song := <-songChannel:
				songQueue.songQ = append(songQueue.songQ, song)
			case mediaReturnRequestChan <- guildID:
			}
			//DO SOMeTHING
			vc.Disconnect()
			mediaReturnFinishChan <- guildID
			return

		}

	}

}

func c(e *dca.EncodeSession) error {
	err := e.Stop()
	if err != nil {
		return err
	}
	e.Cleanup()
	return nil
}

// cleanUp ends the external processes downloading and encoding the media being played
func (s *mediaSession) cleanUp() {

	if s.download != nil {
		s.download.process.Kill()
	}
	if s.encode != nil {
		s.encode.Cleanup()
	}

	s = &mediaSession{}

}

func (s *mediaSession) disconnect() {
	s.stop <- true
}
