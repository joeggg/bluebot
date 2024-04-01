package command

import (
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func HandleSayChat(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	response, err := getAiText(args)
	if err != nil {
		return err
	}
	return HandleTell(session, msg, []string{response})
}

func HandleChat(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	response, err := getAiText(args)
	if err != nil {
		return err
	}
	session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf("%s", response))
	return nil
}

func HandleResetChat(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	sck, err := NewWorkerSocket()
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

func getAiText(args []string) (string, error) {
	sck, err := NewWorkerSocket()
	if err != nil {
		return "", err
	}

	if len(args) < 1 {
		return "", fmt.Errorf("need a message to send")
	}

	to_send := map[string]interface{}{"message": strings.Join(args, " ")}
	sck.Send("do_ai", &to_send)
	resp, err := sck.Receive()
	if err != nil {
		return "", err
	}

	data, ok := resp.(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("failed to decode response: %s", resp)
	}
	return data["response"].(string), nil
}
