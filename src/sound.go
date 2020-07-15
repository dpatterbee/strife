package strife

import (
	"bytes"
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	"github.com/rylio/ytdl"
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

func getSound(s string) *dca.EncodeSession {

	client := ytdl.DefaultClient

	videoInfo, err := client.GetVideoInfo(ctx, s)
	if err != nil {
		panic(err)
	}

	var video bytes.Buffer

	err = client.Download(ctx, videoInfo, videoInfo.Formats[0], &video)
	if err != nil {
		panic(err)
	}

	encodeSess, err := dca.EncodeMem(&video, dca.StdEncodeOptions)

	return encodeSess
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

func soundHandler(guildID, channelID string) {
	currentGuild := bot.servers[guildID]

	vc, err := bot.session.ChannelVoiceJoin(guildID, channelID, false, true)
	if err != nil {
		log.Println(err)
		return
	}
	currentGuild.Lock()
	currentGuild.songPlayingChannel = channelID
	currentGuild.Unlock()

	log.Println("Songqueue length =", len(currentGuild.songQueue))
	for len(currentGuild.songQueue) > 0 {
		currentGuild.Lock()
		var currentSong songURL
		currentSong, currentGuild.songQueue = currentGuild.songQueue[0], currentGuild.songQueue[1:]
		currentGuild.Unlock()

		sound := getSound(currentSong.url)

		vc.Speaking(true)

		done := make(chan error)

		streamingSession := dca.NewStream(sound, vc, done)

		currentGuild.Lock()
		currentGuild.streamingSession = streamingSession
		currentGuild.Unlock()

		log.Println("started stream")
		err = <-done
		log.Println("finished song")
		if err != nil {
			log.Println(err)
		}

		currentGuild.Lock()
		currentGuild.streamingSession = nil
		currentGuild.Unlock()

		sound.Cleanup()
	}

	vc.Speaking(false)

	time.Sleep(10 * time.Second)

	vc.Disconnect()

	currentGuild.Lock()
	currentGuild.songPlaying = false
	currentGuild.Unlock()
}
