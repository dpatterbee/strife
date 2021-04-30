package store

type Store interface {
	GetCommand(guildID, commandName string) (string, error)
	AddCommand(guildID, commandName, commandText string) error
}
