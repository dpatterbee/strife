package strife

import (
	"database/sql"
	"fmt"
	"strings"

	dgo "github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/strife/src/media"
	"github.com/dpatterbee/strife/src/messages"
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
	response *messages.Message
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

func addCommand(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	guildID := m.GuildID

	splitString := strings.SplitN(c.content, " ", 2)

	if len(splitString) < 2 {
		c.response.Set("Correct Syntax is: !addcommand <command name> <command text>")
		return nil
	}

	command := splitString[0]
	_, err := bot.store.GetCommand(guildID, command)
	if err == nil {
		c.response.Set(fmt.Sprintf("Command \"%v\" already exists!", command))
	} else if err != sql.ErrNoRows {
		return err
	}

	err = bot.store.AddOrUpdateCommand(guildID, command, splitString[1])
	if err != nil {
		return err
	}

	c.response.Set(fmt.Sprintf("Command \"%v\" has been successfully added!", command))
	return nil
}

func editCommand(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	guildID := m.GuildID

	splitString := strings.SplitN(c.content, " ", 2)

	if len(splitString) < 2 {
		c.response.Set("Incorrect Syntax")
	}
	command := splitString[0]

	_, err := bot.store.GetCommand(guildID, command)
	if err == sql.ErrNoRows {
		c.response.Set(fmt.Sprintf("Command \"%v\" does not exist!", command))
		return nil
	} else if err != nil {
		return err
	}

	err = bot.store.AddOrUpdateCommand(guildID, command, splitString[1])
	if err != nil {
		return err
	}
	c.response.Set(fmt.Sprintf("Command \"%v\" has been successfully updated!", command))

	return nil
}

func removeCommand(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	guildID := m.GuildID

	splitString := strings.SplitN(c.content, " ", 2)

	if len(splitString) > 1 {
		c.response.Set("Too many args")
	}

	command := splitString[0]
	_, err := bot.store.GetCommand(guildID, command)
	if err == sql.ErrNoRows {
		c.response.Set(fmt.Sprintf("Command \"%v\" doesn't exist", command))
		return nil
	} else if err != nil {
		return err
	}

	err = bot.store.DeleteCommand(guildID, command)
	if err != nil {
		return err
	}

	c.response.Set(fmt.Sprintf("Command \"%v\" successfully removed!", command))

	return nil
}

func commandsCommand(_ *dgo.Session, c commandStuff, _ *dgo.MessageCreate) error {

	c.response.Set("This command will list commands when I can be bothered typing what they all do")

	return nil
}

func prefix(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {

	guildID := m.GuildID

	// Do some check for bad characters
	if len(strings.Split(c.content, " ")) > 1 {
		c.response.Set("Prefix must be a single word")
		return nil
	}

	if len(c.content) > 10 {
		c.response.Set("Prefix must be 10 or fewer characters")
		return nil
	}

	err := bot.store.SetPrefix(guildID, c.content)
	if err != nil {
		return err
	}

	c.response.Set("Prefix successfully updated")
	return nil
}

func polo(_ *dgo.Session, c commandStuff, _ *dgo.MessageCreate) error {
	c.response.Set("polo")
	return nil
}

func listCustoms(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {

	var som strings.Builder

	cmds, err := bot.store.GetAllCommands(m.GuildID)
	if err != nil {
		return err
	}
	if len(cmds) == 0 {
		c.response.Set("Server has no custom commands")
		return nil
	}

	for _, v := range cmds {
		_, _ = fmt.Fprintf(&som, "Command: %v | Text: %v\n", v[0], v[1])
	}

	c.response.Set(som.String())

	return nil
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

func mediaCommand(userID, guildID string, k media.Action, c commandStuff) error {

	userVoiceChannel, err := getUserVoiceChannel(userID, guildID)
	if err != nil {
		return err
	}

	_, err = bot.mediaController.Send(guildID, userVoiceChannel, k, c.content)
	if err != nil {
		return err
	}

	return nil

}

func playSound(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	return mediaCommand(m.Author.ID, m.GuildID, media.PLAY, c)
}

func pauseSound(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	return mediaCommand(m.Author.ID, m.GuildID, media.PAUSE, c)
}

func resumeSound(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	return mediaCommand(m.Author.ID, m.GuildID, media.RESUME, c)
}

func skipSound(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	return mediaCommand(m.Author.ID, m.GuildID, media.SKIP, c)
}

func disconnectVoice(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	return mediaCommand(m.Author.ID, m.GuildID, media.DISCONNECT, c)
}

func inspectQueue(_ *dgo.Session, c commandStuff, m *dgo.MessageCreate) error {
	return mediaCommand(m.Author.ID, m.GuildID, media.INSPECT, c)
}
