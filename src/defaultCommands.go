package strife

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"cloud.google.com/go/firestore"
	"github.com/bwmarrin/discordgo"
)

type dfc struct {
	command    string
	function   defCommand
	permission int
}

const (
	botunknown = iota
	botuser
	botdj
	botmoderator
	botadmin
)

type defCommand func(*discordgo.Session, *discordgo.MessageCreate, string) (string, error)

var something = []dfc{
	{
		command: "marco", function: polo, permission: botunknown,
	},
	{
		command: "commands", function: commandsCommand, permission: botunknown,
	},
	{
		command: "addcommand", function: addCommand, permission: botmoderator,
	},
	{
		command: "editcommand", function: editCommand, permission: botmoderator,
	},
	{
		command: "removecommand", function: removeCommand, permission: botmoderator,
	},
	{
		command: "prefix", function: prefix, permission: botmoderator,
	},
	{
		command: "customs", function: listCustoms, permission: botunknown,
	},
	{
		command: "play", function: playSound, permission: botunknown,
	},
	{
		command: "pause", function: pauseSound, permission: botunknown,
	},
}

func makeDefaultCommands() map[string]dfc {
	cmds := make(map[string]dfc)

	for _, v := range something {
		cmds[v.command] = v
	}

	return cmds
}

func playSound(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	log.Println("setting up sound")
	guildID := m.GuildID

	currentGuild := bot.servers[guildID]

	url, err := parseURL(m, s)
	if err != nil {
		return "", err
	}

	userChannel, err := getUserVoiceChannel(sess, url.requester, guildID)
	if err != nil {
		return "You must be in a voice channel to request a song", nil
	}

	currentGuild.Lock()
	if currentGuild.songPlaying {
		if userChannel != currentGuild.playingChannel {
			currentGuild.Unlock()
			return "You must be in the same voice channel as the music bot to request a song", nil
		}
		currentGuild.songQueue = append(currentGuild.songQueue, url)
		currentGuild.Unlock()
		return "Song added to queue", nil
	}
	currentGuild.songPlaying = true
	currentGuild.Unlock()

	go soundHandler(guildID, userChannel)

	return "Song added to queue", nil
}

func addCommand(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) < 2 {
		return "Correct Syntax is: !addcommand <command name> <command text>", nil
	}

	command := splitString[0]
	_, ok := bot.servers[guildID].Commands[command]
	if ok {
		return fmt.Sprintf("Command \"%v\" already exists!", command), nil
	}

	bot.servers[guildID].Commands[command] = splitString[1]
	_, err := bot.client.Collection("servers").Doc(guildID).Set(ctx, map[string]interface{}{
		"commands": map[string]string{command: splitString[1]},
	}, firestore.MergeAll)
	return fmt.Sprintf("Command \"%v\" has been successfully added!", command), err
}

func editCommand(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) < 2 {
		return "syntax problem", nil
	}
	command := splitString[0]
	_, ok := bot.servers[guildID].Commands[command]
	if !ok {
		return fmt.Sprintf("Command \"%v\" doesn't exist", command), nil
	}

	bot.servers[guildID].Commands[command] = splitString[1]
	_, err := bot.client.Collection("servers").Doc(guildID).Set(ctx, map[string]interface{}{
		"commands": map[string]string{command: splitString[1]},
	}, firestore.MergeAll)
	return fmt.Sprintf("Command \"%v\" has been successfully updated!", command), err

}

func removeCommand(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) > 1 {
		return "Too many args", nil
	}

	command := splitString[0]
	_, ok := bot.servers[guildID].Commands[command]
	if !ok {
		return fmt.Sprintf("Command \"%v\" doesn't exist", command), nil
	}

	delete(bot.servers[guildID].Commands, command)
	_, err := bot.client.Collection("servers").Doc(guildID).Update(ctx, []firestore.Update{
		{
			Path:  "commands." + command,
			Value: firestore.Delete,
		},
	})

	return fmt.Sprintf("Command \"%v\" successfully removed!", command), err

}

func commandsCommand(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {

	return "This command will list commands when I can be bothered typing what they all do", nil

}

func prefix(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {

	guildID := m.GuildID

	//Do some check for bad characters
	if len(strings.Split(s, " ")) > 1 {
		return "Prefix must be a single word", nil
	}

	bot.servers[guildID].Prefix = s

	_, err := bot.client.Collection("servers").Doc(guildID).Set(ctx, map[string]interface{}{"prefix": s}, firestore.MergeAll)

	return "Prefix successfully updated", err
}

func polo(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	return "polo", nil
}

func listCustoms(s *discordgo.Session, m *discordgo.MessageCreate, c string) (string, error) {

	var som strings.Builder

	if len(bot.servers[m.GuildID].Commands) == 0 {
		return "", errors.New("server has no custom commands")
	}

	for i, v := range bot.servers[m.GuildID].Commands {
		fmt.Fprintf(&som, "Command: %v | Text: %v\n", i, v)
	}

	return som.String(), nil
}

func isDefaultCommand(s string) bool {
	_, ok := bot.defaultCommands[s]
	return ok
}

func userPermissionLevel(s *discordgo.Session, m *discordgo.MessageCreate) int {

	b, _ := s.GuildMember(m.GuildID, m.Author.ID)

	highestPermission := botuser
	for _, v := range b.Roles {
		if val, ok := bot.servers[m.GuildID].Roles[v]; ok {
			if int(val) > highestPermission {
				highestPermission = int(val)
			}
		}
	}

	return highestPermission

}

func pauseSound(s *discordgo.Session, m *discordgo.MessageCreate, c string) (string, error) {

	userID, guildID := m.Author.ID, m.GuildID
	currentGuild := bot.servers[guildID]

	userChannel, err := getUserVoiceChannel(s, userID, guildID)
	if err != nil {
		return "", err
	}

	if userChannel != currentGuild.songPlayingChannel {
		return "You must be in the same voice channel as the bot to pause music", nil
	}

	currentGuild.Lock()
	songSession := currentGuild.streamingSession
	songSession.SetPaused(true)
	currentGuild.Unlock()

	return "Song paused", nil

}
