package strife

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	dgo "github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/strife/store"
	"github.com/dpatterbee/strife/store/sqlite"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type strifeBot struct {
	defaultCommands        map[string]botCommand
	mediaControllerChannel chan mediaRequest
	session                *dgo.Session
	store                  store.Store
}

const stdTimeout = time.Millisecond * 500

var bot strifeBot
var ctx context.Context

// Run starts strife
func Run() int {

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	log.Logger = log.With().Caller().Logger()

	ctx = context.Background()

	// Create bot discord session and database store
	err := bot.new()
	if err != nil {
		log.Error().Err(err).Msg("Error creating bot from args.")
		return 1
	}

	// Build commands and server map
	bot.defaultCommands = makeDefaultCommands()

	// Add event handlers to discordgo session
	// https://discord.com/developers/docs/topics/gateway#commands-and-events-gateway-events
	log.Info().Msg("Adding event handlers to discordgo session")
	bot.session.AddHandler(ready)
	bot.session.AddHandler(messageCreate)
	bot.session.AddHandler(guildRoleCreate)
	bot.session.AddHandler(guildRoleUpdate)

	bot.session.Identify.Intents = nil

	// Open Discord connection
	log.Info().Msg("Opening discord connection")
	err = bot.session.Open()
	if err != nil {
		log.Error().Err(err).Msg("")
		return 1
	}

	// defer close
	defer func(bot *strifeBot) {
		err := bot.close()
		if err != nil {
			log.Error().Err(err).Msg("")
		}
	}(&bot)

	log.Info().Msg("Discord connection opened")

	bot.mediaControllerChannel = createMainMediaController(bot.session)

	log.Info().Msg("Setup Complete")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	fmt.Print("\n")
	return 0
}

func (b *strifeBot) new() error {

	credentials := struct {
		Token string
	}{}

	dat, err := os.ReadFile("./creds.yml")
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(dat, &credentials)
	if err != nil {
		return err
	}

	token := credentials.Token
	if len(token) == 0 {
		return errors.New("no Discord token provided")
	}

	log.Info().Msg("Creating Discord Session")
	dg, err := dgo.New("Bot " + token)
	if err != nil {
		return fmt.Errorf("error creating discord session: %v", err)
	}
	b.session = dg

	log.Info().Msg("Getting database")
	b.store = sqlite.New()

	return nil
}

func (b *strifeBot) close() error {
	return b.session.Close()
}

func ready(s *dgo.Session, _ *dgo.Ready) {
	err := s.UpdateStatus(0, "dev")
	if err != nil {
		log.Error().Err(err).Msg("")
	}
}

func guildRoleCreate(s *dgo.Session, r *dgo.GuildRoleCreate) {
	guildID := r.GuildID
	bot.servers[guildID].Roles = getServerRoles(s, guildID)

	data := map[string]interface{}{
		"roles": bot.servers[guildID].Roles,
	}

	_, err := updateServers(guildID, data)
	if err != nil {
		log.Error().Err(err).Msg("")
	}

}

func guildRoleUpdate(s *dgo.Session, r *dgo.GuildRoleUpdate) {
	guildID := r.GuildID
	bot.servers[guildID].Roles = getServerRoles(s, guildID)

	data := map[string]interface{}{
		"roles": bot.servers[guildID].Roles,
	}

	_, err := updateServers(guildID, data)
	if err != nil {
		log.Error().Err(err).Msg("")
	}
}

func messageCreate(s *dgo.Session, m *dgo.MessageCreate) {

	if m.Author.ID == s.State.User.ID {
		return
	}

	currentServer := bot.servers[m.GuildID]

	if !strings.HasPrefix(m.Content, currentServer.Prefix) {
		return
	}
	content := strings.TrimPrefix(m.Content, currentServer.Prefix)

	splitContent := strings.Split(content, " ")
	if len(splitContent) == 1 {
		content = ""
	} else {
		content = strings.TrimPrefix(content, splitContent[0]+" ")
	}

	var response string

	if isDefaultCommand(splitContent[0]) {
		requestedCommand := bot.defaultCommands[splitContent[0]]
		var err error

		neededPermission := requestedCommand.permission
		commandFunc := requestedCommand.function

		if userPermissionLevel(s, m) >= neededPermission {
			response, err = commandFunc(s, m, content)
		} else {
			response = "Invalid Permission level"
		}

		if err != nil {
			response = err.Error()
		}
	} else {
		var err error
		response, err = bot.store.GetCommand(m.GuildID, splitContent[0])
		if err != nil {
			log.Error().Err(err).Msg("")
		}
	}

	if response == "" {
		response = "Command not found"
	}

	response = "**" + response + "**"
	message, err := s.ChannelMessageSend(m.ChannelID, response)
	if err != nil {
		log.Error().
			Err(err).
			Str("msg", message.ContentWithMentionsReplaced()).
			Str("author", message.Author.String()).
			Str("channelID", message.ChannelID).
			Msg("")
	}
	log.Info().
		Str("msg", message.ContentWithMentionsReplaced()).
		Str("author", message.Author.String()).
		Str("channelID", message.ChannelID).
		Msg("")

}

func getServerRoles(s *dgo.Session, i string) (map[string]int64, error) {
	e, err := s.GuildRoles(i)
	if err != nil {
		return nil, err
	}

	m := make(map[string]int64)

	for _, v := range e {
		if v.Name == "botuser" {
			m[v.ID] = botuser
		}
		if v.Name == "botdj" {
			m[v.ID] = botdj
		}
		if v.Name == "botmoderator" {
			m[v.ID] = botmoderator
		}
		if v.Name == "botadmin" {
			m[v.ID] = botadmin
		}
	}

	return m, nil
}

// trySend attempts to send "data" on "channel", timing out after "timeoutDuration".
func trySend(channel chan string, data string, timeoutDuration time.Duration) {
	// this will sure lend itself to generics when the time comes.
	timeout := time.NewTimer(timeoutDuration)

	select {
	case channel <- data:
		timeout.Stop()
	case <-timeout.C:
		return
	}
}

func createMainMediaController(sess *dgo.Session) chan mediaRequest {
	ch := make(chan mediaRequest)

	go mediaControlRouter(sess, ch)

	return ch
}
