package core

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
var usedIDs = make(map[string]bool)

// Represents a downloaded track
type Track struct {
	ID       string
	Filename string
	Title    string
}

// Each instance of the bot playing in a voice channel is a "Subscription"
type Subscription struct {
	id          string      // Unique ID
	folder      string      // Base folder + ID
	queueView   []*Track    // All videos in queue, downloaded or not, needed for displaying queue
	mu          sync.Mutex  // To prevent race condition on queue append
	nextTrigger chan bool   // Event queue (user actions such as pause, next etc.)
	downloads   chan *Track // To download queue
	tracks      chan *Track // Downloaded tracks queue
	running     bool
	container   *Container
}

func NewSubscription(container *Container) VoiceApp {
	return &Subscription{
		container:   container,
		nextTrigger: make(chan bool),
		downloads:   make(chan *Track, MaxQueueLen),
		tracks:      make(chan *Track, MaxQueueLen),
	}
}

func (sub *Subscription) Container() *Container {
	return sub.container
}

func (sub *Subscription) SendEvent(event string, args []string, msgChannelID string) error {
	switch event {
	case "queue":
		sub.addToQueue(msgChannelID, args)
	case "list":
		return sub.listQueue(msgChannelID)
	case "next":
		// in case paused currently
		sub.container.TryResume()
		sub.nextTrigger <- true
	case "pause":
		sub.container.TryPause()
	case "resume":
		sub.container.TryResume()
	case "stop":
		sub.container.cancel()
	default:
		return fmt.Errorf("Unknown event %s", event)
	}
	return nil
}

