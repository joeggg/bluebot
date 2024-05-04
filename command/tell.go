package command

import (
	"bluebot/config"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/hajimehoshi/go-mp3"
	"google.golang.org/api/option"
	"google.golang.org/api/texttospeech/v1"
	"layeh.com/gopus"
)

var (
	// Opus encoding constants
	FrameSize      int    = 960
	Channels       int    = 1
	SampleRate     int    = 48000
	BitRate        int    = 64 * 1000
	VoiceSelection string = "bluebot"
)

/*
Set the current voice preset from the available ones
*/
func HandleSetVoice(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if len(args) < 1 {
		session.ChannelMessageSend(msg.ChannelID, "No voice preset given")
		return nil
	}
	if _, ok := config.VoicePresets[args[0]]; !ok {
		session.ChannelMessageSend(msg.ChannelID, "Preset doesn't exist")
		return nil
	}
	VoiceSelection = args[0]
	session.ChannelMessageSend(msg.ChannelID, "Preset set to "+VoiceSelection)
	return nil
}

/*
Play the MP3 audio file generated by the Python backend
*/
func HandleTell(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if voiceChannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}
	// Join voice channel and start websocket audio communication
	vc, err := session.ChannelVoiceJoin(msg.GuildID, voiceChannelID, false, true)
	if err != nil {
		return err
	}
	defer vc.Disconnect()

	return playText(vc, strings.Join(args, " "), VoiceSelection)
}

func playText(vc *discordgo.VoiceConnection, text string, personality string) error {
	err := generateVoice(text, personality)
	if err != nil {
		return err
	}

	vc.Speaking(true)
	defer vc.Speaking(false)

	err = playMP3(vc)
	if err != nil {
		return err
	}

	return nil
}

func generateVoice(message string, personality string) error {
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
	err = os.WriteFile(config.Cfg.AudioPath+"/output.mp3", decoded, 0644)
	if err != nil {
		return err
	}
	return nil
}

/*
Play a reader of MP3 data over discord voice connection
*/
func playMP3(vc *discordgo.VoiceConnection) error {
	// Check the file opens first
	file, err := os.Open(config.Cfg.AudioPath + "/output.mp3")
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
		MP3Reader(rawChan, file)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		opusSender(rawChan, vc)
	}()

	wg.Wait()
	return nil
}

/*
Decode MP3 data to raw int16 and send in frames of FrameSize through channel
*/
func MP3Reader(c chan []int16, file io.Reader) {
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
		c <- buffer
	}
}

/*
Encodes raw audio data from a channel in opus and sends over discord
*/
func opusSender(c chan []int16, vc *discordgo.VoiceConnection) {
	// Set up encoder
	enc, err := gopus.NewEncoder(SampleRate, Channels, gopus.Audio)
	if err != nil {
		log.Println(err)
		return
	}
	enc.SetBitrate(BitRate)

	for {
		frame, ok := <-c
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
		vc.OpusSend <- opus
	}

}
