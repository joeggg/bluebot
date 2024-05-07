package core

import "time"

type Greeter struct {
	running        bool
	command        chan string
	voiceSelection string
}

func NewGreeter() *Greeter {
	return &Greeter{}
}

func (g *Greeter) IsRunning() bool {
	return g.running
}

func (g *Greeter) SetRunning(running bool) {
	g.running = running
}

func (g *Greeter) SendEvent(event string, args []string, channelID string) error {
	g.command <- event
	return nil
}

func (g *Greeter) Run(container *Container, channelID string) error {
	select {
	case command := <-g.command:
		PlayText(command, "bluebot", container)
	case <-time.After(time.Minute):
	}
	return nil
}
