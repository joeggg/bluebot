package command

import (
	"bluebot/util"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/bwmarrin/discordgo"
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

/*
	Handle any music related command
*/
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
	Begin the download and playback of audio from a YT video or playlist link
*/
func HandlePlay(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if voiceChannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}

	if _, ok := Subscriptions[voiceChannelID]; ok {
		session.ChannelMessageSend(msg.ChannelID, "Already playing music")
		return nil
	}

	if len(args) < 2 {
		session.ChannelMessageSend(msg.ChannelID, "No URL given")
		return nil
	}

	return RunPlayer(session, msg, voiceChannelID, args[1:])
}

/*
	Add a video or playlist to the queue of an existing subscription
*/
func HandleQueue(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if voiceChannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}

	if len(args) < 2 {
		session.ChannelMessageSend(msg.ChannelID, "No URL given")
		return nil
	}

	// Start playing music if none currently being played
	if _, ok := Subscriptions[voiceChannelID]; !ok {
		return RunPlayer(session, msg, voiceChannelID, args[1:])
	}
	sub := Subscriptions[voiceChannelID]

	sub.AddToQueue(session, msg.ChannelID, args[1:])
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

/*
	Run a music player for a voice channel, from start to finish
*/
func RunPlayer(session *discordgo.Session, msg *discordgo.MessageCreate, voiceChannelID string, terms []string) error {
	// Make subscription object
	sub, err := NewSubscription()
	if err != nil {
		return err
	}
	Subscriptions[voiceChannelID] = sub
	defer delete(Subscriptions, voiceChannelID)
	defer delete(UsedIDs, sub.ID)
	log.Printf("Created subscription %s for user %s", sub.ID, msg.Author.Username)

	// Make folder for files
	if err = os.Mkdir(sub.Folder, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	defer os.RemoveAll(sub.Folder)
	go sub.AddToQueue(session, msg.ChannelID, terms)

	// File download manager
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go sub.ManageDownloads(ctx)
	// Wait for 1 track at least downloaded
	start := time.Now()
	for len(sub.Tracks) == 0 && time.Since(start) < 60*time.Second {
		time.Sleep(500 * time.Millisecond)
	}
	// Join voice channel and start websocket audio communication
	vc, err := session.ChannelVoiceJoin(msg.GuildID, voiceChannelID, false, true)
	if err != nil {
		return err
	}
	defer vc.Disconnect()
	// Any error handling past here must close the voice channel connection
	vc.Speaking(true)
	log.Printf("Starting playing for user %s", msg.Author.Username)
	err = sub.ManagePlayback(session, msg.ChannelID, vc)
	if err != nil {
		return err
	}

	vc.Speaking(false)
	session.ChannelMessageSend(msg.ChannelID, "Stopping playing")
	log.Printf("Removing subscription for user %s", msg.Author.Username)
	return nil
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
