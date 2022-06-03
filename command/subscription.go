package command

import (
	"bluebot/config"
	"context"
	"crypto/md5"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/option"

	"github.com/bwmarrin/discordgo"
	"github.com/ebml-go/webm"
	ytdl "github.com/kkdai/youtube/v2"
	"google.golang.org/api/youtube/v3"
)

var (
	MaxQueueLen     int = 50
	MaxQueueDisplay int = 3
	MaxListDisplay  int = 10
)

var Subscriptions = make(map[string]*Subscription)
var usedIDs = make(map[string]bool)

// Each instance of the bot playing in a voice channel is a "Subscription"
type Subscription struct {
	ID        string           // Unique ID
	Queue     []*ytdl.Video    // All videos in queue, downloaded or not, needed for displaying queue
	mu        *sync.Mutex      // To prevent race condition on queue append
	Events    chan string      // Event queue
	Downloads chan *ytdl.Video // To download queue
	Tracks    chan *Track      // Downloaded tracks queue
}

// Represents a downloaded track
type Track struct {
	Filename string
	Title    string
}

func NewSubscription() (*Subscription, error) {
	// Get a random hash as the ID (that isn't in use)
	var id string
	for {
		buffer := make([]byte, 4)
		_, err := rand.Read(buffer)
		if err != nil {
			return nil, err
		}
		id = fmt.Sprintf("%x", md5.Sum(buffer))
		if _, ok := usedIDs[id]; !ok {
			usedIDs[id] = true
			break
		}
	}
	sub := &Subscription{
		ID:        id,
		mu:        &sync.Mutex{},
		Queue:     []*ytdl.Video{},
		Events:    make(chan string),
		Downloads: make(chan *ytdl.Video, MaxQueueLen),
		Tracks:    make(chan *Track, MaxQueueLen),
	}
	return sub, nil
}

/*
	Request metadata for a video or series of videos from a playlist and add to the
	downloads channel
*/
func (sub *Subscription) AddToQueue(session *discordgo.Session, chID string, terms []string) {
	if MaxQueueLen-len(sub.Queue) < 1 {
		session.ChannelMessageSend(chID, "Max queue length reached")
		return
	}
	if !strings.Contains(terms[0], "https://") {
		// Search youtube for an ID
		ctx := context.Background()
		service, err := youtube.NewService(ctx, option.WithCredentialsFile(config.GoogleKeyPath))
		if err != nil {
			session.ChannelMessageSend(chID, "Couldn't add to the queue")
			log.Println(err)
			return
		}
		parts := []string{"snippet"}
		results, err := service.Search.List(parts).Q(strings.Join(terms, "")).MaxResults(5).Do()
		if err != nil {
			session.ChannelMessageSend(chID, "Couldn't add to the queue")
			log.Println(err)
			return
		}
		if len(results.Items) == 0 {
			session.ChannelMessageSend(chID, "YouTube search returned no results")
			return
		}

		firstItem := results.Items[0]
		if videoID := firstItem.Id.VideoId; videoID != "" {
			sub.addVideo(session, chID, videoID)
		} else if playlistID := firstItem.Id.PlaylistId; playlistID != "" {
			sub.addPlaylist(session, chID, playlistID)
		} else {
			session.ChannelMessageSend(chID, "Couldn't find anything on YouTube search")
		}
	} else {
		fmt.Println("hi")
		// Use the given URL
		URL := terms[0]
		if !strings.Contains(URL, "list=") {
			sub.addVideo(session, chID, URL)
		} else {
			sub.addPlaylist(session, chID, URL)
		}
	}
}

