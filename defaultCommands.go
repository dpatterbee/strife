package main

import (
	"fmt"
	"github.com/bwmarrin/discordgo"
	"strings"
)

type dfc struct {
	command string
	funky func(*discordgo.MessageCreate,string) string
}

var something = []dfc{
	{
		command: "marco", funky: polo,
	},
	{
		command: "commands", funky: commandsCommand,
	},
	{
		command: "prefix", funky:prefix,
	},
}

func makeDefaultCommands() map[string]func(*discordgo.MessageCreate, string) string {
	cmds := make(map[string]func(*discordgo.MessageCreate, string) string)

	for _, v := range something {
		cmds[v.command] = v.funky
	}

	return cmds
}

func commandsCommand(m *discordgo.MessageCreate, s string) string {

	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 3)

	if len(splitString) < 2 {
		return "Unrecognised Command"
	}

	switch splitString[0] {
	case "add":
		sss := splitString[1:]
		if len(sss) < 2 {
			return "add syntax is: add <command name> <command text>"
		}

		_, ok := servers[guildID].commands[sss[0]]
		if ok {
			return fmt.Sprintf("Command \"%v\" already exists", sss[0])
		}

		servers[guildID].commands[sss[0]] = sss[1]
		return fmt.Sprintf("Command \"%v\" has been successfully added!", sss[0])
	case "edit":

		sss := splitString[1:]

		if len(sss) < 2 {
			return "edit syntax is: edit <command name> <new command text>"
		}

		_, ok := servers[guildID].commands[sss[0]]
		if !ok {
			return fmt.Sprintf("Command \"%v\" does not exist", sss[0])
		}

		servers[guildID].commands[sss[0]] = sss[1]
		return fmt.Sprintf("Command \"%v\" has been successfully updated.")

	case "remove":

		sss := splitString[1:]

		if len(sss) > 1 {
			return "remove syntax is: remove <command name to be remove>"
		}

		_, ok := servers[guildID].commands[sss[0]]
		if !ok {
			return fmt.Sprintf("Command \"%v\" does not exist", sss[0])
		}

		delete(servers[guildID].commands, sss[0])
		return fmt.Sprintf("Command \"%v\" successfully removed.", sss[0])
	default:
		return "Unrecognised Command"
	}
}

func prefix(m *discordgo.MessageCreate, s string) string {

	guildID := m.GuildID

	//Do some check for bad characters
	if len(strings.Split(s, " ")) > 1 {
		return "Prefix must be a single word"
	}

	servers[guildID].prefix = s
	return "Prefix successfully updated"
}

func polo(m *discordgo.MessageCreate, s string) string {
	return "polo"
}

func isDefaultCommand(s string) bool {
	_, ok := defaultCommands[s]
	return ok
}
