package streamer

import (
	"context"
	"io"
	lg "log"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/dpatterbee/bpipe"
	"github.com/dpatterbee/strife/src/media"
	"github.com/jonas747/dca"
	"github.com/rs/zerolog/log"
)

type Session struct {
	download *downloadSession
	encode   *dca.EncodeSession
	stream   *streamSession

	Done chan error
}

func (s *Session) Start() {
	s.stream.start()
}

func (s *Session) Pause() bool {
	return s.stream.stop()
}

func (s *Session) Resume() bool {
	return s.stream.resume()
}

// Stop will stop a download, encode and stream session, if any exist
func (s *Session) Stop() bool {
	return s.stream.stop()
}

type downloadSession struct {
	song media.Streamable
	pipe bpipe.Bpipe

	cancel context.CancelFunc
	sync.Mutex
}

// downloadToPipe streams the streamable song and writes the data to writePipe
func downloadToPipe(writePipe io.WriteCloser, song media.Streamable) *downloadSession {
	d := downloadSession{}

	d.Lock()
	go download(&d, writePipe, song)

	return &d
}

func download(d *downloadSession, writePipe io.WriteCloser, song media.Streamable) {

	ctx, cancel := context.WithCancel(context.Background())
	stream, length, err := song.Stream(ctx)
	if err != nil {
		log.Error().Err(err).Send()
		cancel()
		err := writePipe.Close()
		if err != nil {
			log.Error().Err(err).Send()
		}
		return
	}
	log.Info().Int64("downloaded video size", length).Send()
	defer func(stream io.ReadCloser) {
		err := stream.Close()
		if err != nil {
			log.Error().Err(err).Send()
		}
	}(stream)
	d.cancel = cancel
	d.Unlock()

	_, err = io.Copy(writePipe, stream)
	if err != nil {
		log.Error().Err(err).Msg("data copy failed")
	} else {
		log.Info().Msg("finished download")
	}

	err = writePipe.Close()
	if err != nil {
		log.Error().Err(err).Send()
	}
}

// NewSession fucks
func NewSession(vc *discordgo.VoiceConnection, song media.Streamable) (*Session, error) {
	bufPipe := bpipe.New()

	d := downloadToPipe(bufPipe, song)

	// Trick the dca module into using my logger with level=warn
	t := log.With().Str("level", "warn").Logger()
	dca.Logger = lg.New(t, "", 0)

	encode, err := dca.EncodeMem(bufPipe, dca.StdEncodeOptions)
	if err != nil {
		return nil, err
	}

	stream := &streamSession{
		vc:     vc,
		cancel: make(chan bool),
		done:   make(chan error),
		source: encode,
	}

	return &Session{
		download: d,
		encode:   encode,
		stream:   stream,
		Done:     stream.done,
	}, nil
}
