package sqlite

import (
	"database/sql"
	"log"
	"os"

	"github.com/dpatterbee/strife/store"
)

type db struct {
	ctx *sql.DB
}

func New() store.Store {
	dbDir := "data"
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		err := os.Mkdir(dbDir, os.ModePerm)
		if err != nil {
			log.Fatalln(err)
		}
	}

	ctx, err := sql.Open("sqlite3", dbDir+"/store.db")
	if err != nil {
		log.Fatalln(err)
	}
	_, err = ctx.Exec(
		`create table commands(
					guildID 	text,
					commandName	text,
					commandText	text,
				constraint command_pk
					primary key(guildID, commandName)
            	);`,
	)

	if err != nil {
		log.Fatalln(err)
	}

	return &db{
		ctx: ctx,
	}

}

func (d db) GetCommand(guildID, commandName string) (string, error) {
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

func (d db) AddCommand(guildID, commandName, commandText string) error {
	_, err := d.ctx.Exec(
		"INSERT INTO commands(guildID, commandName, commandText) VALUES (?,?,?)",
		guildID, commandName, commandText,
	)

	return err
}

func (d db) AddRole(guildID string, botRole int64, roleID string) error {
	return nil
}

func (d db) GetRole(guildID string, botRole int64) (string, error) {
	return "", nil
}
