package strife

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	dgo "github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/strife/src/media"
	"github.com/dpatterbee/strife/src/messages"
	"github.com/dpatterbee/strife/src/store"
	"github.com/dpatterbee/strife/src/store/sqlite"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type strifeBot struct {
	defaultCommands map[string]botCommand
	mediaController media.Controller
	session         *dgo.Session
	store           store.Store
}

const stdTimeout = time.Millisecond * 500

var bot strifeBot

// Run starts strife
func Run() int {

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, NoColor: true})
	log.Logger = log.With().Caller().Logger()

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

	bot.session.Identify.Intents = dgo.IntentsGuilds | dgo.IntentsGuildMessages |
		dgo.IntentsGuildVoiceStates

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

	log.Info().Msg("Setup Complete")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	fmt.Print("\n")
	return 0
}

func (b *strifeBot) new() error {

	token := os.Getenv("TOKEN")
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

	b.mediaController = media.New(b.session)

	return nil
}

func (b *strifeBot) close() error {
	return b.session.Close()
}

func ready(s *dgo.Session, r *dgo.Ready) {

	guilds := r.Guilds
	for _, v := range guilds {
		_, err := bot.store.GetPrefix(v.ID)
		if err == sql.ErrNoRows {
			err := bot.store.SetPrefix(v.ID, "!")
			if err != nil {
				log.Error().Err(err).Msg("")
			}
		} else if err != nil {
			log.Error().Err(err).Msg("")
		}

		rs, err := s.GuildRoles(v.ID)
		if err != nil {
			log.Error().Err(err).Msg("")
		}
		for _, w := range rs {
			if in(w.Name, roles) {
				err = bot.store.AddRole(v.ID, w.Name, w.ID)
				if err != nil {
					log.Error().Err(err).Msg("")
				}
			}
		}
	}

	err := s.UpdateGameStatus(0, "dev")
	if err != nil {
		log.Error().Err(err).Msg("")
	}
}

func in(s string, ss []string) bool {
	for _, v := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func guildRoleCreate(_ *dgo.Session, r *dgo.GuildRoleCreate) {
	if !in(r.Role.Name, roles) {
		return
	}
	err := bot.store.AddRole(r.GuildID, r.Role.Name, r.Role.ID)
	if err != nil {
		log.Error().Err(err).Msg("")
	}
}

func guildRoleUpdate(_ *dgo.Session, r *dgo.GuildRoleUpdate) {
	if !in(r.Role.Name, roles) {
		return
	}
	err := bot.store.AddRole(r.GuildID, r.Role.Name, r.Role.ID)
	if err != nil {
		log.Error().Err(err).Msg("")
	}
}

func messageCreate(s *dgo.Session, m *dgo.MessageCreate) {

	if m.Author.ID == s.State.User.ID {
		return
	}

	prefix, err := bot.store.GetPrefix(m.GuildID)
	if err != nil {
		return
	}

	if !strings.HasPrefix(m.Content, prefix) {
		return
	}

	responder := messages.New(m.ChannelID)

	content := strings.TrimPrefix(m.Content, prefix)

	splitContent := strings.Split(content, " ")
	if len(splitContent) == 1 {
		content = ""
	} else {
		content = strings.TrimPrefix(content, splitContent[0]+" ")
	}

	commandConf := commandStuff{
		content:  content,
		response: responder,
	}

	if isDefaultCommand(splitContent[0]) {
		requestedCommand := bot.defaultCommands[splitContent[0]]
		var err error

		neededPermission := requestedCommand.permission
		commandFunc := requestedCommand.function

		if userPermissionLevel(s, m) >= neededPermission {
			err = commandFunc(s, commandConf, m)
		} else {
			responder.Set("Invalid Permission level")
		}

		if err != nil {
			responder.Set(err.Error())
		}
	} else {
		var err error
		response, err := bot.store.GetCommand(m.GuildID, splitContent[0])
		if err == sql.ErrNoRows {
			return
		}
		if err != nil {
			log.Error().Err(err).Msg("")
			return
		}

		responder.Set(response)
	}
}
