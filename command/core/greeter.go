package core

import (
	"log"
	"time"
)

type Greeter struct {
	running        bool
	command        chan string
	voiceSelection string
}

func NewGreeter() *Greeter {
	return &Greeter{command: make(chan string)}
}

func (g *Greeter) SendEvent(event string, args []string, channelID string) error {
	g.command <- event
	return nil
}

func (g *Greeter) Run(container *Container, channelID string) error {
	select {
	case command := <-g.command:
		err := PlayText(command, "bluebot", container)
		if err != nil {
			log.Fatalln(err)
		}
	case <-time.After(time.Minute):
	}
	return nil
}
