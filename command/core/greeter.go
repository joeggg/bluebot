package core

import (
	"log"
	"time"
)

type Greeter struct {
	container      *Container
	c              chan string
	running        bool
	voiceSelection string
}

func NewGreeter(container *Container) VoiceApp {
	return &Greeter{container: container, c: make(chan string)}
}

func (g *Greeter) Container() *Container {
	return g.container
}

func (g *Greeter) SendEvent(event string, args []string, channelID string) error {
	g.c <- event
	return nil
}

func (g *Greeter) Run(channelID string) error {
	select {
	case command := <-g.c:
		err := PlayGlobalText(command, "bluebot", g.container)
		if err != nil {
			log.Println(err)
		}
	case <-time.After(time.Minute):
	}
	return nil
}
