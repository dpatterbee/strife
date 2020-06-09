package strife

import (
	"context"
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

var servers map[string]*server
var defaultCommands map[string]dfc
var client *firestore.Client
var ctx context.Context

// Run starts strife
func Run(args []string) int {

	ctx = context.Background()

	// Create Discord client
	dg, err := botFromArgs(args)
	if err != nil {
		log.Println("Error creating Discord session: ", err)
		return 1
	}
	defer dg.Close()

	// Create Firestore Client

	projectID := "strife-bot-123"
	log.Println("Creating Firestore client")
	client, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Println("Error creating Firestore Client", err)
		return 1
	}

	// Build commands and server map
	servers = buildServerData(ctx, dg)
	defaultCommands = makeDefaultCommands()

	// Add handlers to discord session
	log.Println("Adding handlers to discord session")
	dg.AddHandler(ready)
	dg.AddHandler(messageCreate)
	dg.AddHandler(guildRoleCreate)
	dg.AddHandler(guildRoleUpdate)

	// Open Discord connection
	log.Println("Opening discord connection")
	err = dg.Open()
	if err != nil {
		panic(err)
	}
	log.Println("Discord connection opened")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	fmt.Print("\n")
	return 0
}

func botFromArgs(args []string) (*discordgo.Session, error) {
	log.Println("Creating Discord Session")
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
	servers[guildID].Roles = getServerRoles(s, guildID)
	fmt.Println("popped")
	data := map[string]interface{}{
		"roles": servers[guildID].Roles,
	}

	_, err := updateServers(guildID, data)
	if err != nil {
		panic(err)
	}

}

func guildRoleUpdate(s *discordgo.Session, r *discordgo.GuildRoleUpdate) {
	guildID := r.GuildID
	servers[guildID].Roles = getServerRoles(s, guildID)
	fmt.Println("pooped")
	data := map[string]interface{}{
		"roles": servers[guildID].Roles,
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

	currentServer := servers[m.GuildID]

	if !strings.HasPrefix(m.Content, currentServer.Prefix) {
		return
	}
	content := strings.TrimPrefix(m.Content, currentServer.Prefix)

	splitContent := strings.Split(content, " ")
	content = strings.TrimPrefix(content, splitContent[0]+" ")

	var response string

	if isDefaultCommand(splitContent[0]) {
		requestedCommand := defaultCommands[splitContent[0]]
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
