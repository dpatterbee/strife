package messages

import (
	"errors"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

type Message struct {
	m         *discordgo.Message
	session   *discordgo.Session
	messageID string
	isSent    bool
	channel   string

	sync.Mutex
}

func New(session *discordgo.Session, channelID string) (*Message, error) {
	if session == nil {
		return nil, errors.New("missing session")
	}
	if channelID == "" {
		return nil, errors.New("missing channelID")
	}
	return &Message{
		channel: channelID,
		session: session,
	}, nil
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

// SetRaw sends message s on Message.channel if Message.isSent is not set, or edits the message that
// has been sent if it is set.
func (m *Message) SetRaw(s string) error {
	errChan := make(chan error)

	go m.setRawAsync(s, errChan)

	return <-errChan
}

// SetRaw sends message s on Message.channel if Message.isSent is not set, or edits the message that
// has been sent if it is set.
func (m *Message) SetRawAsync(s string) {
	go m.setRawAsync(s, nil)
}

// SetRaw sends message s on Message.channel if Message.isSent is not set, or edits the message that
// has been sent if it is set.
func (m *Message) SetRawBytesAsync(b []byte) {
	go m.setRawAsync(string(b), nil)
}

func (m *Message) setRawAsync(s string, errchan chan<- error) {
	m.Lock()
	defer m.Unlock()

	if !m.isSent {
		message, err := m.session.ChannelMessageSend(m.channel, s)
		if err != nil {
			logMessageWithError(message, err)
			if errchan != nil {
				errchan <- err
			}
			return
		}
		logMessage(message)
		m.isSent = true
		m.m = message
		return
	}

	message, err := m.session.ChannelMessageEdit(m.channel, m.m.ID, s)
	if err != nil {
		logMessageWithError(message, err)
		if errchan != nil {
			errchan <- err
		}
		return
	}
	logMessage(message)
}

// SetRawBytes sends message b on Message.channel if Message.isSent is not set, or edits the message that
// has been sent if it is set.
func (m *Message) SetRawBytes(b []byte) error {
	return m.SetRaw(string(b))
}

// Set sends message s on Message.channel if Message.isSent is not set, or edits the message that
// has been sent if it is set.
// Prepends and appends "**" to s
func (m *Message) Set(s string) error {
	if len(s) == 0 {
		return errors.New("empty message")
	}
	return m.SetRaw("**" + s + "**")
}

// SetBytes sends message b on Message.channel if Message.isSent is not set, or edits the message that
// has been sent if it is set.
// Prepends and appends "**" to s
func (m *Message) SetBytes(b []byte) error {
	if len(b) == 0 {
		return errors.New("empty message")
	}
	return m.SetRawBytes(append([]byte("**"), append(b, []byte("**")...)...))
}
