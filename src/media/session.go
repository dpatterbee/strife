package media

import (
	"io"
	"time"

	dgo "github.com/bwmarrin/discordgo"
	"github.com/rs/zerolog/log"
)

// guildSoundPlayer runs while a server has a queue of songs to be played.
// It loops over the queue of songs and plays them in order, exiting once it has drained the list
func guildSoundPlayer(
	discordSession *dgo.Session,
	guildID, channelID string,
	controlChannel <-chan playerCommand,
	songChannel <-chan songReq,
	mediaReturnRequestChan, mediaReturnFinishChan chan<- string,
	previousInstanceWaitChan <-chan bool,
) {
	log.Info().Msg("Sound handler not active, activating")

	if previousInstanceWaitChan != nil {
		<-previousInstanceWaitChan
	}

	queue := newSongQueue(songChannel)

	if !<-queue.firstSongWait {
		log.Info().Msg("Initial song request invalid, shutting down.")
		mediaReturnRequestChan <- guildID
		mediaReturnFinishChan <- guildID
		return
	}

	// Set up voiceconnection
	vc, err := discordSession.ChannelVoiceJoin(guildID, channelID, false, true)

	if err != nil {
		mediaReturnRequestChan <- guildID
		log.Error().Err(err).Msg("Couldn't initialise voice connection")
		return
	}

	disconnectTimer := time.NewTimer(5 * time.Second)

	log.Info().Msg("Media session ready")
mainLoop:
	for {
		if !disconnectTimer.Stop() {
			<-disconnectTimer.C
		}
		disconnectTimer.Reset(5 * time.Second)
		select {
		case control := <-controlChannel:
			switch control.commandType {
			case disconnect:
				go trySend(control.returnChannel, "Goodbye.", stdTimeout)
				break mainLoop
			default:
				go trySend(control.returnChannel, "No media playing.", stdTimeout)
			}
		case song := <-queue.nextSong:
			log.Info().
				Str("Title", song.Title()).
				Str("guildID", guildID).
				Msg("Playing Song")
			//encode, download, err := newSongSession(song)
			//streamingSession := newStreamingSession(encode, vc)
			mediaSession, err := newMediaSession(song, vc)
			if err != nil {
				log.Error().Err(err).Msg("")
				return
			}

			err = vc.Speaking(true)
			if err != nil {
				log.Error().Err(err).Msg("")
			}

			log.Info().
				Str("guildID", guildID).
				Str("channelID", channelID).
				Msg("Starting Audio Stream")

			mediaSession.stream.Start()

			// controlLoop should only be entered once it is possible to control the media ie. once
			// the ffmpeg session is up and running
		controlLoop:
			for {
				select {

				case err := <-mediaSession.stream.done:
					if err := vc.Speaking(false); err != nil {
						log.Error().Err(err).Msg("")
					}
					mediaSession.stop() // Ensure the song cleans up okay.
					if err == io.EOF {
						log.Info().Msg("Song Completed.")
					} else {
						log.Error().Err(err).Msg("Song Stopped.")
					}
					break controlLoop

				case control := <-controlChannel:

					switch control.commandType {

					case pause:
						ok := mediaSession.pause()
						if ok {
							go trySend(control.returnChannel, "Song paused.", stdTimeout)
						} else {
							go trySend(control.returnChannel, "Song already paused.", stdTimeout)
						}

					case resume:
						ok := mediaSession.resume()
						if ok {
							go trySend(control.returnChannel, "Song resumed.", stdTimeout)
						} else {
							go trySend(control.returnChannel, "Song already playing", stdTimeout)
						}

					case skip:
						if !mediaSession.encode.Running() {
							go trySend(control.returnChannel, "Not yet.", stdTimeout)
							continue

						}
						mediaSession.stop()

						go trySend(control.returnChannel, "Song skipped.", stdTimeout)

						break controlLoop

					case disconnect:
						if !mediaSession.encode.Running() {
							go trySend(control.returnChannel, "Not yet.", stdTimeout)
							continue

						}
						mediaSession.stop()

						go trySend(control.returnChannel, "Goodbye.", stdTimeout)

						break mainLoop

					case inspect:
						qch := make(chan []streamable)
						queue.inspectSongQueue <- qch
						q := <-qch
						songTimeRemaining := song.Duration() - mediaSession.stream.PlaybackPos()
						go trySend(control.returnChannel, prettySongList(q, songTimeRemaining), stdTimeout)
					}
				}
			}

		case <-disconnectTimer.C:
			mediaReturnRequestChan <- guildID
			break mainLoop

		}

	}

	// End queue goroutine and disconnect from voice channel before informing the coordinator
	// that we have finished.
	// TODO: I have implemented the potential for returning the queue after a session ends. This
	//  could be recovered afterwards.
	remainingQ := make(chan []streamable)
	queue.shutdown <- remainingQ
	<-remainingQ
	err = vc.Disconnect()
	if err != nil {
		log.Error().Err(err).Msg("")
	}
	mediaReturnFinishChan <- guildID

}
