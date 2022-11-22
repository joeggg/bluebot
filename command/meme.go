package command

import (
	"bluebot/config"

	"github.com/bwmarrin/discordgo"
)

func HandleSay(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	session.ChannelMessageSend(msg.ChannelID, config.GetPhrase("say"))
	return nil
}

func HandleTaxes(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	session.ChannelMessageSend(msg.ChannelID, config.GetPhrase("taxes"))
	return nil
}
