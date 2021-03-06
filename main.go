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
	"civ":      command.HandleCiv,
	"tell":     command.HandleTell,
	"say":      command.HandleSay,
	"setvoice": command.HandleSetVoice,
	"yt":       command.HandleYT,
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
	} else {
		session.ChannelMessageSend(msg.ChannelID, config.GetPhrase("wrongcommand"))
	}
}

// Main entry point: start discord-go client and wait for messages
func main() {
	err := config.LoadConfig()
	if err != nil {
		log.Fatalln(err)
	}
	err = config.SetupLogging()
	if err != nil {
		log.Fatalf("Error setting up log file: %v", err)
	}

	token, err := config.ReadDiscordToken()
	if err != nil {
		log.Fatalf("Failed to open Discord token file: %s", err)
	}
	// Remove old stored audio from ungraceful shutdown
	dirs, err := ioutil.ReadDir(config.Cfg.AudioPath)
	if err != nil {
		log.Fatalf("Failed to read audio path: %s", err)
	}
	for _, dir := range dirs {
		os.RemoveAll(config.Cfg.AudioPath + "/" + dir.Name())
	}

	discord, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalln("Failed to create discord client")
	}

	err = discord.Open()
	if err != nil {
		log.Fatalln("Failed to open connection to Discord")
	}

	discord.AddHandler(MessageHandler)

	log.Println("bluebot is ready to rumble")

	// Wait for OS signal through channel before closing main loop
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
	discord.Close()
	// Remove stored audio
	err = os.RemoveAll(config.Cfg.AudioPath + "/*")
	if err != nil {
		log.Fatalln("Failed to remove temporary audio folder")
	}
}
