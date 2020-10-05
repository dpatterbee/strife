package strife

import (
	"bytes"
	"fmt"

	"github.com/Andreychik32/ytdl"
	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
)

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

func getUserVoiceChannel(sess *discordgo.Session, m *discordgo.MessageCreate) (string, error) {

	userID := m.Author.ID

	guild, err := sess.State.Guild(m.GuildID)
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
