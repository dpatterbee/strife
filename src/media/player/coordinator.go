package player

import (
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/strife/src/media"
)

type returner struct {
	sessionCloseStart  chan string
	sessionCloseFinish chan string

	started, finished bool
	sync.Mutex
}

func (r *returner) StartReturn(guildID string) {
	r.Lock()
	defer r.Unlock()
	if !r.started {
		r.started = true
		r.sessionCloseStart <- guildID
	}
}

func (r *returner) FinishReturn(guildID string) {
	r.Lock()
	defer r.Unlock()
	if !r.finished {
		r.finished = true
		r.sessionCloseFinish <- guildID
	}
}

func (r *returner) Exit(guildID string) {
	r.StartReturn(guildID)
	r.FinishReturn(guildID)
}

type Coordinator struct {
	dgoSession *discordgo.Session
	requests   chan media.Request

	activeSessions map[string]*Session
	dyingMCs       map[string]chan struct{}
	returner       *returner

	active bool
}

func NewCoordinator(session *discordgo.Session) *Coordinator {
	coordinator := &Coordinator{
		dgoSession:     session,
		requests:       make(chan media.Request),
		activeSessions: make(map[string]*Session),
		dyingMCs:       make(map[string]chan struct{}),
		returner: &returner{
			sessionCloseStart:  make(chan string),
			sessionCloseFinish: make(chan string),
		},
	}

	go coordinator.run()

	return coordinator
}

func (c *Coordinator) run() {

	// These channels are used by guildSoundPlayer goroutines to inform this goroutine of their
	// shutdown status.
	// mediaReturnBegin is used to inform that it has begun shutting down and that no more song or
	// command requests should be forwarded to that goroutine.
	// mediaReturnEnd is used to inform that it has completed shutting down and that any waiting
	// goroutines can be released.
	mediaReturnBegin := make(chan string)
	mediaReturnEnd := make(chan string)

	// This loops for the lifetime of the program, responding to messages sent on each channel.
	for {

		select {
		case req := <-c.requests:
			// play and disconnect are special cases of command, as they create and destroy channels
			// all other commands just get passed through to the respective server.
			switch req.CommandType {
			case media.PLAY:

				ch, ok := c.activeSessions[req.GuildID]
				if !ok {
					var waitChan chan struct{}
					if _, ok := c.dyingMCs[req.GuildID]; ok {
						c.dyingMCs[req.GuildID] = waitChan
					}
					// c.activeSessions[req.GuildID] = activeMC{
					// 	controlChannel: make(chan playerCommand, 5),
					// 	songChannel:    make(chan songReq, 100),
					// }
					ch = &Session{
						dgoSession: c.dgoSession,
						guildID:    req.GuildID,
						channelID:  req.ChannelID,
						returner:   c.returner,
						waitChan:   waitChan,
					}
					c.activeSessions[req.GuildID] = ch

					go ch.run()
				}

				ch.passRequest(req)

			case media.DISCONNECT:
				ch, ok := c.activeSessions[req.GuildID]
				if ok {
					ch.passRequest(req)

					c.dyingMCs[req.GuildID] = nil
					delete(c.activeSessions, req.GuildID)
				}

			case media.INSPECT:
				// TODO: get queue

			default:

				if ch, ok := c.activeSessions[req.GuildID]; ok {
					ch.passRequest(req)
				}
			}
		case guildID := <-mediaReturnBegin:

			// When a guildSoundPlayer goroutine informs us that they are beginning to shut down,
			// we close the communications channels and drain them,
			// then create an entry in our dyingMCs map and remove from activeMCs
			if _, ok := c.activeSessions[guildID]; ok {
				c.dyingMCs[guildID] = nil
				delete(c.activeSessions, guildID)
			} else {
				// TODO: log a very strange case
			}
		case guildID := <-mediaReturnEnd:

			// When a guildSoundPlayer goroutine informs us that it has completed shutting down,
			// we check if there is a waiting goroutine, and if so,
			// we signal it by closing the waitChan, then remove it from the map.
			// If there is no waiting goroutine, we just remove it from the map.
			if ch, ok := c.dyingMCs[guildID]; ok {
				if ch != nil {
					go func(ch chan<- struct{}) {
						close(ch)
					}(ch)
				}
				delete(c.dyingMCs, guildID)
			}

		}

	}

}

// Send sends a media.Request to the Coordinator
func (c *Coordinator) Send(
	guildID, channelID string, commandType media.Action,
	commandData string,
) (string, error) {

	timeout := time.NewTimer(5 * time.Second)
	retchan := make(chan string)

	req := media.Request{
		CommandType: commandType,
		GuildID:     guildID,
		ChannelID:   channelID,
		CommandData: commandData,
		ReturnChan:  retchan,
	}

	if !c.active {
		return "", media.ErrNotActive
	}
	select {
	case c.requests <- req:
	case <-timeout.C:
		return "", media.ErrServerBusy
	}

	select {
	case s := <-retchan:
		timeout.Stop()
		return s, nil
	case <-timeout.C:
		return "Request sent", nil

	}

}
