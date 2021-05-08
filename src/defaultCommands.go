package strife

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	dgo "github.com/bwmarrin/discordgo"
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

type defCommand func(*dgo.Session, *dgo.MessageCreate, string) (string, error)

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
	}

	return cmds
}

func playSound(sess *dgo.Session, m *dgo.MessageCreate, s string) (string, error) {

	resultChan := make(chan string)

	userVoiceChannel, err := getUserVoiceChannel(sess, m.Author.ID, m.GuildID)
	if err != nil {
		return "You need to be in a voice channel to use this command.", nil
	}
	var req mediaRequest

	s = strings.TrimSpace(s)
	if len(s) > 0 {
		req = mediaRequest{commandType: play, guildID: m.GuildID, commandData: s, returnChan: resultChan, channelID: userVoiceChannel}
	} else {
		req = mediaRequest{commandType: resume, guildID: m.GuildID, commandData: s, returnChan: resultChan, channelID: userVoiceChannel}
	}

	timeout := time.NewTimer(stdTimeout)

	select {
	case bot.mediaControllerChannel <- req:
		// Not sure if this is actually required
		if !timeout.Stop() {
			<-timeout.C
		}
	case <-timeout.C:
		return "Server busy, please try again", nil
	}

	timeout.Reset(10 * time.Second)
	select {
	case result := <-resultChan:
		return result, nil
	case <-timeout.C:
		return "", nil
	}
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

func pauseSound(s *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {

	userVoiceChannel, err := getUserVoiceChannel(s, m.Author.ID, m.GuildID)
	if err != nil {
		return "You must be in a voice channel to pause the song", nil
	}

	ch := make(chan string)

	req := mediaRequest{commandType: pause, guildID: m.GuildID, channelID: userVoiceChannel, returnChan: ch}

	timeout := time.NewTimer(stdTimeout)

	select {
	case bot.mediaControllerChannel <- req:
		timeout.Stop()
	case <-timeout.C:
		return "Servers busy !", nil
	}

	timeout.Reset(10 * time.Second)
	select {
	case result := <-ch:
		timeout.Stop()
		return result, nil
	case <-timeout.C:
		return "Request sent", nil
	}

}

func resumeSound(s *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {

	userVoiceChannel, err := getUserVoiceChannel(s, m.Author.ID, m.GuildID)
	if err != nil {
		return "You must be in a voice channel to resume the song", nil
	}

	ch := make(chan string)

	req := mediaRequest{commandType: resume, guildID: m.GuildID, channelID: userVoiceChannel, returnChan: ch}

	timeout := time.NewTimer(stdTimeout)

	select {
	case bot.mediaControllerChannel <- req:
		timeout.Stop()
	case <-timeout.C:
		return "Servers busy !", nil
	}

	timeout.Reset(10 * time.Second)
	select {
	case result := <-ch:
		timeout.Stop()
		return result, nil
	case <-timeout.C:
		return "Request sent", nil
	}
}

func skipSound(s *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {

	userVoiceChannel, err := getUserVoiceChannel(s, m.Author.ID, m.GuildID)
	if err != nil {
		return "You must be in a voice channel to skip the song", nil
	}

	ch := make(chan string)

	req := mediaRequest{commandType: skip, guildID: m.GuildID, channelID: userVoiceChannel, returnChan: ch}

	timeout := time.NewTimer(stdTimeout)

	select {
	case bot.mediaControllerChannel <- req:
		timeout.Stop()
	case <-timeout.C:
		return "Servers busy !", nil
	}

	timeout.Reset(10 * time.Second)
	select {
	case result := <-ch:
		timeout.Stop()
		return result, nil
	case <-timeout.C:
		return "Request sent", nil
	}

}

func disconnectVoice(s *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {

	userVoiceChannel, err := getUserVoiceChannel(s, m.Author.ID, m.GuildID)
	if err != nil {
		return "You must be in a voice channel to skip the song", nil
	}

	ch := make(chan string)

	req := mediaRequest{commandType: disconnect, guildID: m.GuildID, channelID: userVoiceChannel, returnChan: ch}

	timeout := time.NewTimer(stdTimeout)

	select {
	case bot.mediaControllerChannel <- req:
		timeout.Stop()
	case <-timeout.C:
		return "Servers busy !", nil
	}

	timeout.Reset(10 * time.Second)
	select {
	case result := <-ch:
		timeout.Stop()
		return result, nil
	case <-timeout.C:
		return "Request sent", nil
	}

}

func inspectQueue(_ *dgo.Session, m *dgo.MessageCreate, _ string) (string, error) {
	ch := make(chan string)

	req := mediaRequest{commandType: inspect, guildID: m.GuildID, channelID: "", returnChan: ch}

	timeout := time.NewTimer(stdTimeout)
	select {
	case bot.mediaControllerChannel <- req:
		timeout.Stop()
	case <-timeout.C:
		return "Servers busy !", nil
	}

	timeout = time.NewTimer(10 * time.Second)
	select {
	case result := <-ch:
		timeout.Stop()
		return result, nil
	case <-timeout.C:
		return "Request send", nil
	}

}
