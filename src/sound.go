package strife

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/bwmarrin/discordgo"
	"github.com/jonas747/dca"
	"github.com/rylio/ytdl"
)

func getSound(s string) dca.EncodeSession {

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

	cmd := exec.Command("ffmpeg", "-i", "pipe:0", "-c", "copy", "-f", "adts", "pipe:1")
	cmd.Stdin = &video
	var out bytes.Buffer
	cmd.Stdout = &out
	err = cmd.Run()
	if err != nil {
		panic(err)
	}

	encodeSess, err := dca.EncodeMem(&out, dca.StdEncodeOptions)

	return *encodeSess
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
