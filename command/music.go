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
	"play":   HandlePlay,
	"pause":  HandleEvent,
	"resume": HandleEvent,
	"stop":   HandleEvent,
}

func HandleYT(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if subCommand, ok := subCommands[args[0]]; !ok {
		session.ChannelMessageSend(msg.ChannelID, "Unknown music command")
		return nil
	} else {
		return subCommand(session, msg, args)
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
	url := args[1]
	filename, err := DownloadAudio(url)
	if err != nil {
		return err
	}

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	defer os.Remove(filename)

	eventChan := make(chan string)
	log.Printf("Creating subscription for user %s", msg.Author.Username)
	subscriptions[msg.Author.ID] = eventChan
	vc, err := JoinAuthorVoiceChannel(session, msg)
	if err != nil {
		return err
	}
	// Any error handling past here must close the voice channel connection
	vc.Speaking(true)
	log.Println("Playing track")
	err = PlayWebMFile(session, msg, vc, eventChan, file)
	if err != nil {
		vc.Disconnect()
		return err
	}

	vc.Speaking(false)
	vc.Disconnect()
	log.Printf("Removing subscription for user %s", msg.Author.Username)
	delete(subscriptions, msg.Author.ID)
	return nil
}

func HandleEvent(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if _, ok := subscriptions[msg.Author.ID]; !ok {
		session.ChannelMessageSend(msg.ChannelID, "No music playing")
		return nil
	}
	subscriptions[msg.Author.ID] <- args[0]
	return nil
}

func DownloadAudio(url string) (string, error) {
	var filename string
	client := youtube.Client{}

	video, err := client.GetVideo(url)
	if err != nil {
		return filename, err
	}

	format, err := GetFirstOpusFormat(&video.Formats)
	if err != nil {
		return filename, err
	}

	log.Println("Downloading audio file")
	stream, _, err := client.GetStream(video, format)
	if err != nil {
		return filename, err
	}

	filename = video.ID + ".webm"
	// Must save to file for webm decoder
	file, err := os.Create(filename)
	if err != nil {
		return filename, err
	}
	defer file.Close()
	_, err = io.Copy(file, stream)
	if err != nil {
		return filename, err
	}

	return filename, nil
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

func PlayWebMFile(session *discordgo.Session, msg *discordgo.MessageCreate, vc *discordgo.VoiceConnection, ch chan string, file *os.File) error {
	// Parse webm
	var w webm.WebM
	wr, err := webm.Parse(file, &w)
	if err != nil {
		return err
	}
	// Read opus data from parsed webm and pass into sending channel
	for {
		// Weird sleep needed to not be faster than the parsing
		// Shouldn't matter as packets are 20ms long
		time.Sleep(100 * time.Microsecond)
		select {
		case event := <-ch:
			switch event {
			case "pause":
				WaitForResume(ch)
			case "stop":
				session.ChannelMessageSend(msg.ChannelID, "Stopping playing")
				return nil
			default:
				continue
			}
		// Send the opus data
		case packet, ok := <-wr.Chan:
			if !ok {
				return nil
			}
			vc.OpusSend <- packet.Data
		default:
			return nil
		}
	}
}

func WaitForResume(ch chan string) {
	for {
		select {
		case event := <-ch:
			if event == "resume" {
				return
			}
			time.Sleep(time.Millisecond * 500)
		default:
			time.Sleep(time.Millisecond * 500)
		}
	}
}
