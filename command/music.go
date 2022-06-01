package command

import (
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/hajimehoshi/go-mp3"
)

func HandleTell(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	// Find sender's voice channel
	guild, err := session.State.Guild(msg.GuildID)
	if err != nil {
		return err
	}
	voicechannelID := ""
	for _, vs := range guild.VoiceStates {
		if vs.UserID == msg.Author.ID {
			voicechannelID = vs.ChannelID
			break
		}
	}
	if voicechannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}

	// Join voice channel and start websocket audio communication
	vc, err := session.ChannelVoiceJoin(msg.GuildID, voicechannelID, false, true)
	if err != nil {
		return err
	}
	// Any error handling past here must close the voice channel connection

	buffer, err := LoadClip("botsounds/output.mp3")
	if err != nil {
		vc.Disconnect()
		return err
	}
	log.Println(buffer)

	vc.Speaking(true)
	// Playing sound here
	vc.Speaking(false)

	vc.Disconnect()
	return nil
}

func LoadClip(filename string) ([]byte, error) {
	var buffer []byte
	file, err := os.Open(filename)
	if err != nil {
		return buffer, err
	}
	defer file.Close()

	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return buffer, err
	}
	decoder.Read(buffer)

	return buffer, nil
}
