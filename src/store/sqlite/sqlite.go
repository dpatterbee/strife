package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dpatterbee/strife/src/store"
	"github.com/rs/zerolog/log"

	_ "github.com/mattn/go-sqlite3"
)

type db struct {
	ctx *sql.DB
	sync.RWMutex
}

// New returns a new sqlite db object
func New() store.Store {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		log.Fatal().Msg("No database directory environment variable")
	}
	dbDir := filepath.Dir(dbPath)
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		err := os.Mkdir(dbDir, os.ModePerm)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
	}

	ctx, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	_, err = ctx.Exec(
		`create table if not exists commands(
					guildID 	text,
					commandName	text,
					commandText	text,
				constraint command_pk
					primary key(guildID, commandName)
            	);`,
	)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	_, err = ctx.Exec(
		`create table if not exists servers(
    				guildID text,
    				name text,
    				prefix text,
    				botuser text,
    				botdj text,
    				botmoderator text,
    				botadmin text,
    			constraint server_pk
                    primary key (guildID)
                );`,
	)

	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	return &db{
		ctx: ctx,
	}

}

// AddOrUpdateCommand inserts or replaces a command in the database
func (d *db) AddOrUpdateCommand(guildID, commandName, commandText string) error {
	d.Lock()
	defer d.Unlock()
	_, err := d.ctx.Exec(
		"INSERT OR REPLACE INTO commands(guildID, commandName, commandText) VALUES (?,?,?)",
		guildID, commandName, commandText,
	)

	return err
}

// GetCommand gets the specified command from the database or returns an error
func (d *db) GetCommand(guildID, commandName string) (string, error) {
	d.RLock()
	defer d.RUnlock()
	stmt, err := d.ctx.Prepare(
		"SELECT commandText FROM commands WHERE guildID = ? AND commandName= ?")
	if err != nil {
		return "", err
	}

	defer func(stmt *sql.Stmt) {
		_ = stmt.Close()
	}(stmt)

	var contents string

	err = stmt.QueryRow(guildID, commandName).Scan(&contents)
	if err != nil {
		return "", err
	}

	return contents, nil

}

// GetAllCommands returns all commands and their bodies from the db
func (d *db) GetAllCommands(guildID string) ([][2]string, error) {
	d.RLock()
	defer d.RUnlock()
	stmt, err := d.ctx.Prepare(
		"SELECT commandName, commandText FROM commands WHERE guildID = ?")
	if err != nil {
		return nil, err
	}

	defer func(stmt *sql.Stmt) {
		err := stmt.Close()
		if err != nil {
			log.Error().Err(err).Msg("")
		}
	}(stmt)

	rows, err := stmt.Query(guildID)
	if err != nil {
		return nil, err
	}

	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			log.Error().Err(err).Msg("")
		}
	}(rows)

	var contents [][2]string

	for rows.Next() {
		var name, text string
		if err := rows.Scan(&name, &text); err != nil {
			log.Error().Err(err).Msg("")
			continue
		}

		contents = append(contents, [2]string{name, text})
	}

	return contents, nil

}

// DeleteCommand removes the specified command from the database, error if nonexistent
func (d *db) DeleteCommand(guildID, commandName string) error {
	d.Lock()
	defer d.Unlock()
	_, err := d.ctx.Exec("DELETE FROM commands WHERE guildID = ? AND commandName = ?",
		guildID, commandName)
	return err
}

func (d *db) serverCreate(guildID string) {
	_, _ = d.ctx.Exec("INSERT OR ABORT INTO servers (guildID) VALUES (?)", guildID)
}

// AddRole adds a role to the database
func (d *db) AddRole(guildID, botRole, roleID string) error {
	d.Lock()
	defer d.Unlock()
	d.serverCreate(guildID)

	_, err := d.ctx.Exec(fmt.Sprintf("UPDATE servers SET %v = ? WHERE guildID = ?", botRole),
		roleID, guildID)

	return err
}

// GetRoles gets the IDs for all roles in the specified guild
func (d *db) GetRoles(guildID string) ([]string, error) {
	d.RLock()
	defer d.RUnlock()

	stmt, err := d.ctx.Prepare(
		`SELECT botuser, botdj, botmoderator, botadmin FROM servers
		WHERE guildID = ?`)

	if err != nil {
		return nil, err
	}
	var botuser, botdj, botmoderator, botadmin string
	err = stmt.QueryRow(guildID).Scan(&botuser, &botdj, &botmoderator, &botadmin)
	if err != nil {
		return nil, err
	}

	var contents []string
	contents = append(contents, botuser)
	contents = append(contents, botdj)
	contents = append(contents, botmoderator)
	contents = append(contents, botadmin)

	return contents, nil
}

// SetPrefix sets the prefix of the specified guild
func (d *db) SetPrefix(guildID, prefix string) error {
	d.Lock()
	defer d.Unlock()
	d.serverCreate(guildID)

	_, err := d.ctx.Exec("UPDATE servers SET prefix = ? WHERE guildID = ?",
		prefix, guildID)

	return err
}

// GetPrefix gets the prefix of the specified guild
func (d *db) GetPrefix(guildID string) (string, error) {
	d.RLock()
	defer d.RUnlock()

	stmt, err := d.ctx.Prepare("SELECT prefix FROM servers WHERE guildID = ?")
	if err != nil {
		return "", err
	}

	var prefix string
	err = stmt.QueryRow(guildID).Scan(&prefix)
	if err != nil {
		return "", err
	}

	return prefix, nil

}

// SetName stores the name of the server
func (d *db) SetName(guildID, name string) error {
	d.Lock()
	defer d.Unlock()
	d.serverCreate(guildID)

	_, err := d.ctx.Exec("UPDATE servers SET name = ? WHERE guildID = ?", name, guildID)
	return err
}

// GetName gets the name of the specified guild
func (d *db) GetName(guildID string) (string, error) {
	d.RLock()
	defer d.RUnlock()

	stmt, err := d.ctx.Prepare("SELECT name FROM servers WHERE guildID = ?")
	if err != nil {
		return "", err
	}

	var name string
	err = stmt.QueryRow(guildID).Scan(&name)
	if err != nil {
		return "", err
	}
	return name, nil

}
