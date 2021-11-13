package log

import (
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
	"github.com/rs/zerolog/log"
)

func init() {
	out := diode.NewWriter(os.Stdout, 1000, 10*time.Millisecond, func(missed int) {
		log.Printf("Logger dropped %v messages", missed)
	})
	log.Logger = log.Output(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
		w.Out = out
	}))
}
