package core

import (
	"bluebot/config"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/hajimehoshi/go-mp3"
	"google.golang.org/api/option"
	"google.golang.org/api/texttospeech/v1"
	"layeh.com/gopus"
)

var (
	// Opus encoding constants
	FrameSize  int = 960
	Channels   int = 1
	SampleRate int = 48000
	BitRate    int = 64 * 1000
)

func PlayText(text string, personality string, container *Container) error {
	filename := fmt.Sprintf("%s/%s_output.mp3", config.Cfg.AudioPath, container.vc.ChannelID)
	start := time.Now()
	err := generateVoice(text, personality, filename)
	if err != nil {
		return err
	}
	log.Printf("Took %f seconds to generate voice", time.Since(start).Seconds())

	err = PlayMP3(container, filename)
	if err != nil {
		return err
	}

	return nil
}

/*
Play a reader of MP3 data over discord voice connection
*/
func PlayMP3(container *Container, filename string) error {
	// Check the file opens first
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	var wg sync.WaitGroup
	rawChan := make(chan []int16, 10)

	// Run reader and sender concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		mp3Reader(rawChan, file, container)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		opusSender(rawChan, container)
	}()

	wg.Wait()
	return nil
}

func generateVoice(message string, personality string, filename string) error {
	ctx := context.Background()
	tts, err := texttospeech.NewService(ctx, option.WithCredentialsFile(config.Cfg.GoogleKeyPath))
	if err != nil {
		return err
	}

	preset, ok := config.VoicePresets[personality]
	if !ok {
		return errors.New("personality given does not exist")
	}
	// Request for voice clip data from Google
	req := &texttospeech.SynthesizeSpeechRequest{
		Input: &texttospeech.SynthesisInput{
			Text: message,
		},
		AudioConfig: &texttospeech.AudioConfig{
			AudioEncoding: "MP3",
			Pitch:         preset.Pitch,
			SpeakingRate:  preset.Rate,
		},
		Voice: &texttospeech.VoiceSelectionParams{
			LanguageCode: preset.Language,
			Name:         preset.Name,
			SsmlGender:   preset.Gender,
		},
	}
	resp, err := tts.Text.Synthesize(req).Do()
	if err != nil {
		return err
	}
	// Convert to bytes and save
	decoded, err := base64.StdEncoding.DecodeString(resp.AudioContent)
	if err != nil {
		return err
	}
	err = os.WriteFile(filename, decoded, 0644)
	if err != nil {
		return err
	}
	return nil
}

/*
Decode MP3 data to raw int16 and send in frames of FrameSize through channel
*/
func mp3Reader(c chan []int16, file io.Reader, container *Container) {
	// Ensure channel closed after all is read
	defer close(c)
	// Set up decoder
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		log.Println(err)
		return
	}

	// Decode and break up into opus frames
	for {
		buffer := make([]int16, FrameSize*Channels)
		err := binary.Read(decoder, binary.LittleEndian, &buffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			c <- buffer
			return
		}
		if err != nil {
			log.Println(err)
			return
		}
		select {
		case <-container.ctx.Done():
			return
		// Send buffer into channel
		case c <- buffer:
		}
	}
}

/*
Encodes raw audio data from a channel in opus and sends over discord
*/
func opusSender(c chan []int16, container *Container) {
	// Set up encoder
	enc, err := gopus.NewEncoder(SampleRate, Channels, gopus.Audio)
	if err != nil {
		log.Println(err)
		return
	}
	enc.SetBitrate(BitRate)

	container.AcquirePlayLock()
	defer container.ReleasePlayLock()
	container.vc.Speaking(true)
	defer container.vc.Speaking(false)

	for {
		select {
		case <-container.ctx.Done():
			return

		case <-*container.pauseRequests:
			container.WaitForResume()

		case frame, ok := <-c:
			if !ok {
				// Reached end of channel
				return
			}
			// Encode frame and send through opus channel
			opus, err := enc.Encode(frame, FrameSize, FrameSize*Channels*2)
			if err != nil {
				log.Println(err)
				return
			}
			container.vc.OpusSend <- opus

		}
	}
}
