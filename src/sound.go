package strife

import (
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

type songURL struct {
	location  string
	url       string
	requester string
}

func parseURL(m *discordgo.MessageCreate, s string) (songURL, error) {
	var loc songURL

	loc.url = s
	loc.location = "youtube"
	loc.requester = m.Author.ID

	return loc, nil
}

// streamSong uses youtube-dl to download the song and pipe the stream of data to w
func streamSong(w *io.PipeWriter, s string) {
	ytdlArgs := []string{
		"-o", "-",
	}

	ytdlArgs = append(ytdlArgs, s)

	ytdl := exec.Command("youtube-dl", ytdlArgs...)

	ytdl.Stdout = w

	err := ytdl.Start()
	if err != nil {
		log.Println(err)
	}

	err = ytdl.Wait()
	if err != nil {
		log.Println(err)
	}
}

func makeSongSession(s string) (*dca.EncodeSession, error) {

	r, w := io.Pipe()

	go streamSong(w, s)

	ss, err := dca.EncodeMem(r, dca.StdEncodeOptions)
	if err != nil {
		return nil, err
	}

	return ss, nil
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

	vc, err := bot.session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		log.Println(err)
		return
	}
	currentGuild.Lock()
	currentGuild.songPlayingChannel = channelID
	log.Println("Songqueue length =", len(currentGuild.songQueue))
	for {

		for len(currentGuild.songQueue) > 0 {
			var currentSong songURL
			currentSong, currentGuild.songQueue = currentGuild.songQueue[0], currentGuild.songQueue[1:]
			currentGuild.Unlock()

			sound, err := makeSongSession(currentSong.url)
			if err != nil {
				log.Println(err)
				break
			}

			vc.Speaking(true)

			done := make(chan error)

			log.Println("started stream")

			streamingSession := newStream(sound, vc)
			currentGuild.Lock()
			currentGuild.streamingSession = streamingSession
			currentGuild.Unlock()

			select {
			case <-currentGuild.songStopper:
				err = fmt.Errorf("User interrupt")
			case err = <-done:
			}
			log.Println("finished song; reason: ", err)

			currentGuild.Lock()
			currentGuild.streamingSession = nil
			currentGuild.Unlock()
			sound.Cleanup()

			currentGuild.Lock()
		}
		vc.Speaking(false)
		currentGuild.Unlock()

		time.Sleep(10 * time.Second)

		currentGuild.Lock()
		if len(currentGuild.songQueue) < 1 {
			break
		}

	}
	vc.Disconnect()

	currentGuild.songPlaying = false
	currentGuild.songPlayingChannel = ""
	currentGuild.streamingSession = nil
	currentGuild.Unlock()
}
