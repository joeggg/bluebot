package command

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"fmt"
	"log"
	"os"
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

var Subscriptions = make(map[string]*Subscription)

// Each instance of the bot playing in a voice channel is a "Subscription"
type Subscription struct {
	ID        string
	Queue     []*youtube.Video    // All videos in queue, downloaded or not, needed for displaying queue
	Events    chan string         // Event queue
	Downloads chan *youtube.Video // To download queue
	Tracks    chan *Track         // Downloaded tracks queue
}

// Represents a downloaded track
type Track struct {
	filename string
	title    string
}

func NewSubscription() (*Subscription, error) {
	// Get a random hash as the ID
	buffer := make([]byte, 8)
	_, err := rand.Read(buffer)
	if err != nil {
		return nil, err
	}
	sub := &Subscription{
		ID:        fmt.Sprintf("%x", md5.Sum(buffer)),
		Queue:     []*youtube.Video{},
		Events:    make(chan string),
		Downloads: make(chan *youtube.Video, MaxQueueLen),
		Tracks:    make(chan *Track, MaxQueueLen),
	}
	return sub, nil
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

	if MaxQueueLen-len(sub.Queue) < 1 {
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
			if MaxQueueLen-len(sub.Queue) < 1 {
				session.ChannelMessageSend(msg.ChannelID, "Max queue length reached")
				break
			}
			video, err := client.GetVideo(item.ID)
			if err != nil {
				continue
			}
			// Add to queue list and download channel
			sub.Queue = append(sub.Queue, video)
			sub.Downloads <- video
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
		sub.Queue = append(sub.Queue, video)
		sub.Downloads <- video
		session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf("Adding track [ %s ] to the queue", video.Title))
		return nil
	} else {
		return err
	}
}

func ManageFileQueue(ctx context.Context, sub *Subscription) {
	for {
		// Only download 2 tracks in advance
		for len(sub.Tracks) > 1 {
			time.Sleep(time.Millisecond)
		}
		select {
		// Get video metadata from queue and download the audio file
		case video := <-sub.Downloads:
			track, err := downloadAudio(sub.ID, video)
			if err != nil {
				log.Printf("Failed to download file for %s", video.Title)
				continue
			}
			sub.Tracks <- track

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
		case track := <-sub.Tracks:
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
				case event := <-sub.Events:
					switch event {
					case "next":
						playing = false
					case "pause":
						quit := WaitForResume(sub.Events)
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
				case <-time.After(2 * time.Second):
					playing = false
				}
			}
			// Cleanup file and queue list
			file.Close()
			os.Remove(track.filename)
			sub.Queue = sub.Queue[1:]
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
