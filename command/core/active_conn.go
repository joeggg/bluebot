package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

var activeConnections = make(map[string]*ActiveConnection)

type App interface {
	SendEvent(event string, args []string, channelID string) error
	Run(*Container, string) error
}

// Resources required by an app to run
type Container struct {
	app_name            string
	session             *discordgo.Session
	vc                  *discordgo.VoiceConnection
	mu                  *sync.Mutex
	ctx                 context.Context
	cancel              context.CancelFunc
	playingNotification *chan string
	pauseRequests       *chan bool // for requesting what's playing to stop directly
	resumeRequests      *chan bool // for requesting the conn to pause everything
	resumeRecveiver     chan bool  // for receiving a resume signal from the conn
}

/*
Try pausing with a timeout in case nothing is playing
*/
func (c *Container) TryPause() {
	select {
	case *c.pauseRequests <- true:
	case <-time.After(10 * time.Millisecond):
	}
}

/*
Try resume this container's app with a timeout in case it's playing
*/
func (c *Container) TryResume() {
	select {
	case c.resumeRecveiver <- true:
	case <-time.After(10 * time.Millisecond):
	}
}

/*
Try resume last running app with a timeout in case it's playing
*/
func (c *Container) TryResumeLast() {
	select {
	case *c.resumeRequests <- true:
	case <-time.After(10 * time.Millisecond):
	}
}

/*
Acquire the lock to begin outputting sound
*/
func (c *Container) AcquirePlayLock() {
	c.TryPause()
	c.mu.Lock()
	c.vc.Speaking(true)
}

/*
Release the lock after finishing outputting
*/
func (c *Container) ReleasePlayLock() {
	c.mu.Unlock()
	c.vc.Speaking(false)
	c.TryResumeLast()
}

/*
Run this on receiving a pause request, will relinquish control unil a resume request is received
*/
func (c *Container) WaitForResume() {
	// Set to speaking as will be unset when the app we're waiting for finishes
	*c.playingNotification <- c.app_name
	c.mu.Unlock()
	c.vc.Speaking(false)
	defer c.vc.Speaking(true)
	// Lock even if done so release of PlayLock still works
	defer c.mu.Lock()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.resumeRecveiver:
			return
		}
	}
}

type ActiveConnection struct {
	container *Container
	mu        sync.Mutex
	wg        sync.WaitGroup
	// Apps
	apps          map[string]App
	appContainers map[string]*Container
	lastPlaying   string
}

func NewActiveConnection(
	session *discordgo.Session, vc *discordgo.VoiceConnection,
) *ActiveConnection {
	ctx, cancel := context.WithCancel(context.Background())
	pauseReqs := make(chan bool)
	resumeReqs := make(chan bool)
	playingNotification := make(chan string)
	return &ActiveConnection{
		container: &Container{
			session:             session,
			vc:                  vc,
			mu:                  &sync.Mutex{},
			ctx:                 ctx,
			cancel:              cancel,
			pauseRequests:       &pauseReqs,
			resumeRequests:      &resumeReqs,
			playingNotification: &playingNotification,
		},
		apps: map[string]App{
			"sub":     NewSubscription(),
			"conv":    NewConversation(),
			"greeter": NewGreeter(),
		},
		appContainers: make(map[string]*Container),
	}
}

/*
Start the active voice connection, returns an error if unable to connect to the voice channel
*/
func GetActiveConnection(
	session *discordgo.Session, guildID string, voiceChannelID string, authorID string,
) (*ActiveConnection, error) {
	if voiceChannelID == "" {
		voiceChannelID := getAuthorVoiceChannel(session, guildID, authorID)
		if voiceChannelID == "" {
			return nil, errors.New("User not in a voice channel")
		}
	}

	if conn, ok := activeConnections[voiceChannelID]; ok {
		return conn, nil
	}
	// Join voice channel and start websocket audio communication
	vc, err := session.ChannelVoiceJoin(guildID, voiceChannelID, false, true)
	if err != nil {
		return nil, err
	}

	conn := NewActiveConnection(session, vc)
	activeConnections[voiceChannelID] = conn
	go conn.run()
	return conn, nil
}

