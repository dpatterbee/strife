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

type Controller interface {
	Send(
		guildID, channelID string, commandType Action,
		commandData string,
	) (string, error)
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

type Streamable interface {
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

func Search(m *discordgo.MessageCreate, c string) (string, error) {
	return searchClient.Search(m, c)
}
