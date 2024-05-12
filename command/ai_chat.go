package command

import (
	"bluebot/command/core"
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func HandleSayChat(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	sck, err := core.NewWorkerSocket()
	if err != nil {
		return err
	}
	response, err := core.GetAiText(sck, strings.Join(args, " "), msg.ChannelID, VoiceSelection)
	if err != nil {
		return err
	}
	return HandleTell(session, msg, []string{response})
}

func HandleChat(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	sck, err := core.NewWorkerSocket()
	if err != nil {
		return err
	}
	response, err := core.GetAiText(sck, strings.Join(args, " "), msg.ChannelID, VoiceSelection)
	if err != nil {
		return err
	}
	session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf("%s", response))
	return nil
}

func HandleResetChat(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	sck, err := core.NewWorkerSocket()
	if err != nil {
		return err
	}

	to_send := map[string]interface{}{}
	sck.Send("reset_ai", &to_send)
	resp, err := sck.Receive()
	if err != nil {
		return err
	}

	_, ok := resp.(map[string]interface{})
	if !ok {
		return fmt.Errorf("failed to decode response: %s", resp)
	}
	session.ChannelMessageSend(msg.ChannelID, "oh no you killed me")

	return nil
}

func HandleListen(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	conn, err := core.GetActiveConnectionByAuthor(session, msg.GuildID, msg.Author.ID)
	if err != nil {
		log.Println(err)
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}
	conn.SendEvent(core.AiApp, "start", msg.ChannelID)
	return nil
}
