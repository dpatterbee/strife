package strife

import (
	"database/sql"
	"fmt"
	"strings"

	dgo "github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/strife/src/media"
)

type botCommand struct {
	command    string
	function   defCommand
	permission int
	aliases    []string
}

const (
	botunknown = iota
	botuser
	botdj
	botmoderator
	botadmin
)

var roles = []string{
	"botunknown",
	"botuser",
	"botdj",
	"botmoderator",
	"botadmin",
}

type commandStuff struct {
	content  string
	response *Message
}

type defCommand func(*dgo.Session, commandStuff, *dgo.MessageCreate) error

var something = []botCommand{
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
	{
		command: "resume", function: resumeSound, permission: botunknown,
	},
	{
		command: "skip", function: skipSound, permission: botunknown,
	},
	{
		command: "disconnect", function: disconnectVoice, permission: botunknown,
	},
	{
		command: "queue", function: inspectQueue, permission: botunknown,
	},
}

func makeDefaultCommands() map[string]botCommand {
	cmds := make(map[string]botCommand)

	for _, v := range something {
		cmds[v.command] = v
		for _, w := range v.aliases {
			cmds[w] = v
		}
	}

	return cmds
}

func addCommand(_ *dgo.Session, m *dgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) < 2 {
		return "Correct Syntax is: !addcommand <command name> <command text>", nil
	}

	command := splitString[0]
	_, err := bot.store.GetCommand(guildID, command)
	if err == nil {
		return fmt.Sprintf("Command \"%v\" already exists!", command), nil
	} else if err != sql.ErrNoRows {
		return "", err
	}

	err = bot.store.AddOrUpdateCommand(guildID, command, splitString[1])
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Command \"%v\" has been successfully added!", command), nil
}

func editCommand(_ *dgo.Session, m *dgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) < 2 {
		return "Incorrect Syntax", nil
	}
	command := splitString[0]

	_, err := bot.store.GetCommand(guildID, command)
	if err == sql.ErrNoRows {
		return fmt.Sprintf("Command \"%v\" does not exist!", command), nil
	} else if err != nil {
		return "", nil
	}

	err = bot.store.AddOrUpdateCommand(guildID, command, splitString[1])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Command \"%v\" has been successfully updated!", command), nil

}

func removeCommand(_ *dgo.Session, m *dgo.MessageCreate, s string) (string, error) {
	guildID := m.GuildID

	splitString := strings.SplitN(s, " ", 2)

	if len(splitString) > 1 {
		return "Too many args", nil
	}

	command := splitString[0]
	_, err := bot.store.GetCommand(guildID, command)
	if err == sql.ErrNoRows {
		return fmt.Sprintf("Command \"%v\" doesn't exist", command), nil
	} else if err != nil {
		return "", err
	}

	err = bot.store.DeleteCommand(guildID, command)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("Command \"%v\" successfully removed!", command), nil

}

func commandsCommand(_ *dgo.Session, _ *dgo.MessageCreate, _ string) (string, error) {

	return "This command will list commands when I can be bothered typing what they all do", nil

}

func prefix(_ *dgo.Session, m *dgo.MessageCreate, s string) (string, error) {

	guildID := m.GuildID

	// Do some check for bad characters
	if len(strings.Split(s, " ")) > 1 {
		return "Prefix must be a single word", nil
	}

	if len(s) > 10 {
		return "Prefix must be 10 or fewer characters", nil
	}

	err := bot.store.SetPrefix(guildID, s)
	if err != nil {
		return "", err
	}

	return "Prefix successfully updated", nil
}

func polo(_ *dgo.Session, _ *dgo.MessageCreate, _ string) (string, error) {
	return "polo", nil
}

func listCustoms(_ *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {

	var som strings.Builder

	cmds, err := bot.store.GetAllCommands(m.GuildID)
	if err != nil {
		return "", err
	}
	if len(cmds) == 0 {
		return "Server has no custom commands", nil
	}

	for _, v := range cmds {
		_, _ = fmt.Fprintf(&som, "Command: %v | Text: %v\n", v[0], v[1])
	}

	return som.String(), nil
}

func isDefaultCommand(s string) bool {
	_, ok := bot.defaultCommands[s]
	return ok
}

func userPermissionLevel(s *dgo.Session, m *dgo.MessageCreate) int {

	b, _ := s.GuildMember(m.GuildID, m.Author.ID)

	highestPermission := botuser
	roles, err := bot.store.GetRoles(m.GuildID)
	if err != nil {
		return botuser
	}
	for _, v := range b.Roles {
		for i, w := range roles {
			if v == w {
				highestPermission = i
			}
		}
	}

	return highestPermission

}

func getUserVoiceChannel(userID, guildID string) (string, error) {

	guild, err := bot.session.State.Guild(guildID)
	if err != nil {
		return "", err
	}

	for _, v := range guild.VoiceStates {
		if v.UserID == userID {
			return v.ChannelID, nil
		}
	}

	return "", fmt.Errorf("user not in voice channel")

}

func mediaCommand(userID, guildID string, k media.Action, data string) (string, error) {

	userVoiceChannel, err := getUserVoiceChannel(userID, guildID)
	if err != nil {
		return fmt.Sprintf("You must be in a voice channel to %v the song",
			strings.ToLower(k.String())), nil
	}

	return bot.mediaController.Send(guildID, userVoiceChannel, k, data)

}

func playSound(_ *dgo.Session, m *dgo.MessageCreate, s string) (string, error) {
	return mediaCommand(m.Author.ID, m.GuildID, media.PLAY, s)
}

func pauseSound(_ *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {
	return mediaCommand(m.Author.ID, m.GuildID, media.PAUSE, "")
}

func resumeSound(_ *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {
	return mediaCommand(m.Author.ID, m.GuildID, media.RESUME, "")
}

func skipSound(_ *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {
	return mediaCommand(m.Author.ID, m.GuildID, media.SKIP, "")
}

func disconnectVoice(_ *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {
	return mediaCommand(m.Author.ID, m.GuildID, media.DISCONNECT, "")
}

func inspectQueue(_ *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {
	return mediaCommand(m.Author.ID, m.GuildID, media.INSPECT, "")
}
