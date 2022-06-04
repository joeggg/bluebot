package main

import (
	"bluebot/command"
	"bluebot/config"
	"bluebot/util"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Mapping of commands to handler functions
var commands = map[string]util.HandlerFunc{
	"tell": command.HandleTell,
	"say":  command.HandleSay,
	"yt":   command.HandleYT,
}

func MessageHandler(session *discordgo.Session, msg *discordgo.MessageCreate) {
	if msg.Author.ID == session.State.User.ID {
		return
	}
	// Check for prefix and split msg into command and args
	if !strings.HasPrefix(msg.Content, "%") {
		return
	}
	message_list := strings.Split(msg.Content[1:], " ")
	command, args := message_list[0], message_list[1:]
	log.Printf("Recevied command: %s with args: %s", command, args)

	// Check command exists and run its handler if so
	if handler, ok := commands[command]; ok {
		start := time.Now()
		err := handler(session, msg, args)
		log.Printf("%s took %s", command, time.Since(start))
		// Any errors returned cause internal server error message
		// Handle errors more specifically if needed within the handler function
		if err != nil {
			session.ChannelMessageSend(msg.ChannelID, "A fatal internal error occurred")
			log.Printf("Command %s failed with error: %s", command, err)
		}
	}
}

// Main entry point: start discord-go client and wait for messages
func main() {
	log.Println("Hello")
	token, err := config.ReadDiscordToken()
	if err != nil {
		return
	}
	// Remove old stored audio from ungraceful shutdown
	dirs, err := ioutil.ReadDir(config.AudioPath)
	if err != nil {
		return
	}
	for _, dir := range dirs {
		os.RemoveAll(config.AudioPath + "/" + dir.Name())
	}

	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Println("Failed to create discord client")
		return
	}

	err = discord.Open()
	if err != nil {
		log.Println("Failed to open connection to Discord")
		return
	}

	discord.AddHandler(MessageHandler)

	log.Println("We're connected boyaa")

	// Wait for OS signal through channel before closing main loop
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	discord.Close()
	// Remove stored audio
	err = os.RemoveAll(config.AudioPath + "/*")
	if err != nil {
		return
	}
}
