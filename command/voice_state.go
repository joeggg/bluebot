package command

import (
	"bluebot/command/core"
	"bluebot/config"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var numUsers = map[string]int{}

func HandleVoiceState(session *discordgo.Session, msg *discordgo.VoiceStateUpdate) error {
	// Get the member of the voice state update
	user, err := session.GuildMember(msg.GuildID, msg.UserID)
	if err != nil {
		return err
	}
	if user.User.ID == session.State.User.ID {
		return nil
	}

	// User joined
	if msg.BeforeUpdate == nil {
		numUsers[msg.ChannelID]++
		greetUser(session, msg, user)
		// User left
	} else if msg.VoiceState.ChannelID == "" {
		numUsers[msg.BeforeUpdate.ChannelID]--
		// Ensure cant go less than 0
		if numUsers[msg.BeforeUpdate.ChannelID] < 0 {
			numUsers[msg.BeforeUpdate.ChannelID] = 0
		}
	}

	return nil
}

func greetUser(session *discordgo.Session, msg *discordgo.VoiceStateUpdate, user *discordgo.Member) error {
	var name string
	if user.Nick == "" {
		name = user.User.Username
	} else {
		name = user.Nick
	}

	var phrase, text string
	if numUsers[msg.ChannelID] == 1 {
		phrase = config.GetPhrase("first_greet")
	} else if numUsers[msg.ChannelID] < 3 {
		phrase = config.GetPhrase("normal_greet")
	} else {
		phrase = config.GetPhrase("busy_greet")
	}

	if strings.Contains(phrase, "%s") {
		text = fmt.Sprintf(phrase, name)
	} else {
		text = phrase
	}

	conn, err := core.GetActiveConnection(session, msg.GuildID, msg.ChannelID)
	if err != nil {
		log.Println(err)
		return err
	}
	return conn.SendEvent(core.GreeterApp, text, msg.ChannelID)
}