func (sub *Subscription) addPlaylist(session *discordgo.Session, chID string, URL string) {
	client := ytdl.Client{}
	playlist, err := client.GetPlaylist(URL)
	if err != nil {
		session.ChannelMessageSend(chID, "Failed to find a download for the link given")
		return
	}
	condensedMsg := false
	tracksAdded := 0
	// Show single condensed message if too many tracks
	if len(playlist.Videos) > MaxQueueDisplay {
		condensedMsg = true
	}
	for _, item := range playlist.Videos {
		if MaxQueueLen-len(sub.Queue) < 1 {
			session.ChannelMessageSend(chID, "Max queue length reached")
			break
		}
		video, err := client.GetVideo(item.ID)
		if err != nil { // Skip broken videos
			continue
		}
		// Add to queue list and download channel
		sub.mu.Lock()
		sub.Queue = append(sub.Queue, video)
		sub.mu.Unlock()
		sub.Downloads <- video
		if !condensedMsg {
			session.ChannelMessageSend(chID, fmt.Sprintf("Added track [ %s ] to the queue", video.Title))
		} else {
			// Count tracks added for condensed message
			tracksAdded++
		}
	}
	session.ChannelMessageSend(chID, fmt.Sprintf("Added %d tracks to the queue", tracksAdded))
}

func (sub *Subscription) addVideo(session *discordgo.Session, chID string, URL string) {
	client := ytdl.Client{}
	video, err := client.GetVideo(URL)
	if err != nil {
		session.ChannelMessageSend(chID, "Failed to find a download for the link given")
		return
	}
	sub.mu.Lock()
	sub.Queue = append(sub.Queue, video)
	sub.mu.Unlock()
	sub.Downloads <- video
	session.ChannelMessageSend(chID, fmt.Sprintf("Added track [ %s ] to the queue", video.Title))
}

/*
	Manages downloading and saving tracks using the metadata added to the downloads channel
	by the info manager

	Puts the Track object containing the filename on the tracks channel to be picked up by the
	playback manager
*/
func (sub *Subscription) ManageDownloads(ctx context.Context) {
	for {
		// Only download 2 tracks in advance
		for len(sub.Tracks) > 1 {
			time.Sleep(500 * time.Millisecond)
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
			time.Sleep(500 * time.Millisecond)
		}
	}
}

/*
	Play over the dowloaded tracks from the track channel by parsing the WebM and directly sending
	the opus packets, deleting each track's file after it's finished

	Accepts control events through the events channel

	Waits for a bit when the track channel is empty before closing in case download is slow.
	There's a long timeout on the parsed WebM channel for a similar reason
*/
func (sub *Subscription) ManagePlayback(session *discordgo.Session, chID string, vc *discordgo.VoiceConnection) error {
	for {
		// Iterate over the Tracks channel
		select {
		case track := <-sub.Tracks:
			file, err := os.Open(track.Filename)
			if err != nil {
				return err
			}

			// Parse webm
			var w webm.WebM
			wr, err := webm.Parse(file, &w)
			if err != nil {
				return err
			}
			session.ChannelMessageSend(chID, fmt.Sprintf("Playing track [ %s ]", track.Title))
			log.Printf("Playing track [ %s ] for subscription %s", track.Title, sub.ID)
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
			os.Remove(track.Filename)
			sub.mu.Lock()
			sub.Queue = sub.Queue[1:]
			sub.mu.Unlock()
		// Wait 20 seconds after queue is empty
		case <-time.After(20 * time.Second):
			return nil
		}
	}
}

/*
	Wait for an event through the channel to end the pause.
	Returns true if we need to stop rather than just resume
*/
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

/*
	Download a youtube video's audio to a WebM file with opus audio and return a Track object
*/
func downloadAudio(folder string, video *ytdl.Video) (*Track, error) {
	format, err := getFirstOpusFormat(&video.Formats)
	if err != nil {
		return nil, err
	}

	log.Println("Downloading audio file")
	client := ytdl.Client{}
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
func getFirstOpusFormat(formats *ytdl.FormatList) (*ytdl.Format, error) {
	var format ytdl.Format
	for _, format := range *formats {
		if format.AudioChannels > 0 && strings.Contains(format.MimeType, "opus") {
			return &format, nil
		}
	}
	return &format, errors.New("no format could be found")
}