func CloseActiveConnections() {
	wg := sync.WaitGroup{}
	for _, conn := range activeConnections {
		wg.Add(1)

		go func(c *ActiveConnection) {
			defer wg.Done()
			c.ShutDown()
		}(conn)
	}
	wg.Wait()
}

func (conn *ActiveConnection) run() {
	log.Printf("Starting new active connection for vc %s", conn.container.vc.ChannelID)
	defer log.Printf("Closed active connection for vc %s", conn.container.vc.ChannelID)
	defer delete(activeConnections, conn.container.vc.ChannelID)
	conn.wg.Add(1)
	defer conn.wg.Done()

	defer conn.container.vc.Disconnect()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for {
		// Listen for shutdown signal
		select {
		case <-ctx.Done():
			return

		case app_name := <-*conn.container.playingNotification:
			conn.lastPlaying = app_name

		case <-*conn.container.resumeRequests:
			container := conn.getAppContainer(conn.lastPlaying)
			if container != nil {
				container.TryResume()
			}

		case <-time.After(500 * time.Millisecond):
			// Shut down if no apps are running
			if len(conn.appContainers) == 0 {
				return
			}
		}
	}
}

func (a *ActiveConnection) ShutDown() {
	a.container.cancel()
	a.wg.Wait()
}

func (conn *ActiveConnection) SendEvent(appName string, event string, msgChannelID string) error {
	return conn.SendEventWithArgs(appName, event, msgChannelID, []string{})
}

func (conn *ActiveConnection) SendEventWithArgs(appName string, event string, msgChannelID string, args []string) error {
	app, ok := conn.apps[appName]
	if !ok {
		return fmt.Errorf("Received command for unknown app: %s", appName)
	}

	if conn.getAppContainer(appName) == nil {
		appCtx, appCancel := context.WithCancel(conn.container.ctx)
		appContainer := &Container{
			app_name:            appName,
			session:             conn.container.session,
			vc:                  conn.container.vc,
			mu:                  conn.container.mu,
			ctx:                 appCtx,
			cancel:              appCancel,
			pauseRequests:       conn.container.pauseRequests,
			resumeRequests:      conn.container.resumeRequests,
			resumeRecveiver:     make(chan bool),
			playingNotification: conn.container.playingNotification,
		}
		conn.insertAppContainer(appName, appContainer)
		conn.wg.Add(1)

		go func() {
			log.Printf("Starting app %s for connection %s", appName, conn.container.vc.ChannelID)

			defer conn.wg.Done()
			defer conn.deleteAppContainer(appName)
			// Run spans the 'lifetime' of the app
			err := app.Run(appContainer, msgChannelID)
			if err != nil {
				log.Println(err)
			}

			log.Printf("Closed app %s for connection %s", appName, conn.container.vc.ChannelID)
		}()

	}
	return app.SendEvent(event, args, msgChannelID)
}

func (conn *ActiveConnection) insertAppContainer(appName string, container *Container) {
	conn.mu.Lock()
	conn.appContainers[appName] = container
	conn.mu.Unlock()
}

func (conn *ActiveConnection) deleteAppContainer(appName string) {
	conn.mu.Lock()
	delete(conn.appContainers, appName)
	conn.mu.Unlock()
}

func (conn *ActiveConnection) getAppContainer(appName string) *Container {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	container, ok := conn.appContainers[appName]
	if ok && container != nil {
		return container
	}
	return nil
}

/*
Find if a the message author is in a channel
*/
func getAuthorVoiceChannel(session *discordgo.Session, guildID string, authorID string) string {
	// Find sender's voice channel
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return ""
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == authorID {
			return vs.ChannelID
		}
	}
	return ""
}
