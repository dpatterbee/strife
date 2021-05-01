package store

type Store interface {
	GetCommand(guildID, commandName string) (string, error)
	AddCommand(guildID, commandName, commandText string) error

	AddRole(guildID string, botRole int64, roleID string) error
	GetRole(guildID string, botRole int64) (string, error)
}
