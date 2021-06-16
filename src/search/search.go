package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

type Client struct {
	apiKey   string
	queryMap map[string]query
	mapMut   sync.RWMutex
}

type query struct {
	qs string
	ch chan string
}

const LIMIT = 10

func NewClient(apiKey string) (*Client, error) {
	if len(apiKey) == 0 {
		return nil, errors.New("no api key provided")
	}
	return &Client{
		apiKey:   apiKey,
		queryMap: make(map[string]query),
	}, nil
}

func (c *Client) searchForIDs(request string, limit int64) ([]string, error) {
	ctx := context.Background()

	yt, err := youtube.NewService(ctx, option.WithAPIKey(c.apiKey))
	if err != nil {
		return nil, err
	}

	searchService := youtube.NewSearchService(yt)

	x, err := searchService.
		List([]string{"snippet"}).
		Q(request).
		MaxResults(limit).
		Type("video").
		Do()

	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(x.Items))
	for _, v := range x.Items {
		ids = append(ids, v.Id.VideoId)
	}

	return ids, nil
}

func (c *Client) SearchTopID(request string) (string, error) {
	id, err := c.searchForIDs(request, 1)
	if err != nil {
		return "", err
	}

	return id[0], nil
}

func (c *Client) Search(m *discordgo.MessageCreate, request string) (string, error) {

	identifier := m.GuildID + m.Author.ID
	c.mapMut.RLock()
	_, ok := c.queryMap[identifier]
	c.mapMut.RUnlock()
	if ok {
		return "", nil
	}
	q := query{
		qs: request,
		ch: make(chan string),
	}
	c.mapMut.Lock()
	c.queryMap[identifier] = q
	c.mapMut.Unlock()

	ctx := context.Background()
	yt, err := youtube.NewService(ctx, option.WithAPIKey(c.apiKey))
	if err != nil {
		return "", err
	}

	ids, err := c.searchForIDs(request, LIMIT)
	if err != nil {
		return "", err
	}

	vs := youtube.NewVideosService(yt)

	vid, err := vs.List([]string{"snippet", "contentDetails"}).Id(ids...).Do()
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for i, v := range vid.Items {
		_, _ = fmt.Fprintf(&sb, "%v. %v [%v]\n", i, v.Snippet.Title,
			strings.ToLower(v.ContentDetails.Duration[2:]))
	}
	return sb.String(), nil
}
