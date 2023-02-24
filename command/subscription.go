package command

import (
	"bluebot/config"
	"bluebot/jytdl"
	"bluebot/util"
	"context"
	"crypto/md5"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"google.golang.org/api/option"

	"github.com/bwmarrin/discordgo"
	"github.com/ebml-go/webm"
	"google.golang.org/api/youtube/v3"
)

var (
	MaxQueueLen     int = 30
	MaxQueueDisplay int = 3
	MaxListDisplay  int = 10
)

var Subscriptions = make(map[string]*Subscription)
var UsedIDs = make(map[string]bool)

// Each instance of the bot playing in a voice channel is a "Subscription"
type Subscription struct {
	ID        string      // Unique ID
	Folder    string      // Base folder + ID
	QueueView []*Track    // All videos in queue, downloaded or not, needed for displaying queue
	mu        *sync.Mutex // To prevent race condition on queue append
	Events    chan string // Event queue (user actions such as pause, next etc.)
	Downloads chan *Track // To download queue
	Tracks    chan *Track // Downloaded tracks queue
}

// Represents a downloaded track
type Track struct {
	ID       string
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
		if _, ok := UsedIDs[id]; !ok {
			UsedIDs[id] = true
			break
		}
	}
	sub := &Subscription{
		ID:        id,
		Folder:    config.Cfg.AudioPath + "/" + id,
		mu:        &sync.Mutex{},
		QueueView: []*Track{},
		Events:    make(chan string),
		Downloads: make(chan *Track, MaxQueueLen),
		Tracks:    make(chan *Track, MaxQueueLen),
	}
	return sub, nil
}

/*
Add a video or playlist to the queue and downloads channel. Directly get the metadata and add
to queue if a URL otherwise first search youtube and use the first valid result
*/
func (sub *Subscription) AddToQueue(session *discordgo.Session, chID string, terms []string) {
	if MaxQueueLen-len(sub.QueueView) < 1 {
		session.ChannelMessageSend(chID, "Max queue length reached")
		return
	}

	var err error
	if !strings.Contains(terms[0], "https://") {
		// Not a URL so search youtube for a video/playlist
		items, err := searchYT(strings.Join(terms, " "))
		if err != nil {
			session.ChannelMessageSend(chID, "YouTube search returned no results")
			log.Println(err)
			return
		}
		// Use first result with an ID that can be added
		for i := range items {
			if items[i].Id.VideoId != "" {
				track := &Track{items[i].Id.VideoId, "", items[i].Snippet.Title}
				err = sub.addVideo(session, chID, track, true)

			} else if items[i].Id.PlaylistId != "" {
				err = sub.addPlaylist(session, chID, items[i].Id.PlaylistId)
			}
			if err == nil {
				return
			}
		}
		session.ChannelMessageSend(chID, "YouTube search returned no results")
		log.Println(err)

	} else {
		// Use the given URL
		URL := terms[0]
		if !strings.Contains(URL, "list=") {
			track, err := trackFromURL(URL)
			if err == nil {
				err = sub.addVideo(session, chID, track, true) //lint:ignore SA4006 error is handled
			}
		} else {
			err = sub.addPlaylist(session, chID, strings.Split(URL, "list=")[1])
		}
		if err != nil {
			session.ChannelMessageSend(chID, "Failed to find a download for "+URL)
			log.Println(err)
		}
	}
}

/*
Search youtube for a list of videos or playlists
*/
func searchYT(query string) ([]*youtube.SearchResult, error) {
	ctx := context.Background()
	service, err := youtube.NewService(ctx, option.WithCredentialsFile(config.Cfg.GoogleKeyPath))
	if err != nil {
		return nil, err
	}
	parts := []string{"snippet"}
	results, err := service.Search.List(parts).Q(query).MaxResults(5).Do()
	if err != nil || len(results.Items) == 0 {
		return nil, err
	}
	return results.Items, nil
}

func trackFromURL(URL string) (*Track, error) {
	ctx := context.Background()
	service, err := youtube.NewService(ctx, option.WithCredentialsFile(config.Cfg.GoogleKeyPath))
	if err != nil {
		return nil, err
	}

	parts := []string{"snippet"}
	vid := strings.Split(URL, "v=")[1]
	videos, err := service.Videos.List(parts).Id(vid).Do()
	if err != nil {
		return nil, err
	}
	if len(videos.Items) < 1 {
		return nil, errors.New("no video found for given ID")
	}

	return &Track{vid, "", videos.Items[0].Snippet.Title}, nil
}

