package store

// Store represents a server database
type Store interface {
	AddOrUpdateCommand(guildID, commandName, commandText string) error
	GetCommand(guildID, commandName string) (string, error)
	GetAllCommands(guildID string) ([][2]string, error)
	DeleteCommand(guildID, commandName string) error

	AddRole(guildID, botRole, roleID string) error
	GetRoles(guildID string) ([]string, error)

	SetPrefix(guildID, prefix string) error
	GetPrefix(guildID string) (string, error)

	SetName(guildID, name string) error
	GetName(guildID string) (string, error)
}
