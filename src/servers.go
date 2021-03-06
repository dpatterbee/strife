package strife

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/iterator"
)

type serverl struct {
	Commands map[string]string `firestore:"commands"`
	Name     string            `firestore:"name"`
	Prefix   string            `firestore:"prefix"`
	Roles    map[string]int64  `firestore:"roles"`
	ID       string            `firestore:"ID"`
}

type server struct {
	Commands map[string]string
	Name     string
	Prefix   string
	Roles    map[string]int64
	ID       string
}

func buildServerData(ctx context.Context, s *discordgo.Session) (map[string]*server, error) {

	// log := log.With().Caller().Logger()

	svs := make(map[string]*serverl)

	log.Info().Msg("Getting Server info from Database")

	// Get server data from database
	iter := bot.client.Collection("servers").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}

		var s2 serverl
		err = doc.DataTo(&s2)
		if err != nil {
			return nil, err
		}
		svs[doc.Ref.ID] = &s2
	}
	log.Info().Msg("Updating server info from Discord")
	// Update retrieved data with values which have changed since the database was last updated
	for _, v := range svs {
		guildID := v.ID
		roles := getServerRoles(s, guildID)
		_, err := updateServers(guildID, map[string]interface{}{
			"roles": roles,
		})
		if err != nil {
			return nil, err
		}
	}

	log.Info().Msg("Creating newly found servers")
	// Create new entries for servers which were not previously in the database
	guilds, _ := s.UserGuilds(100, "", "")
	for _, v := range guilds {
		if _, ok := svs[v.ID]; !ok {
			svs[v.ID] = &serverl{
				Commands: make(map[string]string),
				Name:     v.Name,
				Prefix:   "!",
				Roles:    getServerRoles(s, v.ID),
				ID:       v.ID,
			}
			_, err := bot.client.Collection("servers").Doc(v.ID).Set(ctx, *svs[v.ID])
			if err != nil {
				return nil, err
			}
		}
	}

	sss := make(map[string]*server)

	for i, v := range svs {
		sss[i] = &server{
			Commands: v.Commands,
			Name:     v.Name,
			Prefix:   v.Prefix,
			Roles:    v.Roles,
			ID:       v.ID,
		}
	}

	return sss, nil

}
