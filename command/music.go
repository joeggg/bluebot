package command

import (
	"bluebot/command/core"
	"bluebot/util"

	"github.com/bwmarrin/discordgo"
)

var MusicCommands = map[string]util.HandlerFunc{
	"queue":  handleQueue,
	"list":   handleList,
	"next":   handleNext,
	"pause":  handlePause,
	"resume": handleResume,
	"stop":   handleStop,
}

/*
Begin the download and playback of audio from a YT video or playlist link or add to the queue
of an existing subscription
*/
func handleQueue(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if len(args) < 1 {
		session.ChannelMessageSend(msg.ChannelID, "No URL or search text given")
		return nil
	}
	return handleEvent("queue", session, msg)
}

func handleList(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	return handleEvent("list", session, msg)
}

func handleNext(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	return handleEvent("next", session, msg)
}

func handlePause(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	return handleEvent("pause", session, msg)
}

func handleResume(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	return handleEvent("resume", session, msg)
}

func handleStop(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	return handleEvent("stop", session, msg)
}

func handleEvent(event string, session *discordgo.Session, msg *discordgo.MessageCreate) error {
	conn, err := core.GetActiveConnection(session, msg.GuildID, "", msg.Author.ID)
	if err != nil {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}

	return conn.SendEvent("sub", event, msg.ChannelID)
}
