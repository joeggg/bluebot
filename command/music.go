package command

import (
	"encoding/binary"
	"io"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/hajimehoshi/go-mp3"
	"layeh.com/gopus"
)

var (
	FrameSize  int = 960
	Channels   int = 1
	SampleRate int = 48000
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

	vc.Speaking(true)
	err = LoadAndPlay(vc, "botsounds/output.mp3")
	if err != nil {
		vc.Disconnect()
		return err
	}
	vc.Speaking(false)

	vc.Disconnect()
	return nil
}

func LoadAndPlay(vc *discordgo.VoiceConnection, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read mp3 data
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		return err
	}

	var inputAudio [][]int16
	// Break up into opus frames
	for {
		buffer := make([]int16, FrameSize*Channels)
		err := binary.Read(decoder, binary.LittleEndian, &buffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return err
		}
		inputAudio = append(inputAudio, buffer)
	}

	// Encode with opus and send
	enc, err := gopus.NewEncoder(SampleRate, Channels, gopus.Audio)
	if err != nil {
		return err
	}
	enc.SetBitrate(64 * 1000)
	for i := 0; i < len(inputAudio); i++ {
		opus, err := enc.Encode(inputAudio[i], FrameSize, FrameSize*Channels*2)
		if err != nil {
			return err
		}
		vc.OpusSend <- opus
	}

	return nil
}
