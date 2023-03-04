package command

import (
	"bluebot/config"
	"fmt"

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

func HandleMemeOfTheDay(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	sck, err := NewWorkerSocket()
	if err != nil {
		return err
	}
	sck.Send("memeoftheday", nil)
	resp, err := sck.Receive()
	if err != nil {
		return err
	}

	data, ok := resp.(map[string]interface{})
	if !ok {
		return fmt.Errorf("failed to decode response: %s", resp)
	}
	session.ChannelMessageSend(
		msg.ChannelID,
		fmt.Sprintf(
			"**%s**\n*by %s in %s*\n%s",
			data["title"],
			data["author"],
			data["subreddit"],
			data["url"],
		),
	)

	return nil
}
