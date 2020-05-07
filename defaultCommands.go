package main

import (
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strings"
)

type dfc struct {
	command    string
	function   defCommand
	permission int
}

const (
	botunknown   = iota
	botuser      = iota
	botdj        = iota
	botmoderator = iota
	botadmin     = iota
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
		command: "prefix", function:prefix, permission: botmoderator,
	},
	{
		command: "customs", function:listCustoms, permission: botunknown,
	},
}

func makeDefaultCommands() map[string]dfc {
	cmds := make(map[string]dfc)

	for _, v := range something {
		cmds[v.command] = v
	}

	return cmds
}

func addCommand(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) < 2 {
		return "Correct Syntax is: !addcommand <command name> <command text>", nil
	}

	command := splitString[0]
	_, ok := servers[guildID].commands[command]
	if ok {
		return fmt.Sprintf("Command \"%v\" already exists!", command), nil
	}

	servers[guildID].commands[command] = splitString[1]
	return fmt.Sprintf("Command \"%v\" has been successfully added!", command), nil
}

func editCommand(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) < 2 {
		return "syntax problem", nil
	}
	command := splitString[0]
	_, ok := servers[guildID].commands[command]
	if !ok {
		return fmt.Sprintf("Command \"%v\" doesn't exist", command), nil
	}

	servers[guildID].commands[command] = splitString[1]
	return fmt.Sprintf("Command \"%v\" has been successfully updated!", command), nil

}

func removeCommand(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) > 1 {
		return "Too many args", nil
	}

	command := splitString[0]
	_, ok := servers[guildID].commands[command]
	if !ok {
		return fmt.Sprintf("Command \"%v\" doesn't exist", command), nil
	}

	delete(servers[guildID].commands, command)

	return fmt.Sprintf("Command \"%v\" successfully removed!", command), nil

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

	servers[guildID].prefix = s
	return "Prefix successfully updated", nil
}

func polo(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {
	return "polo", nil
}

func listCustoms(s *discordgo.Session, m *discordgo.MessageCreate, c string) (string, error) {

	var som strings.Builder

	if len(servers[m.GuildID].commands) == 0 {
		return "", errors.New("server has no custom commands")
	}

	for i, v := range servers[m.GuildID].commands{
		fmt.Fprintf(&som, "Command: %v | Text: %v\n", i, v)
	}

	return som.String(), nil
}

func isDefaultCommand(s string) bool {
	_, ok := defaultCommands[s]
	return ok
}

func userPermissionLevel(s *discordgo.Session, m *discordgo.MessageCreate) int {

	b, _ := s.GuildMember(m.GuildID, m.Author.ID)

	highestPermission := botuser
	for _, v := range b.Roles {
		if val, ok := servers[m.GuildID].roles[v]; ok {
			if val > highestPermission {
				highestPermission = val
			}
		}
	}

	return highestPermission


}