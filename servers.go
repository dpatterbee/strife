package main

import (
	"cloud.google.com/go/firestore"
	"context"
	"fmt"
	"github.com/bwmarrin/discordgo"
	"google.golang.org/api/iterator"
)

type server struct {
	Commands map[string]string `firestore:"commands"`
	Name     string            `firestore:"name"`
	Prefix   string            `firestore:"prefix"`
	Roles    map[string]int64  `firestore:"roles"`
}

func buildServerData(s *discordgo.Session, client *firestore.Client, ctx context.Context) map[string]*server {

	svs := make(map[string]*server)

	iter := client.Collection("servers").Documents(ctx)
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}

		var s server
		err = doc.DataTo(&s)
		if err != nil {
			panic(err)
		}
		svs[doc.Ref.ID] = &s
	}

	fmt.Println(svs)

	guilds, _ := s.UserGuilds(100, "", "")
	for _, v := range guilds {
		if _, ok := svs[v.ID]; !ok {
			svs[v.ID] = &server{
				Commands: make(map[string]string),
				Name:     v.Name,
				Prefix:   "!",
				Roles:    getServerRoles(s, v.ID),
			}
			_, err := client.Collection("servers").Doc(v.ID).Set(ctx, *svs[v.ID])
			if err != nil {
				panic(err)
			}
		}
	}

	fmt.Println(svs)

	return svs

}
