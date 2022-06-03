package command

import (
	"bluebot/util"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/kkdai/youtube/v2"
)

var subCommands = map[string]util.HandlerFunc{
	"queue":  HandleQueue,
	"list":   HandleList,
	"next":   HandleEvent,
	"play":   HandlePlay,
	"pause":  HandleEvent,
	"resume": HandleEvent,
	"stop":   HandleEvent,
}

func HandleYT(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if len(args) < 1 {
		session.ChannelMessageSend(msg.ChannelID, "Invalid yt command")
		return nil
	}
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
	if len(args) < 2 {
		session.ChannelMessageSend(msg.ChannelID, "No URL given")
		return nil
	}
	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if voiceChannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}

	if _, ok := Subscriptions[voiceChannelID]; ok {
		session.ChannelMessageSend(msg.ChannelID, "Already playing music")
		return nil
	}

	log.Printf("Creating subscription for user %s", msg.Author.Username)
	sub := &Subscription{
		voiceChannelID + "-" + msg.Author.ID,
		[]*youtube.Video{},
		make(chan string),
		make(chan *youtube.Video, MaxQueueLen),
		make(chan *Track, MaxQueueLen),
	}
	Subscriptions[voiceChannelID] = sub
	defer delete(Subscriptions, voiceChannelID)
	err := AddToQueue(session, msg, sub, args[1])
	if err != nil {
		return err
	}
	// Make folder for files
	if err = os.Mkdir(sub.ID, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	defer os.RemoveAll(sub.ID)

	// File download manager
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go ManageFileQueue(ctx, sub)

	// Join voice channel and start websocket audio communication
	vc, err := session.ChannelVoiceJoin(msg.GuildID, voiceChannelID, false, true)
	if err != nil {
		return err
	}
	defer vc.Disconnect()
	// Any error handling past here must close the voice channel connection
	vc.Speaking(true)
	log.Printf("Starting playing for user %s", msg.Author.Username)
	err = PlayWebMFileQueue(session, msg, vc, sub)
	if err != nil {
		return err
	}

	vc.Speaking(false)
	session.ChannelMessageSend(msg.ChannelID, "Stopping playing")
	log.Printf("Removing subscription for user %s", msg.Author.Username)
	return nil
}

func HandleQueue(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if len(args) < 2 {
		session.ChannelMessageSend(msg.ChannelID, "No URL given")
		return nil
	}
	URL := args[1]
	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if _, ok := Subscriptions[voiceChannelID]; !ok {
		return HandlePlay(session, msg, args)
	}
	sub := Subscriptions[voiceChannelID]

	err := AddToQueue(session, msg, sub, URL)
	if err != nil {
		return err
	}
	return nil
}

func HandleList(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if _, ok := Subscriptions[voiceChannelID]; !ok {
		session.ChannelMessageSend(msg.ChannelID, "No music playing")
		return nil
	}

	sub := Subscriptions[voiceChannelID]
	output := "\\~~\\~~\\~~\\~~\\~~\\~~ Current queue \\~~\\~~\\~~\\~~\\~~\\~~\n"
	numTracks := len(sub.Queue)
	max := MaxListDisplay
	if numTracks < max {
		max = numTracks
	}
	for i := 0; i < max; i++ {
		output += fmt.Sprintf("%d - %s\n", i+1, sub.Queue[i].Title)
	}
	if numTracks > max {
		output += fmt.Sprintf("...and %d more tracks", numTracks-max)
	}
	session.ChannelMessageSend(msg.ChannelID, output)
	return nil
}

func HandleEvent(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if _, ok := Subscriptions[voiceChannelID]; !ok {
		session.ChannelMessageSend(msg.ChannelID, "No music playing")
		return nil
	}
	Subscriptions[voiceChannelID].Events <- args[0]
	return nil
}

func downloadAudio(folder string, video *youtube.Video) (*Track, error) {
	format, err := getFirstOpusFormat(&video.Formats)
	if err != nil {
		return nil, err
	}

	log.Println("Downloading audio file")
	client := youtube.Client{}
	stream, _, err := client.GetStream(video, format)
	if err != nil {
		return nil, err
	}

	// Create unique file name
	num, err := rand.Int(rand.Reader, big.NewInt(10000))
	if err != nil {
		return nil, err
	}
	filename := fmt.Sprintf("%s/%s-%s.webm", folder, video.ID, num)
	// Must save to file for webm decoder
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	_, err = io.Copy(file, stream)
	if err != nil {
		return nil, err
	}

	return &Track{filename, video.Title}, nil
}

/*
	Find first opus format youtube audio
*/
func getFirstOpusFormat(formats *youtube.FormatList) (*youtube.Format, error) {
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
func getAuthorVoiceChannel(session *discordgo.Session, msg *discordgo.MessageCreate) string {
	// Find sender's voice channel
	guild, err := session.State.Guild(msg.GuildID)
	if err != nil {
		return ""
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == msg.Author.ID {
			return vs.ChannelID
		}
	}
	return ""
}
