package main

import (
	"errors"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strings"
)

type dfc struct {
	command string
	fonk defCommand
}

type defCommand func(*discordgo.Session, *discordgo.MessageCreate, string) (string, error)

var something = []dfc{
	{
		command: "marco", fonk: polo,
	},
	{
		command: "commands", fonk: commandsCommand,
	},
	{
		command: "prefix", fonk:prefix,
	},
	{
		command: "list", fonk:listCustoms,
	},
}

func makeDefaultCommands() map[string]defCommand {
	cmds := make(map[string]defCommand)

	for _, v := range something {
		cmds[v.command] = v.fonk
	}

	return cmds
}

func commandsCommand(sess *discordgo.Session, m *discordgo.MessageCreate, s string) (string, error) {

	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 3)

	if len(splitString) < 2 {
		return "", fmt.Errorf("Unrecognised Command: %v", splitString[0])
	}

	switch splitString[0] {
	case "add":

		if !hasPermission(sess, m, "add") {
			return "", errors.New("invalid permission")
		}

		sss := splitString[1:]
		if len(sss) < 2 {
			return "add syntax is: add <command name> <command text>", nil
		}

		_, ok := servers[guildID].commands[sss[0]]
		if ok {
			return fmt.Sprintf("Command \"%v\" already exists", sss[0]), nil
		}

		servers[guildID].commands[sss[0]] = sss[1]
		return fmt.Sprintf("Command \"%v\" has been successfully added!", sss[0]), nil
	case "edit":

		if !hasPermission(sess, m, "add") {
			return "", errors.New("invalid permission")
		}

		sss := splitString[1:]

		if len(sss) < 2 {
			return "edit syntax is: edit <command name> <new command text>", nil
		}

		_, ok := servers[guildID].commands[sss[0]]
		if !ok {
			return fmt.Sprintf("Command \"%v\" does not exist", sss[0]), nil
		}

		servers[guildID].commands[sss[0]] = sss[1]
		return fmt.Sprintf("Command \"%v\" has been successfully updated."), nil

	case "remove":

		if !hasPermission(sess, m, "add") {
			return "", errors.New("invalid permission")
		}

		sss := splitString[1:]

		if len(sss) > 1 {
			return "remove syntax is: remove <command name to be remove>", nil
		}

		_, ok := servers[guildID].commands[sss[0]]
		if !ok {
			return fmt.Sprintf("Command \"%v\" does not exist", sss[0]), nil
		}

		delete(servers[guildID].commands, sss[0])
		return fmt.Sprintf("Command \"%v\" successfully removed.", sss[0]), nil
	default:
		return "Unrecognised Command", nil
	}
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

func hasPermission(s *discordgo.Session, m *discordgo.MessageCreate, c string) bool {
	st, _ := s.GuildMember(m.GuildID, m.Author.ID)

	for _, v := range st.Roles {

	}

	return false
}