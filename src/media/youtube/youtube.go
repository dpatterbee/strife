package youtube

import (
	"context"
	"errors"
	"io"
	"time"

	yt "github.com/kkdai/youtube/v2"
)

type Video struct {
	video    *yt.Video
	title    string
	duration time.Duration
}

func NewVideo(url string) (Video, error) {
	client := yt.Client{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	vid, err := client.GetVideoContext(ctx, url)
	cancel()
	if err != nil {
		return Video{}, err
	}

	return Video{
		video:    vid,
		title:    vid.Title,
		duration: vid.Duration,
	}, nil
}

func (v Video) Stream(ctx context.Context) (io.ReadCloser, int64, error) {

	client := yt.Client{}
	format, err := videoFormatFinder(*v.video)
	if err != nil {
		return nil, 0, err
	}
	stream, length, err := client.GetStreamContext(ctx, v.video, &v.video.Formats[format])
	if err != nil {
		return nil, 0, err
	}
	return stream, length, nil
}

var audioQualities = map[string]int{
	"AUDIO_QUALITY_LOW":    1,
	"AUDIO_QUALITY_MEDIUM": 2,
	"AUDIO_QUALITY_HIGH":   3,
}

func videoFormatFinder(vid yt.Video) (int, error) {
	highestSoFar := 0
	highestQualityIndex := 0
	for i, v := range vid.Formats {
		if v.AudioChannels == 0 || v.AudioQuality == "" {
			continue
		}
		if j, ok := audioQualities[v.AudioQuality]; ok && j > highestSoFar {
			highestSoFar = j
			highestQualityIndex = i
		}
	}

	if vid.Formats[highestQualityIndex].AudioChannels == 0 {
		return 0, errors.New("no audio found")
	}

	return highestQualityIndex, nil
}

func (v Video) Title() string {
	return v.title
}

func (v Video) Duration() time.Duration {
	return v.duration
}
