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

type downloadSession struct {
	process *os.Process
	sync.Mutex
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
func soundHandler(guildID, channelID string) {
	currentGuild := bot.servers[guildID]

	var guildMediaSession mediaSession
	guildMediaSession.stop = make(chan bool)

	vc, err := bot.session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		log.Println(err)
		return
	}

	// Defer disconnection and mediaSession cleanup
	defer func() {
		vc.Disconnect()

		currentGuild.Lock()
		currentGuild.songPlaying = false
		currentGuild.songPlayingChannel = ""
		currentGuild.mediaSessions.cleanUp()
		currentGuild.mediaSessions = nil
		currentGuild.Unlock()
	}()

	currentGuild.Lock()
	currentGuild.mediaSessions = &guildMediaSession
	currentGuild.songPlayingChannel = channelID
	log.Println("Songqueue length =", len(currentGuild.songQueue))
	for {

		for len(currentGuild.songQueue) > 0 {
			var currentSong songURL
			currentSong, currentGuild.songQueue = currentGuild.songQueue[0], currentGuild.songQueue[1:]
			currentGuild.Unlock()

			encode, download, err := makeSongSession(currentSong.submission)
			if err != nil {
				log.Println(err)
				break
			}

			vc.Speaking(true)

			log.Println("started stream")

			streamingSession := newStream(encode, vc)

			guildMediaSession.Lock()
			guildMediaSession.download = download
			guildMediaSession.encode = encode
			guildMediaSession.stream = streamingSession
			guildMediaSession.Unlock()

			// Goroutine blocks here until song ends or commanded to disconnect
			select {
			case err = <-currentGuild.mediaSessions.stream.done:
			case <-currentGuild.mediaSessions.stop:
				return
			}
			log.Println("finished song; reason: ", err)

			guildMediaSession.cleanUp()

			currentGuild.Lock()
		}
		vc.Speaking(false)
		currentGuild.Unlock()

		// 10 seconds before bot disconnects to reduce churn if someone adds a song after previous has finished.
		time.Sleep(10 * time.Second)

		currentGuild.Lock()
		if len(currentGuild.songQueue) < 1 {
			break
		}

	}
	currentGuild.Unlock()
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