/*
Get the list of videos for a playlist and add them to the download queue
*/
func (sub *Subscription) addPlaylist(session *discordgo.Session, chID string, ID string) error {
	ctx := context.Background()
	service, err := youtube.NewService(ctx, option.WithCredentialsFile(config.Cfg.GoogleKeyPath))
	if err != nil {
		return err
	}
	parts := []string{"snippet"}
	maxResults := int64(MaxQueueLen - len(sub.QueueView))
	results, err := service.PlaylistItems.List(parts).PlaylistId(ID).MaxResults(maxResults).Do()
	if err != nil {
		return err
	}

	tracksAdded := 0
	message, _ := session.ChannelMessageSend(chID, "--> Adding playlist to the queue...")
	for _, item := range results.Items {
		track := &Track{item.Snippet.ResourceId.VideoId, "", item.Snippet.Title}
		err := sub.addVideo(session, chID, track, false)
		// Count tracks added for condensed message
		if err == nil {
			tracksAdded++
			session.ChannelMessageEdit(
				chID, message.ID, fmt.Sprintf("--> Added track [ %s ] to the queue", item.Snippet.Title),
			)
		}

	}
	session.ChannelMessageDelete(chID, message.ID)
	session.ChannelMessageSend(chID, fmt.Sprintf("--> Added %d tracks to the queue", tracksAdded))
	return nil
}

/*
Get metadata including audio formats for a video from its URL and add to the
queue & downloads channel
*/
func (sub *Subscription) addVideo(
	session *discordgo.Session, chID string, track *Track, isShowingMessage bool,
) error {
	sub.Downloads <- track
	sub.mu.Lock()
	sub.QueueView = append(sub.QueueView, track)
	sub.mu.Unlock()
	if isShowingMessage {
		session.ChannelMessageSend(chID, fmt.Sprintf("--> Added track [ %s ] to the queue", track.Title))
	}
	return nil
}

/*
Manages downloading and saving tracks using the metadata added to the downloads channel
by the queue manager

Puts the Track object containing the filename on the tracks channel to be picked up by the
playback manager
*/
func (sub *Subscription) ManageDownloads(ctx context.Context) {
	for {
		// Only download 2 tracks in advance
		for len(sub.Tracks) > 1 {
			time.Sleep(time.Second)
		}
		select {
		// Get video metadata from queue and download the audio file
		case track := <-sub.Downloads:
			err := downloadAudio(sub.Folder, track)
			if err != nil {
				log.Printf("Failed to download file for %s", track.ID)
				log.Println(err)
				sub.removeQueueItem(track)
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
Download a youtube video's audio to a WebM file with opus audio.
Sets the output filename on the track object
*/
func downloadAudio(folder string, track *Track) error {
	log.Printf("Downloading audio file for %s\n", track.ID)
	// Create unique file name
	randHex, err := util.RandomHex(4)
	if err != nil {
		return err
	}
	filename := fmt.Sprintf("%s/%s-%s.weba", folder, track.ID, randHex)
	// Download
	err = jytdl.GetAudio(track.ID, filename, "audio/webm")
	if err != nil {
		return err
	}
	log.Printf("Finished downloading for %s\n", track.ID)
	track.Filename = filename
	return nil
}

func (sub *Subscription) removeQueueItem(track *Track) {
	// Remove element from queue view
	for i, item := range sub.QueueView {
		if item.ID == track.ID {
			sub.mu.Lock()
			sub.QueueView = append(sub.QueueView[:i], sub.QueueView[i+1:]...)
			sub.mu.Unlock()
			break
		}
	}
}

/*
Play over the dowloaded tracks from the track channel by parsing the WebM and directly sending
the opus packets, deleting each track's file after it's finished

# Accepts control events through the events channel

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
			session.ChannelMessageSend(chID, fmt.Sprintf("--> Playing track [ %s ]", track.Title))
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
			sub.QueueView = sub.QueueView[1:]
			sub.mu.Unlock()
		// Wait 20 seconds after queue is empty
		case <-time.After(60 * time.Second):
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
