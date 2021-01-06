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

type mediaChannel struct {
	songChannel    chan string
	controlChannel chan mediaCommand
}

// mediaControlRouter function runs perpetually, maintaining a pool of all currently active media sessions.
// It routes commands to the correct channel, creating a new media session if one is required to fulfill the request.
func mediaControlRouter(session *discordgo.Session, mediaCommandChannel chan mediaRequest) {

	// This is a map of guildIDs to mediaChannels. It contains all currently active media sessions.
	mediaChannels := make(map[string]mediaChannel)

	// This channel is used by a media channel to inform the router that it no longer exists and can be removed from the map
	// As there is only one instance of this function running there should never be a race condition here
	mediaReturnChannel := make(chan string)

	// This loops for the lifetime of the program, responding to messages sent on each channel.
	for {

		select {
		case elem := <-mediaCommandChannel:
			switch elem.commandType {
			case play:

				ch, ok := mediaChannels[elem.guildID]
				if !ok {
					controlChannel := make(chan mediaCommand, 10)
					songChannel := make(chan string, 100) // This serves as a song queue. Which in hindsight is pretty terrible.
					mediaChannels[elem.guildID] = mediaChannel{
						controlChannel: controlChannel,
						songChannel:    songChannel,
					}

					ch = mediaChannels[elem.guildID]
					go soundHandler(session, elem.guildID, elem.channelID, controlChannel, songChannel, mediaReturnChannel)
				}

				result := "Song added to queue"

				// Perform operations in separate goroutine to avoid blocking
				go func(returnChannel, mediaChannel chan string, commandData string) {
					select {
					case mediaChannel <- commandData:
					default:
						result = "Queue full, please try again later."
					}

					go trySend(returnChannel, result, standardTimeout)

				}(elem.returnChannel, ch.songChannel, elem.commandData)

			default:
				mc, ok := mediaChannels[elem.guildID]
				go func(ch chan mediaCommand) {
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
				}(mc.controlChannel)

			}
		case guildID := <-mediaReturnChannel:
			// Ensure the song channel is drained for potential gc reasons
			for range mediaChannels[guildID].songChannel {
			}
			delete(mediaChannels, guildID)

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
	d.Lock()
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

// soundHandler runs while a server has a queue of songs to be played.
// It loops over the queue of songs and plays them in order, exiting once it has drained the list
func soundHandler(s *discordgo.Session, guildID, channelID string, controlChannel <-chan mediaCommand, songChannel <-chan string, mediaReturnChannel chan<- string) {

	log.Println("Soundhandler not active, activating")

	// Set up voiceconnection
	//TODO: HANDLE THIS ERROR - IF YOU CAN'T JOIN THE CHANNEL IDK WHAT TO DO
	vc, _ := s.ChannelVoiceJoin(guildID, channelID, false, true)

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
						// TODO:
						go trySend(control.returnChannel, "Song paused.", standardTimeout)

					case resume:
						go streamingSession.stream()
						// TODO:
						go trySend(control.returnChannel, "Song resumed.", standardTimeout)

					case skip:
						streamingSession.skip <- true
						go trySend(control.returnChannel, "Song skipped.", standardTimeout)
						encode.Cleanup()
						download.process.Kill()
						break controlLoop
						// TODO:

					case disconnect:
						download.process.Kill()
						encode.Cleanup()
						streamingSession.stop <- true

						vc.Disconnect()
						mediaReturnChannel <- guildID
						go trySend(control.returnChannel, "Goodbye.", standardTimeout)
						return
						// TODO:
					}
				}
			}

		case <-disconnectTimer.C:
			//DO SOMeTHING
			vc.Disconnect()

			mediaReturnChannel <- guildID
			return

		}

	}

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
