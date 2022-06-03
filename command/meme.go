package command

import "github.com/bwmarrin/discordgo"

func HandleSay(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	session.ChannelMessageSend(msg.ChannelID, "Hello!")
	return nil
}
