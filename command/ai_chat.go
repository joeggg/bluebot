package command

import (
	"bluebot/config"
	"context"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	leopard "github.com/Picovoice/leopard/binding/go"
	porcupine "github.com/Picovoice/porcupine/binding/go/v2"
	"github.com/bwmarrin/discordgo"
	"layeh.com/gopus"
)

func HandleSayChat(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	response, err := getAiText(args, msg.ChannelID, VoiceSelection)
	if err != nil {
		return err
	}
	return HandleTell(session, msg, []string{response})
}

func HandleChat(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	response, err := getAiText(args, msg.ChannelID, VoiceSelection)
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

func getAiText(args []string, uid string, personality string) (string, error) {
	sck, err := NewWorkerSocket()
	if err != nil {
		return "", err
	}

	if len(args) < 1 {
		return "", fmt.Errorf("need a message to send")
	}

	to_send := map[string]interface{}{
		"uid":         uid,
		"message":     strings.Join(args, " "),
		"personality": personality,
	}
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

func HandleListen(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	voiceChannelID := getAuthorVoiceChannel(session, msg)
	if voiceChannelID == "" {
		session.ChannelMessageSend(msg.ChannelID, "You're not in a voice channel")
		return nil
	}
	// Join voice channel and start websocket audio communication
	vc, err := session.ChannelVoiceJoin(msg.GuildID, voiceChannelID, false, false)
	if err != nil {
		return err
	}
	defer vc.Disconnect()

	return listenForWake(vc)
}

func listenForWake(vc *discordgo.VoiceConnection) error {
	p := porcupine.Porcupine{
		AccessKey: config.PorcupineToken,
		BuiltInKeywords: []porcupine.BuiltInKeyword{
			porcupine.PICOVOICE,
			porcupine.COMPUTER,
			porcupine.ALEXA,
		},
	}
	err := p.Init()
	if err != nil {
		return err
	}
	defer p.Delete()
	// Send empty packet to start receiving
	// https://github.com/bwmarrin/discordgo/issues/598#issuecomment-466658213
	decoder, err := gopus.NewDecoder(SampleRate, Channels)
	if err != nil {
		return err
	}

	pcm_chan := make(chan []int16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case packet := <-vc.OpusRecv:
				pcm, err := decoder.Decode(packet.Opus, FrameSize, false)
				if err != nil {
					log.Println(err)
					continue
				}
				// Downsample to 16K
				var data []int16
				for i, sample := range pcm {
					if i%3 == 0 {
						data = append(data, sample)
					}
				}
				pcm_chan <- data
			case <-ctx.Done():
				return
			}
		}
	}()

	buffer := make([]int16, 0)
	input_len := 320
	output_len := 512
	for {
		select {
		case pcm := <-pcm_chan:
			// Convert to 512 length buffer
			amount_needed := output_len - len(buffer)
			// Use at a maximum the length of the input data
			idx_unil := int16(math.Min(float64(amount_needed), float64(input_len)))
			buffer = append(buffer, pcm[:idx_unil]...)

			// Not filling the buffer means we must have used all the data
			if len(buffer) != output_len {
				continue
			}

			keywordIndex, err := p.Process(buffer)
			// Put remaining data into next buffer
			buffer = pcm[amount_needed:]

			if err != nil {
				log.Println(err)
				continue
			}
			var personality string
			if keywordIndex == 0 {
				personality = "bluebot"
			} else if keywordIndex == 1 {
				personality = "best"
			} else if keywordIndex == 2 {
				personality = "mandarin"
			} else {
				continue
			}

			if err = playText(vc, "hey", personality); err != nil {
				log.Println(err)
				continue
			}
			transcript, err := listenForAudio(pcm_chan)
			if err != nil {
				log.Println(err)
				continue
			}
			response, err := getAiText([]string{transcript}, vc.ChannelID, personality)
			if err != nil {
				log.Println(err)
				continue
			}
			go playText(vc, response, personality)

		case <-time.After(time.Minute):
			log.Println("Exit listen due to inactivity")
			return nil
		}

	}
}

func listenForAudio(pcm_chan chan []int16) (string, error) {
	l := leopard.NewLeopard(config.PorcupineToken)
	err := l.Init()
	if err != nil {
		return "", err
	}
	defer l.Delete()
	var data []int16

	for {
		select {
		case pcm := <-pcm_chan:
			data = append(data, pcm...)

		case <-time.After(time.Second):
			transcript, _, err := l.Process(data)
			if err != nil {
				return "", err
			}

			log.Printf(transcript)
			return transcript, nil
		}
	}
}
