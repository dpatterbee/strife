package media

import (
	"errors"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Controller represents an active media controller.
type Controller struct {
	rch     chan Request
	session *discordgo.Session
	active  bool
}

//go:generate stringer -type=Action
type Action int

const (
	PLAY Action = iota
	PAUSE
	RESUME
	SKIP
	DISCONNECT
	INSPECT
)

// Request contains the fields required to communicate an intention to the media controller.
type Request struct {
	CommandType Action
	GuildID     string
	ChannelID   string
	CommandData string
	ReturnChan  chan string
}

// ErrNotActive is the error used when the Controller is not active
var ErrNotActive = errors.New("controller inactive")

// ErrServerBusy is the error used when the controller takes too long to accept a new command
var ErrServerBusy = errors.New("server busy")

// New returns a new media.Controller
func New(s *discordgo.Session) Controller {
	ch := make(chan Request)

	go controller(s, ch)

	return Controller{rch: ch, session: s, active: true}
}

// Send sends a Request to the Controller
func (c Controller) Send(guildID, channelID string, commandType Action,
	commandData string) (string, error) {

	timeout := time.NewTimer(time.Second)
	retchan := make(chan string)

	req := Request{
		CommandType: commandType,
		GuildID:     guildID,
		ChannelID:   channelID,
		CommandData: commandData,
		ReturnChan:  retchan,
	}

	if !c.active {
		return "", ErrNotActive
	}
	select {
	case c.rch <- req:
	case <-timeout.C:
		return "", ErrServerBusy
	}

	select {
	case s := <-retchan:
		timeout.Stop()
		return s, nil
	case <-timeout.C:
		return "Request sent", nil

	}

}
