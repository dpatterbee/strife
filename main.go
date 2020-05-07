package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"os/signal"
	"strings"
	"syscall"
)

func init() {
	flag.StringVar(&token, "t", "", "Bot Token")
	flag.Parse()
	if token == "" {
		reader := bufio.NewReader(os.Stdin)
		d, _ := reader.ReadString('\n')
		token = d[1:len(d)-2]
	}
	servers = make(map[string]*server)
	defaultCommands = makeDefaultCommands()

}

var token string
var servers map[string]*server
var defaultCommands map[string]dfc

type server struct {
	commands map[string]string
	name string
	prefix string
	roles map[string]int
}

func main() {
	if token == "" {
		fmt.Println("No token provided. Please use: scrabl -t <bot token>")
		return
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		fmt.Println("Error creating Discord session: ", err)
		return
	}
	defer dg.Close()

	guilds, _ := dg.UserGuilds(100, "", "")
	for _, v := range guilds {
		servers[v.ID] = &server{
			commands:make(map[string]string),
			name:v.Name,
			prefix:getGuildPrefix(v.ID),
			roles:getServerRoles(dg, v.ID),
		}
	}

	dg.AddHandler(ready)
	dg.AddHandler(messageCreate)

	err = dg.Open()
	if err != nil {
		panic(err)
	}

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	fmt.Print("\n")
}

func getGuildPrefix(id string) string {
	return "!"
}

func ready(s *discordgo.Session, event *discordgo.Ready) {
	err := s.UpdateStatus(0, "dev")
	if err != nil {
		fmt.Println(err)
	}
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {

	if m.Author.ID == s.State.User.ID {
		return
	}

	currentServer := servers[m.GuildID]

	if !strings.HasPrefix(m.Content, currentServer.prefix) {
		return
	}
	content := strings.TrimPrefix(m.Content, currentServer.prefix)

	splitContent := strings.Split(content, " ")
	content = strings.TrimPrefix(content, splitContent[0] + " ")

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
		var ok bool
		response, ok = currentServer.commands[splitContent[0]]
		if !ok {
			response = "Command doesn't exist!"
		}
	}

	if response != "" {
		test, err := s.ChannelMessageSend(m.ChannelID, response)
		if err != nil {
			panic(err)
		}
		fmt.Println(test)
	}

}

func getServerRoles(s *discordgo.Session, i string) map[string]int {
	e, _ := s.GuildRoles(i)

	m := make(map[string]int)

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