package main

import (
	"bufio"
	"cloud.google.com/go/firestore"
	"context"
	"flag"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func init() {
	flag.StringVar(&token, "t", "", "Bot Token")
	flag.Parse()
	ctx = context.Background()
	if token == "" {
		reader := bufio.NewReader(os.Stdin)
		d, _ := reader.ReadString('\n')
		token = d
	}
	servers = make(map[string]*server)
	defaultCommands = makeDefaultCommands()
}

var token string
var servers map[string]*server
var defaultCommands map[string]dfc
var client *firestore.Client
var ctx context.Context

func main() {
	if token == "" {
		fmt.Println("No token provided. Please use: scrabl -t <bot token>")
		return
	}

	log.Println("Creating discord session")
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}
	defer dg.Close()

	projectID := "strife-bot-123"
	log.Println("Creating Firestore client")
	client, err = firestore.NewClient(ctx, projectID)
	if err != nil {
		panic(err)
	}

	servers = buildServerData(dg, ctx)

	log.Println("Adding handlers to discord session")
	dg.AddHandler(ready)
	dg.AddHandler(messageCreate)
	dg.AddHandler(guildRoleCreate)
	dg.AddHandler(guildRoleUpdate)

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
