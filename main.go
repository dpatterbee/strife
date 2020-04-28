package main

import (
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
	servers = make(map[string]*server)
	defaultCommands = makeDefaultCommands()

}

var token string
var servers map[string]*server
var defaultCommands map[string]func(*discordgo.MessageCreate, string) string

type server struct {
	commands map[string]string
	name string
	prefix string
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
		oo := defaultCommands[splitContent[0]]

		response = oo(m, content)
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