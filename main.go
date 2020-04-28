package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"cloud.google.com/go/datastore"
	"time"
)

type Task struct {
	Category        string
	Done            bool
	Priority        float64
	Description     string `datastore:",noindex"`
	PercentComplete float64
	Created         time.Time
}

func init() {
	flag.StringVar(&token, "t", "", "Bot Token")
	flag.Parse()
	commands = make(map[string]map[string]string)
}

var token string
var commands map[string]map[string]string

func main() {
	ctx := context.Background()
	projectID := "temporal-storm-273719"

	client, err := datastore.NewClient(ctx, projectID)
	if err != nil {
		panic(err)
	}

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

	if !strings.HasPrefix(m.Content, "!") {
		return
	}
	content := strings.TrimPrefix(m.Content, "!")

	splitContent := strings.Split(content, " ")
	content = strings.TrimPrefix(content, splitContent[0] + " ")

	var response string

	switch splitContent[0] {
	case "marco":
		s.ChannelMessageSend(m.ChannelID, "polo")
	case "commands":

		if len(splitContent) < 2 {
			response = "options for commands are add, remove"
			break
		}

		switch splitContent[1] {
		case "add":

			if len(splitContent) < 4 {
				response = "add syntax is: add <command name> <command text>"
				break
			}

			var commandText strings.Builder
			for _, v := range splitContent[3:] {
				commandText.WriteString(v + " ")
			}

			response = createCommand(m.GuildID, splitContent[2], strings.TrimSpace(commandText.String()))

		case "remove":

			if len(splitContent) < 3 {
				response = "remove syntax is: remove <commmand name to be remove>"
				break
			}



			response = removeCommand(m.GuildID, splitContent[2])

		case "edit":

			if len(splitContent) < 4 {
				response = "edit syntax is: edit <command name> <new command text>"
				break
			}

			var commandText strings.Builder
			for _, v := range splitContent[3:] {
				commandText.WriteString(v + " ")
			}

			response = editCommand(m.GuildID, splitContent[2], strings.TrimSpace(commandText.String()))
		}


	default:
		rp, ok := commands[m.GuildID][splitContent[0]]

		if ok {
			response = rp
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

func createCommand(guildID, command, response string) string {

	_, ok := commands[guildID]
	if !ok {
		commands[guildID] = make(map[string]string)
	}

	if strings.HasPrefix(command, "!") {
		command = strings.TrimPrefix(command, "!")
	}

	_, ok = commands[guildID][command]
	if ok {
		return fmt.Sprintf("Command \"%v\" already exists", command)
	}

	commands[guildID][command] = response
	return fmt.Sprintf("Command \"%v\" has been successfully added!", command)
}

func removeCommand(guildID, commandName string) string {

	if strings.HasPrefix(commandName, "!") {
		commandName = strings.TrimPrefix(commandName, "!")
	}

	_, ok := commands[guildID][commandName]

	if !ok {
		return fmt.Sprintf("Command \"%v\" does not exist", commandName)
	}

	delete(commands[guildID], commandName)
	return fmt.Sprintf("Command \"%v\" has been deleted", commandName)
}

func editCommand(guildID, commandName, commandText string) string {

	if strings.HasPrefix(commandName, "!") {
		commandName = strings.TrimPrefix(commandName, "!")
	}

	_, ok := commands[guildID][commandName]

	if !ok {
		return fmt.Sprintf("Command %v does not exist", commandName)
	}

	commands[guildID][commandName] = commandText
	return fmt.Sprintf("Command %v has been edited", commandName)

}