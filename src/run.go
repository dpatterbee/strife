package strife

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
)

type strifeBot struct {
	servers         map[string]*server
	defaultCommands map[string]dfc
	client          *firestore.Client
	session         *discordgo.Session
}

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
	defer bot.close()

	// Build commands and server map
	bot.servers = buildServerData(ctx, bot.session)
	bot.defaultCommands = makeDefaultCommands()

	// Add handlers to discord session
	log.Println("Adding handlers to discord session")
	bot.session.AddHandler(ready)
	bot.session.AddHandler(messageCreate)
	bot.session.AddHandler(guildRoleCreate)
	bot.session.AddHandler(guildRoleUpdate)

	// Open Discord connection
	log.Println("Opening discord connection")
	err = bot.session.Open()
	if err != nil {
		log.Println("Error opening discord connection", err)
		return 1
	}
	log.Println("Discord connection opened")

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

	if err := fl.Parse(args); err != nil {
		return err
	}

	if len(*token) == 0 {
		return errors.New("No Discord token provided")
	}

	if len(*projectID) == 0 {
		return errors.New("No Project ID provided")
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

func botFromArgs(args []string) (*discordgo.Session, error) {
	var token string
	fl := flag.NewFlagSet("strife", flag.ContinueOnError)

	fl.StringVar(&token, "t", "", "Bot Token")

	if err := fl.Parse(args); err != nil {
		return nil, err
	}

	if len(token) == 0 {
		return nil, fmt.Errorf("No token provided")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	return dg, nil
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
	content = strings.TrimPrefix(content, splitContent[0]+" ")

	var response string

	if isDefaultCommand(splitContent[0]) {
		requestedCommand := bot.defaultCommands[splitContent[0]]
		var err error

		neededPermission := requestedCommand.permission
		commandFunc := requestedCommand.function

		if userPermissionLevel(s, m) >= neededPermission {
			fmt.Println(userPermissionLevel(s, m), neededPermission)
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
		test, err := s.ChannelMessageSend(m.ChannelID, response)
		if err != nil {
			panic(err)
		}
		fmt.Println(test)
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