func (sub *Subscription) Run(channelID string) error {
	log.Printf("Created subscription %s for user %s", sub.id, sub.container.vc.ChannelID)
	defer sub.container.session.ChannelMessageSend(channelID, "Stopping playing")
	defer log.Printf("Removing subscription for user %s", sub.container.vc.ChannelID)
	// Get a random hash as the ID (that isn't in use)
	var id string
	for {
		buffer := make([]byte, 4)
		_, err := rand.Read(buffer)
		if err != nil {
			return err
		}
		id = fmt.Sprintf("%x", md5.Sum(buffer))
		if _, ok := usedIDs[id]; !ok {
			usedIDs[id] = true
			break
		}
	}
	sub.id = id
	sub.folder = config.Cfg.AudioPath + "/" + id
	// Make folder for files
	if err := os.Mkdir(sub.folder, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	defer os.RemoveAll(sub.folder)

	go sub.ManageDownloads()
	go sub.ManagePlayback(channelID)
	defer sub.container.cancel()

	start := time.Now()
	for {
		select {
		case <-sub.container.ctx.Done():
			return nil

		case <-time.After(2 * time.Second):
			// Wait for 1 track at least downloaded
			if len(sub.queueView) == 0 && time.Since(start) > 60*time.Second {
				// Nothing was added
				log.Printf("No new tracks for a while for channel %s", channelID)
				return nil
			}
			if time.Since(start) > 12*time.Hour {
				return nil
			}
		}
	}
}

/*
Add a video or playlist to the queue and downloads channel. Directly get the metadata and add
to queue if a URL otherwise first search youtube and use the first valid result
*/
func (sub *Subscription) addToQueue(chID string, terms []string) {
	if MaxQueueLen-len(sub.queueView) < 1 {
		sub.container.session.ChannelMessageSend(chID, "Max queue length reached")
		return
	}

	var err error
	if !strings.Contains(terms[0], "https://") {
		// Not a URL so search youtube for a video/playlist
		items, err := searchYT(strings.Join(terms, " "))
		if err != nil {
			sub.container.session.ChannelMessageSend(chID, "YouTube search returned no results")
			log.Println(err)
			return
		}
		// Use first result with an ID that can be added
		for i := range items {
			if items[i].Id.VideoId != "" {
				track := &Track{items[i].Id.VideoId, "", items[i].Snippet.Title}
				err = sub.addVideo(sub.container.session, chID, track, true)

			} else if items[i].Id.PlaylistId != "" {
				err = sub.addPlaylist(sub.container.session, chID, items[i].Id.PlaylistId)
			}
			if err == nil {
				return
			}
		}
		sub.container.session.ChannelMessageSend(chID, "YouTube search returned no results")
		log.Println(err)

	} else {
		// Use the given URL
		URL := terms[0]
		if !strings.Contains(URL, "list=") {
			track, err := trackFromURL(URL)
			if err == nil {
				err = sub.addVideo(sub.container.session, chID, track, true)
			}
		} else {
			err = sub.addPlaylist(sub.container.session, chID, strings.Split(URL, "list=")[1])
		}
		if err != nil {
			sub.container.session.ChannelMessageSend(chID, "Failed to find a download for "+URL)
			log.Println(err)
		}
	}
}

func (sub *Subscription) listQueue(msgChannelID string) error {
	output := "\\~~\\~~\\~~\\~~\\~~\\~~ Current queue \\~~\\~~\\~~\\~~\\~~\\~~\n"
	numTracks := len(sub.queueView)
	max := MaxListDisplay
	if numTracks < max {
		max = numTracks
	}
	for i := 0; i < max; i++ {
		output += fmt.Sprintf("%d - %s", i+1, sub.queueView[i].Title)
		if i == 0 {
			output += " <--\n"
		} else {
			output += "\n"
		}
	}
	if numTracks > max {
		output += fmt.Sprintf("...and %d more tracks", numTracks-max)
	}
	sub.container.session.ChannelMessageSend(msgChannelID, output)
	return nil
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
	maxResults := int64(MaxQueueLen - len(sub.queueView))
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
	sub.downloads <- track
	sub.mu.Lock()
	sub.queueView = append(sub.queueView, track)
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
func (sub *Subscription) ManageDownloads() {
	for {
		// // Only download 2 tracks in advance
		// for len(sub.Tracks) > 1 {
		// 	time.Sleep(time.Second)
		// }
		select {
		case <-sub.container.ctx.Done():
			log.Println("Closing file download manager")
			return

		// Get video metadata from queue and download the audio file
		case track := <-sub.downloads:
			err := downloadAudio(sub.folder, track)
			if err != nil {
				log.Printf("Failed to download file for %s", track.ID)
				log.Println(err)
				sub.removeQueueItem(track)
				continue
			}
			sub.tracks <- track

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
	for i, item := range sub.queueView {
		if item.ID == track.ID {
			sub.mu.Lock()
			sub.queueView = append(sub.queueView[:i], sub.queueView[i+1:]...)
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
func (sub *Subscription) ManagePlayback(chID string) {
	log.Printf("Starting playing for user %s", sub.container.vc.UserID)
	for {
		// Iterate over the Tracks channel
		select {
		case <-sub.container.ctx.Done():
			return

		case track := <-sub.tracks:

			file, err := os.Open(track.Filename)
			if err != nil {
				log.Printf(
					"An error occurred opening [ %s ] for subscription %s: %s", track.Title, sub.id, err,
				)
				sub.container.session.ChannelMessageSend(
					chID, fmt.Sprintf("Failed to play [ %s ], skipping", track.Title),
				)
				continue
			}

			// Parse webm
			var w webm.WebM
			wr, err := webm.Parse(file, &w)
			if err != nil {
				log.Printf(
					"An error occurred parsing [ %s ] for subscription %s: %s", track.Title, sub.id, err,
				)
				sub.container.session.ChannelMessageSend(
					chID, fmt.Sprintf("Failed to play [ %s ], skipping", track.Title),
				)
			}
			sub.container.session.ChannelMessageSend(
				chID, fmt.Sprintf("--> Playing track [ %s ]", track.Title),
			)
			log.Printf("Playing track [ %s ] for subscription %s", track.Title, sub.id)
			// Read opus data from parsed webm and pass into sending channel
			sub.container.AcquirePlayLock()
			playing := true
			for playing {
				select {
				case <-sub.container.ctx.Done():
					return
				// Check for play requests
				case <-*sub.container.pauseRequests:
					sub.container.WaitForResume()
					// Next track
				case <-sub.nextTrigger:
					playing = false
				// Send the opus data
				case packet, ok := <-wr.Chan:
					if !ok {
						playing = false
					}
					sub.container.vc.OpusSend <- packet.Data
				// Move on after 2 seconds of no packets
				case <-time.After(2 * time.Second):
					log.Printf("Failed to read any packets for subscription %s", sub.id)
					playing = false
				}
			}
			sub.container.ReleasePlayLock()
			// Cleanup file and queue list
			file.Close()
			os.Remove(track.Filename)
			sub.mu.Lock()
			sub.queueView = sub.queueView[1:]
			sub.mu.Unlock()

		}
	}
}
