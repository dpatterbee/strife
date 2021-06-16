package messages

import (
	"errors"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

// Message represents a message that has or will be sent to a text channel.
type Message struct {
	m         *discordgo.Message
	messageID string
	isSent    bool
	channel   string

	session *discordgo.Session

	sync.Mutex
}

func New(session *discordgo.Session, channel string) Message {
	return Message{
		session: session,
		channel: channel,
	}
}

func logMessageWithError(message *discordgo.Message, err error) {
	log.Error().
		Err(err).
		Str("msg", message.ContentWithMentionsReplaced()).
		Str("author", message.Author.String()).
		Str("channelID", message.ChannelID).
		Msg("")
}

func logMessage(message *discordgo.Message) {
	log.Info().
		Str("msg", message.ContentWithMentionsReplaced()).
		Str("author", message.Author.String()).
		Str("channelID", message.ChannelID).
		Msg("")
}

func (m *Message) Delete() error {
	m.Lock()
	defer m.Unlock()

	if !m.isSent {
		return errors.New("message not sent")
	}

	return m.session.ChannelMessageDelete(m.channel, m.m.ID)

}
