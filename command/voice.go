package command

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func HandleVoiceState(session *discordgo.Session, msg *discordgo.VoiceStateUpdate) error {
	if msg.BeforeUpdate == nil {
		user, err := session.GuildMember(msg.GuildID, msg.UserID)
		if err != nil {
			return err
		}
		if user.User.ID == session.State.User.ID {
			return nil
		}

		var name string
		if user.Nick == "" {
			name = user.User.Username
		} else {
			name = user.Nick
		}

		err = generateVoice(fmt.Sprintf("Hello %s", name))
		if err != nil {
			return err
		}

		// Join channel
		vc, err := session.ChannelVoiceJoin(msg.GuildID, msg.ChannelID, false, true)
		if err != nil {
			return err
		}
		defer vc.Disconnect()
		vc.Speaking(true)
		defer vc.Speaking(false)

		err = playMP3(vc)
		if err != nil {
			return err
		}
	}
	return nil
}
