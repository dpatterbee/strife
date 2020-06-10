package strife

import (
	"context"
	"log"

	"github.com/bwmarrin/discordgo"
	"google.golang.org/api/iterator"
)

type server struct {
	Commands map[string]string `firestore:"commands"`
	Name     string            `firestore:"name"`
	Prefix   string            `firestore:"prefix"`
	Roles    map[string]int64  `firestore:"roles"`
	ID       string            `firestore:"ID"`
}

func buildServerData(ctx context.Context, s *discordgo.Session) map[string]*server {

	svs := make(map[string]*server)

	log.Println("Getting Server info from Database")

	// Get server data from database
	iter := bot.client.Collection("servers").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}

		var s2 server
		err = doc.DataTo(&s2)
		if err != nil {
			log.Printf("%v", err)
		}
		svs[doc.Ref.ID] = &s2
	}
	log.Println("Updating server info from Discord")
	// Update retrieved data with values which have changed since the database was last updated
	for _, v := range svs {
		guildID := v.ID
		roles := getServerRoles(s, guildID)
		_, err := updateServers(guildID, map[string]interface{}{
			"roles": roles,
		})
		if err != nil {
			panic(err)
		}
	}

	log.Println("Creating newly found servers")
	// Create new entries for servers which were not previously in the database
	guilds, _ := s.UserGuilds(100, "", "")
	for _, v := range guilds {
		if _, ok := svs[v.ID]; !ok {
			svs[v.ID] = &server{
				Commands: make(map[string]string),
				Name:     v.Name,
				Prefix:   "!",
				Roles:    getServerRoles(s, v.ID),
				ID:       v.ID,
			}
			_, err := bot.client.Collection("servers").Doc(v.ID).Set(ctx, *svs[v.ID])
			if err != nil {
				panic(err)
			}
		}
	}

	return svs

}
