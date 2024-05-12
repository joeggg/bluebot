package core

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const tryWaitTime = 100 * time.Millisecond

// Resources required by an app to run
type Container struct {
	appName appType
	// Shared between all a single conn's containers by ref
	session             *discordgo.Session
	vc                  *discordgo.VoiceConnection
	mu                  *sync.Mutex   // output lock
	playingNotification *chan appType // to notify the conn we were the last playing
	pauseRequests       *chan bool    // for requesting what's playing to stop directly
	resumeRequests      *chan bool    // for requesting the conn to resume what was last playing
	// Unique per app container
	ctx             context.Context
	cancel          context.CancelFunc
	resumeRecveiver chan bool // for receiving a resume signal from the conn
}

/*
Try pausing with a timeout in case nothing is playing
*/
func (c *Container) TryPause() {
	select {
	case *c.pauseRequests <- true:
	case <-time.After(tryWaitTime):
	}
}

/*
Try resume this container's app with a timeout in case it's playing
*/
func (c *Container) TryResume() {
	select {
	case c.resumeRecveiver <- true:
	case <-time.After(tryWaitTime):
	}
}

/*
Try resume last running app with a timeout in case it's playing
*/
func (c *Container) TryResumeLast() {
	select {
	case *c.resumeRequests <- true:
	case <-time.After(tryWaitTime):
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
	*c.playingNotification <- c.appName
	c.mu.Unlock()
	c.vc.Speaking(false)
	// Set to speaking as will be unset when the app we're waiting for finishes
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

type appType = int

const (
	MusicApp appType = iota
	AiApp
	GreeterApp
)

// Respresents anything running requiring a voice connection
type VoiceApp interface {
	Container() *Container
	SendEvent(event string, args []string, channelID string) error
	Run(string) error
}

type VoiceAppConstructor = func(*Container) VoiceApp

// A connection to a given voice channel, managing an arbitrary number of apps running at once
// Shuts down and clears its memory when all apps have finished
type ActiveConnection struct {
	container *Container
	mu        sync.Mutex
	wg        sync.WaitGroup
	// Apps
	appConstructors map[appType]VoiceAppConstructor
	apps            map[appType]VoiceApp
	lastPlaying     appType
}

var activeConnections = make(map[string]*ActiveConnection)

func NewActiveConnection(
	session *discordgo.Session, vc *discordgo.VoiceConnection,
) *ActiveConnection {
	ctx, cancel := context.WithCancel(context.Background())
	pauseReqs := make(chan bool)
	resumeReqs := make(chan bool)
	playingNotification := make(chan appType)

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
		appConstructors: map[appType]VoiceAppConstructor{
			MusicApp:   NewSubscription,
			AiApp:      NewConversation,
			GreeterApp: NewGreeter,
		},
		apps: make(map[appType]VoiceApp),
	}
}

func GetActiveConnectionByAuthor(
	session *discordgo.Session, guildID string, authorID string,
) (*ActiveConnection, error) {
	// Find sender's voice channel
	voiceChannelID := ""
	guild, err := session.State.Guild(guildID)
	if err != nil {
		return nil, err
	}
	for _, vs := range guild.VoiceStates {
		if vs.UserID == authorID {
			voiceChannelID = vs.ChannelID
		}
	}
	if voiceChannelID == "" {
		return nil, errors.New("User not in a voice channel")
	}
	return GetActiveConnection(session, guildID, voiceChannelID)
}

/*
Start the active voice connection, returns an error if unable to connect to the voice channel
*/
func GetActiveConnection(
	session *discordgo.Session, guildID string, voiceChannelID string,
) (*ActiveConnection, error) {
	if conn, ok := activeConnections[voiceChannelID]; ok {
		return conn, nil
	}
	// Join voice channel and start websocket audio communication
	vc, err := session.ChannelVoiceJoin(guildID, voiceChannelID, false, false)
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
			if len(conn.apps) == 0 {
				return
			}
		}
	}
}

func (a *ActiveConnection) ShutDown() {
	a.container.cancel()
	a.wg.Wait()
}

func (conn *ActiveConnection) SendEvent(appName appType, event string, msgChannelID string) error {
	return conn.SendEventWithArgs(appName, event, msgChannelID, []string{})
}

func (conn *ActiveConnection) SendEventWithArgs(
	appName appType, event string, msgChannelID string, args []string,
) error {
	app, ok := conn.apps[appName]
	if !ok {
		appCtx, appCancel := context.WithCancel(conn.container.ctx)
		appContainer := &Container{
			appName:             appName,
			session:             conn.container.session,
			vc:                  conn.container.vc,
			mu:                  conn.container.mu,
			playingNotification: conn.container.playingNotification,
			pauseRequests:       conn.container.pauseRequests,
			resumeRequests:      conn.container.resumeRequests,
			ctx:                 appCtx,
			cancel:              appCancel,
			resumeRecveiver:     make(chan bool),
		}
		app = conn.appConstructors[appName](appContainer)
		conn.insertApp(appName, app)
		conn.wg.Add(1)

		go func() {
			log.Printf("Starting app %d for connection %s", appName, conn.container.vc.ChannelID)

			defer conn.wg.Done()
			defer conn.deleteApp(appName)
			// Run spans the 'lifetime' of the app
			err := app.Run(msgChannelID)
			if err != nil {
				log.Println(err)
			}

			log.Printf("Closed app %d for connection %s", appName, conn.container.vc.ChannelID)
		}()

	}
	return app.SendEvent(event, args, msgChannelID)
}

func (conn *ActiveConnection) insertApp(appName appType, app VoiceApp) {
	conn.mu.Lock()
	conn.apps[appName] = app
	conn.mu.Unlock()
}

func (conn *ActiveConnection) deleteApp(appName appType) {
	conn.mu.Lock()
	delete(conn.apps, appName)
	conn.mu.Unlock()
}

func (conn *ActiveConnection) getAppContainer(appName appType) *Container {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	app, ok := conn.apps[appName]
	if ok && app != nil {
		return app.Container()
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
