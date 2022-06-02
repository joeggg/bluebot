package command

import (
	"bluebot/util"
	"errors"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/ebml-go/webm"
	"github.com/kkdai/youtube/v2"
)

var (
	FrameSize  int = 960
	Channels   int = 1
	SampleRate int = 48000
)

var subscriptions = make(map[string]chan string)
var subCommands = map[string]util.HandlerFunc{
	"play":  HandlePlay,
	"pause": HandlePause,
	"stop":  HandleStop,
}

func HandleYT(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if subCommand, ok := subCommands[args[0]]; !ok {
		session.ChannelMessageSend(msg.ChannelID, "Unknown music command")
		return nil
	} else {
		return subCommand(session, msg, args[1:])
	}
}

/*
	Download and play the audio of a video from YouTube
*/
func HandlePlay(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if _, ok := subscriptions[msg.Author.ID]; ok {
		session.ChannelMessageSend(msg.ChannelID, "Already playing music")
		return nil
	}
	url := args[0]
	client := youtube.Client{}

	video, err := client.GetVideo(url)
	if err != nil {
		return err
	}

	format, err := GetFirstOpusFormat(&video.Formats)
	if err != nil {
		return err
	}

	stream, _, err := client.GetStream(video, format)
	if err != nil {
		return err
	}
	log.Println("Downloading audio file")
	file, err := SaveWebMStream(stream)
	if err != nil {
		return err
	}
	defer file.Close()
	defer os.Remove("test.webm")

	eventChan := make(chan string)
	subscriptions[msg.Author.ID] = eventChan
	vc, err := JoinAuthorVoiceChannel(session, msg)
	if err != nil {
		return err
	}
	// Any error handling past here must close the voice channel connection
	vc.Speaking(true)
	err = PlayWebMFile(vc, file)
	if err != nil {
		vc.Disconnect()
		return err
	}

	vc.Speaking(false)
	vc.Disconnect()
	return nil
}

func HandlePause(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	return nil
}

func HandleStop(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	return nil
}

/*
	Find first opus format youtube audio
*/
func GetFirstOpusFormat(formats *youtube.FormatList) (*youtube.Format, error) {
	var format youtube.Format
	for _, format := range *formats {
		if format.AudioChannels > 0 && strings.Contains(format.MimeType, "opus") {
			return &format, nil
		}
	}
	return &format, errors.New("no format could be found")
}

/*
	Find if a the message author is in a channel and join it
*/
func JoinAuthorVoiceChannel(session *discordgo.Session, msg *discordgo.MessageCreate) (*discordgo.VoiceConnection, error) {
	var vc *discordgo.VoiceConnection
	// Find sender's voice channel
	guild, err := session.State.Guild(msg.GuildID)
	if err != nil {
		return vc, err
	}
	voicechannelID := ""
	for _, vs := range guild.VoiceStates {
		if vs.UserID == msg.Author.ID {
			voicechannelID = vs.ChannelID
			break
		}
	}
	if voicechannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return vc, nil
	}

	// Join voice channel and start websocket audio communication
	vc, err = session.ChannelVoiceJoin(msg.GuildID, voicechannelID, false, true)
	if err != nil {
		return vc, err
	}
	return vc, nil
}

func SaveWebMStream(stream io.ReadCloser) (*os.File, error) {
	// Save to file first
	var file *os.File
	first, err := os.Create("test.webm")
	if err != nil {
		return file, err
	}
	_, err = io.Copy(first, stream)
	if err != nil {
		return file, err
	}
	first.Close()
	// Reopen needed for some reason
	file, err = os.Open("test.webm")
	return file, err
}

func PlayWebMFile(vc *discordgo.VoiceConnection, file *os.File) error {
	// Parse webm
	var w webm.WebM
	wr, err := webm.Parse(file, &w)
	if err != nil {
		return err
	}
	// Read data from parsed webm
	for {
		// Weird sleep needed to not be faster than the parsing
		// Shouldn't matter as packets are 20ms long
		time.Sleep(time.Millisecond)
		select {
		case packet, ok := <-wr.Chan:
			if !ok {
				return nil
			}
			log.Println(packet.Timecode)
			vc.OpusSend <- packet.Data
		default:
			return nil
		}
	}
}
