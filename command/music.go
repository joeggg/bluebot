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
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/ebml-go/webm"
	"github.com/kkdai/youtube/v2"
)

var (
	MaxQueueLen     int = 50
	MaxQueueDisplay int = 3
	MaxListDisplay  int = 10
)

type Subscription struct {
	ID        string
	queue     []*youtube.Video    // All videos in queue, downloaded or not, needed for displaying queue
	events    chan string         // Event queue
	downloads chan *youtube.Video // To download queue
	tracks    chan *Track         // Downloaded tracks queue
}

type Track struct {
	filename string
	title    string
}

var subscriptions = make(map[string]*Subscription)
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
	voiceChannelID := GetAuthorVoiceChannel(session, msg)
	if voiceChannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}

	if _, ok := subscriptions[voiceChannelID]; ok {
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
	subscriptions[voiceChannelID] = sub
	defer delete(subscriptions, voiceChannelID)
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
	voiceChannelID := GetAuthorVoiceChannel(session, msg)
	if _, ok := subscriptions[voiceChannelID]; !ok {
		return HandlePlay(session, msg, args)
	}
	sub := subscriptions[voiceChannelID]

	err := AddToQueue(session, msg, sub, args[1])
	if err != nil {
		return err
	}
	return nil
}

func HandleList(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	voiceChannelID := GetAuthorVoiceChannel(session, msg)
	if _, ok := subscriptions[voiceChannelID]; !ok {
		session.ChannelMessageSend(msg.ChannelID, "No music playing")
		return nil
	}

	sub := subscriptions[voiceChannelID]
	output := "\\~~\\~~\\~~ Current queue \\~~\\~~\\~~\n"
	numTracks := len(sub.queue)
	max := MaxListDisplay
	if numTracks < max {
		max = numTracks
	}
	for i := 0; i < max; i++ {
		output += fmt.Sprintf("%d - %s\n", i+1, sub.queue[i].Title)
	}
	if numTracks > max {
		output += fmt.Sprintf("...and %d more tracks", numTracks-max)
	}
	session.ChannelMessageSend(msg.ChannelID, output)
	return nil
}

func HandleEvent(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	voiceChannelID := GetAuthorVoiceChannel(session, msg)
	if _, ok := subscriptions[voiceChannelID]; !ok {
		session.ChannelMessageSend(msg.ChannelID, "No music playing")
		return nil
	}
	subscriptions[voiceChannelID].events <- args[0]
	return nil
}

/*
	Add a video or playlist to queue
*/
func AddToQueue(
	session *discordgo.Session,
	msg *discordgo.MessageCreate,
	sub *Subscription,
	url string,
) error {

	if MaxQueueLen-len(sub.queue) < 1 {
		session.ChannelMessageSend(msg.ChannelID, "Max queue length reached")
		return nil
	}
	client := youtube.Client{}
	if playlist, err := client.GetPlaylist(url); err == nil {
		condensedMsg := false
		tracksAdded := 0
		if len(playlist.Videos) > MaxQueueDisplay {
			condensedMsg = true
		}
		for _, item := range playlist.Videos {
			if MaxQueueLen-len(sub.queue) < 1 {
				session.ChannelMessageSend(msg.ChannelID, "Max queue length reached")
				break
			}
			video, err := client.GetVideo(item.ID)
			if err != nil {
				continue
			}
			// Add to queue list and download channel
			sub.queue = append(sub.queue, video)
			sub.downloads <- video
			if !condensedMsg {
				session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf("Adding track [ %s ] to the queue", video.Title))
			} else {
				// Count tracks added for condensed message
				tracksAdded++
			}
		}
		session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf("Added %d tracks to the queue", tracksAdded))
		return nil
	} else if video, err := client.GetVideo(url); err == nil {
		sub.queue = append(sub.queue, video)
		sub.downloads <- video
		session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf("Adding track [ %s ] to the queue", video.Title))
		return nil
	} else {
		return err
	}
}

func DownloadAudio(folder string, video *youtube.Video) (*Track, error) {
	format, err := GetFirstOpusFormat(&video.Formats)
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
func GetAuthorVoiceChannel(session *discordgo.Session, msg *discordgo.MessageCreate) string {
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

func ManageFileQueue(ctx context.Context, sub *Subscription) {
	for {
		// Only download 2 tracks in advance
		for len(sub.tracks) > 1 {
			time.Sleep(time.Millisecond)
		}
		select {
		// Get video metadata from queue and download the audio file
		case video := <-sub.downloads:
			track, err := DownloadAudio(sub.ID, video)
			if err != nil {
				log.Printf("Failed to download file for %s", video.Title)
				continue
			}
			sub.tracks <- track

		case <-ctx.Done():
			log.Println("Closing file download manager")
			return

		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func PlayWebMFileQueue(session *discordgo.Session, msg *discordgo.MessageCreate, vc *discordgo.VoiceConnection, sub *Subscription) error {
	for {
		// Iterate over the queue
		select {
		case track := <-sub.tracks:
			file, err := os.Open(track.filename)
			if err != nil {
				return err
			}

			// Parse webm
			var w webm.WebM
			wr, err := webm.Parse(file, &w)
			if err != nil {
				return err
			}
			session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf("Playing track [ %s ]", track.title))
			// Read opus data from parsed webm and pass into sending channel
			playing := true
			for playing {
				select {
				// Check for events
				case event := <-sub.events:
					switch event {
					case "next":
						playing = false
					case "pause":
						quit := WaitForResume(sub.events)
						if quit {
							return nil
						}
					case "stop":
						return nil
					default:
						continue
					}
				// Send the opus data
				case packet, ok := <-wr.Chan:
					if !ok {
						playing = false
					}
					vc.OpusSend <- packet.Data
				case <-time.After(time.Second):
					playing = false
				}
			}
			// Cleanup file and queue list
			file.Close()
			os.Remove(track.filename)
			sub.queue = sub.queue[1:]
		// Wait 5 seconds after queue is empty in case of delay/adding more tracks
		case <-time.After(5 * time.Second):
			return nil
		}
	}
}

func WaitForResume(ch chan string) bool {
	for {
		select {
		case event := <-ch:
			if event == "resume" {
				return false
			} else if event == "stop" {
				return true
			}
			time.Sleep(time.Millisecond * 500)
		default:
			time.Sleep(time.Millisecond * 500)
		}
	}
}
