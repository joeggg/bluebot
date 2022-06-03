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
	// Opus encoding constants
	FrameSize  int = 960
	Channels   int = 1
	SampleRate int = 48000
	BitRate    int = 64 * 1000
)

/*
	Play the audio file generated by the Python backend
*/
func HandleTell(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	// Check the file opens first
	file, err := os.Open("botsounds/output.mp3")
	if err != nil {
		return err
	}
	defer file.Close()

	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if voiceChannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}
	// Join voice channel and start websocket audio communication
	vc, err := session.ChannelVoiceJoin(msg.GuildID, voiceChannelID, false, true)
	if err != nil {
		return err
	}
	// Any error handling past here must close the voice channel connection
	vc.Speaking(true)
	PlayMP3(vc, file)
	vc.Speaking(false)

	vc.Disconnect()
	return nil
}

/*
	Play a reader of MP3 data over discord voice connection
*/
func PlayMP3(vc *discordgo.VoiceConnection, file io.Reader) {
	var wg sync.WaitGroup
	rawChan := make(chan []int16, 10)

	// Run reader and sender concurrently
	wg.Add(1)
	go func() {
		defer wg.Done()
		MP3Reader(rawChan, file)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		OpusSender(rawChan, vc)
	}()

	wg.Wait()
}

/*
	Decode MP3 data to raw int16 and send in frames of FrameSize through channel
*/
func MP3Reader(c chan []int16, file io.Reader) {
	// Ensure channel closed after all is read
	defer close(c)
	// Set up decoder
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		log.Println(err)
		return
	}

	// Decode and break up into opus frames
	for {
		buffer := make([]int16, FrameSize*Channels)
		err := binary.Read(decoder, binary.LittleEndian, &buffer)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			c <- buffer
			return
		}
		if err != nil {
			log.Println(err)
			return
		}
		c <- buffer
	}
}

/*
	Encodes raw audio data from a channel in opus and sends over discord
*/
func OpusSender(c chan []int16, vc *discordgo.VoiceConnection) {
	// Set up encoder
	enc, err := gopus.NewEncoder(SampleRate, Channels, gopus.Audio)
	if err != nil {
		log.Println(err)
		return
	}
	enc.SetBitrate(BitRate)

	for {
		frame, ok := <-c
		if !ok {
			// Reached end of channel
			return
		}
		// Encode frame and send through opus channel
		opus, err := enc.Encode(frame, FrameSize, FrameSize*Channels*2)
		if err != nil {
			log.Println(err)
			return
		}
		vc.OpusSend <- opus
	}

}
