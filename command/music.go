package command

import (
	"encoding/binary"
	"io"
	"log"
	"os"
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/hajimehoshi/go-mp3"
	"layeh.com/gopus"
)

var (
	FrameSize  int = 960
	Channels   int = 1
	SampleRate int = 48000
)

func HandleYT(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	return nil
}

func HandleTell(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	// Check the file opens first
	file, err := os.Open("botsounds/output.mp3")
	if err != nil {
		return err
	}
	defer file.Close()

	vc, err := JoinAuthorVoiceChannel(session, msg)
	if err != nil {
		return err
	}
	// Any error handling past here must close the voice channel connection
	vc.Speaking(true)
	err = PlayMP3(vc, file)
	if err != nil {
		vc.Disconnect()
		return err
	}
	vc.Speaking(false)

	vc.Disconnect()
	return nil
}

/*
	Find if a the message author is in a channel and join it
*/
func JoinAuthorVoiceChannel(session *discordgo.Session, msg *discordgo.MessageCreate) (*discordgo.VoiceConnection, error) {
	var vc *discordgo.VoiceConnection
	// Find sender's voice channel
	guild, err := session.State.Guild(msg.GuildID)
	if err != nil {
		return vc, err
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
		return vc, nil
	}

	// Join voice channel and start websocket audio communication
	vc, err = session.ChannelVoiceJoin(msg.GuildID, voicechannelID, false, true)
	if err != nil {
		return vc, err
	}
	return vc, nil
}

/*
	Play a reader of MP3 data over discord voice connection
*/
func PlayMP3(vc *discordgo.VoiceConnection, file io.Reader) error {
	var wg sync.WaitGroup
	var err error
	rawChan := make(chan []int16, 10)

	// Run reader and sender concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = MP3Reader(rawChan, file)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = OpusSender(rawChan, vc)
	}()

	wg.Wait()
	return err
}

/*
	Decode MP3 data to raw int16 and send in frames of FrameSize through channel
*/
func MP3Reader(c chan []int16, file io.Reader) error {
	// Ensure channel closed after all is read
	defer close(c)
	// Set up decoder
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		log.Println(err)
		return err
	}

	// Decode and break up into opus frames
	for {
		buffer := make([]int16, FrameSize*Channels)
		err := binary.Read(decoder, binary.LittleEndian, &buffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			c <- buffer
			return nil
		}
		if err != nil {
			log.Println(err)
			return err
		}
		c <- buffer
	}
}

/*
	Encodes raw audio data from a channel in opus and sends over discord
*/
func OpusSender(c chan []int16, vc *discordgo.VoiceConnection) error {
	// Set up encoder
	enc, err := gopus.NewEncoder(SampleRate, Channels, gopus.Audio)
	if err != nil {
		log.Println(err)
		return err
	}
	enc.SetBitrate(64 * 1000)

	for {
		frame, ok := <-c
		if !ok {
			// Reached end of channel
			return nil
		}
		// Encode frame and send through opus channel
		opus, err := enc.Encode(frame, FrameSize, FrameSize*Channels*2)
		if err != nil {
			log.Println(err)
			return err
		}
		vc.OpusSend <- opus
	}

}
