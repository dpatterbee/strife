package strife

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
	"gopkg.in/yaml.v2"
)

type strifeBot struct {
	servers                map[string]*server
	defaultCommands        map[string]botCommand
	mediaControllerChannel chan mediaRequest
	client                 *firestore.Client
	session                *discordgo.Session
}

const standardTimeout = time.Millisecond * 500

var bot strifeBot
var ctx context.Context

// Run starts strife
func Run(args []string) int {

	ctx = context.Background()

	// Create bot discord session and firestore client
	err := bot.fromArgs(args)
	if err != nil {
		log.Println("Setup Error:", err)
		return 1
	}

	// Build commands and server map
	bot.servers = buildServerData(ctx, bot.session)
	bot.defaultCommands = makeDefaultCommands()

	// Add event handlers to discordgo session https://discord.com/developers/docs/topics/gateway#commands-and-events-gateway-events
	log.Println("Adding event handlers to discordgo session")
	bot.session.AddHandler(ready)
	bot.session.AddHandler(messageCreate)
	bot.session.AddHandler(guildRoleCreate)
	bot.session.AddHandler(guildRoleUpdate)

	bot.session.Identify.Intents = nil

	// Open Discord connection
	log.Println("Opening discord connection")
	err = bot.session.Open()
	if err != nil {
		log.Println("Error opening discord connection", err)
		return 1
	}
	defer bot.close()
	log.Println("Discord connection opened")

	bot.mediaControllerChannel = createMainMediaController(bot.session)

	log.Println("Setup Complete")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	fmt.Print("\n")
	return 0
}

func (b *strifeBot) fromArgs(args []string) error {

	fl := flag.NewFlagSet("strife", flag.ContinueOnError)

	token := fl.String("t", "", "Discord Bot Token")
	projectID := fl.String("p", "", "Firestore Project ID")

	tod := struct {
		Token string
		ID    string
	}{}

	dat, err := ioutil.ReadFile("./creds.yml")
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(dat, &tod)
	if err != nil {
		return err
	}

	if err := fl.Parse(args); err != nil {
		return err
	}

	if len(*token) == 0 {
		tok := os.Getenv("DISCORD_TOKEN")
		if tok == "" {
			tok = tod.Token
			if tok == "" {
				return errors.New("No Discord token provided")
			}
		}
		token = &tok
	}

	if len(*projectID) == 0 {
		proji := os.Getenv("PROJECT_ID")
		if proji == "" {
			proji = tod.ID
			if proji == "" {
				return errors.New("No Project ID provided")
			}
		}
		projectID = &proji
	}

	log.Println("Creating Discord Session")
	dg, err := discordgo.New("Bot " + *token)
	if err != nil {
		return fmt.Errorf("Error creating discord session: %v", err)
	}
	b.session = dg

	log.Println("Creating Firestore Client")
	client, err := firestore.NewClient(ctx, *projectID)
	if err != nil {
		return fmt.Errorf("Error creating Firestore Client: %v", err)
	}
	b.client = client

	return nil
}

func (b *strifeBot) close() {
	b.session.Close()
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	err := s.UpdateStatus(0, "dev")
	if err != nil {
		fmt.Println(err)
	}
}

func guildRoleCreate(s *discordgo.Session, r *discordgo.GuildRoleCreate) {
	guildID := r.GuildID
	bot.servers[guildID].Roles = getServerRoles(s, guildID)

	data := map[string]interface{}{
		"roles": bot.servers[guildID].Roles,
	}

	_, err := updateServers(guildID, data)
	if err != nil {
		panic(err)
	}

}

func guildRoleUpdate(s *discordgo.Session, r *discordgo.GuildRoleUpdate) {
	guildID := r.GuildID
	bot.servers[guildID].Roles = getServerRoles(s, guildID)

	data := map[string]interface{}{
		"roles": bot.servers[guildID].Roles,
	}

	_, err := updateServers(guildID, data)
	if err != nil {
		panic(err)
	}
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

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
		response, _ = currentServer.Commands[splitContent[0]]
	}

	if response != "" {
		response = "**" + response + "**"
		message, err := s.ChannelMessageSend(m.ChannelID, response)
		if err != nil {
			panic(err)
		}
		log.Println(message.ContentWithMentionsReplaced(), message.Author, message.ChannelID, message.GuildID)
	}

}

func getServerRoles(s *discordgo.Session, i string) map[string]int64 {
	e, _ := s.GuildRoles(i)

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

	return m
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

func createMainMediaController(sess *discordgo.Session) chan mediaRequest {
	ch := make(chan mediaRequest)

	go mediaControlRouter(sess, ch)

	return ch
}
