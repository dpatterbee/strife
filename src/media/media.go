package media

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/strife/src/search"
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

type streamable interface {
	Stream(context.Context) (data io.ReadCloser, duration int64, err error)
	Title() string
	Duration() time.Duration
}

// ErrNotActive is the error used when the Controller is not active
var ErrNotActive = errors.New("controller inactive")

// ErrServerBusy is the error used when the controller takes too long to accept a new command
var ErrServerBusy = errors.New("server busy")

var searchClient *search.Client

func init() {
	var err error
	apiKey := os.Getenv("YOUTUBE_API_TOKEN")
	if len(apiKey) == 0 {
		log.Fatalln("no Youtube Api token provided")
	}
	searchClient, err = search.NewClient(apiKey)
	if err != nil {
		log.Fatalf("error creating youtube search client: %v", err)
	}
}

// New returns a new media.Controller
func New(s *discordgo.Session) Controller {
	ch := make(chan Request)

	go controller(s, ch)

	return Controller{rch: ch, session: s, active: true}
}

func Search(m *discordgo.MessageCreate, c string) (string, error) {
	return searchClient.Search(m, c)
}

// Send sends a Request to the Controller
func (c Controller) Send(guildID, channelID string, commandType Action,
	commandData string) (string, error) {

	timeout := time.NewTimer(5 * time.Second)
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
