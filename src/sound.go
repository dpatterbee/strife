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

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

const (
	pause = iota
	resume
	skip
	disconnect
)

var (
	reg  *regexp.Regexp
	once sync.Once
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
	sync.Mutex
}

type mediaCommandHolder struct {
	command mediaCommand
	result  chan error
}

type mediaCommand struct {
	commandType int
	commandData string
}

type downloadSession struct {
	process *os.Process
	sync.Mutex
}

func mediaController() {
	// This function will run perpertually, awaiting requests to create mediaHandlers for different servers.
	// It somehow returns a channel which is for mediaCommandHolders to be sent down.
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

func addRegexp() {
	reg = regexp.MustCompile(`^(https?)://(-\.)?([^\s/?\.#-]+\.?)+(/[^\s]*)?$`)
}

func isURL(s string) bool {
	once.Do(addRegexp)

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
		log.Println(err)
		return
	}
	d.Lock()
	d.process = ytdl.Process
	d.Unlock()

	err = ytdl.Wait()
	if err != nil {
		log.Println(err)
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
func soundHandler(s *discordgo.Session, guildID, channelID string, currentGuild *server, controlChannel <-chan int) {

	log.Println("Soundhandler not active, activating")

	// Set up voiceconnection
	//TODO: HANDLE THIS ERROR - IF YOU CAN'T JOIN THE CHANNEL IDK WHAT TO DO
	vc, _ := s.ChannelVoiceJoin(guildID, channelID, false, true)

	for {
		select {
		case song := <-currentGuild.songQueue:
			encode, download, err := makeSongSession(song.submission)
			streamingSession := newStreamingSession(encode, vc)
			if err != nil {
				log.Println(err)
				return
			}

			vc.Speaking(true)

			log.Println("started stream")

			streamingSession.Start()

			select {
			case err := <-streamingSession.done:
				vc.Speaking(false)
				log.Println("Finished Song; reason: ", err)
				continue
			case control := <-controlChannel:
				switch control {
				case pause:
					// TODO:
				case resume:
					// TODO:
				case skip:
					download.process.Kill()
					encode.Cleanup()
					// TODO:
				case disconnect:
					download.process.Kill()
					encode.Cleanup()
					streamingSession.stop <- true
					// TODO:
				}
			}
		default:
			//DO SOMeTHING
			currentGuild.Lock()

			currentGuild.mediaStatus.songPlaying = false

		}

	}

}

// cleanUp ends the external processes downloading and encoding the media being played
func (s *mediaSession) cleanUp() {

	s.Lock()
	if s.download != nil {
		s.download.process.Kill()
	}
	if s.encode != nil {
		s.encode.Cleanup()
	}
	s.Unlock()

	s = &mediaSession{}

}

func (s *mediaSession) disconnect() {
	s.stop <- true
}
