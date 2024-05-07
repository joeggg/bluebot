package core

import (
	"bluebot/config"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	leopard "github.com/Picovoice/leopard/binding/go"
	porcupine "github.com/Picovoice/porcupine/binding/go/v2"
	"layeh.com/gopus"
)

func GetAiText(sck *WorkerSocket, text string, uid string, personality string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("need a message to send")
	}

	to_send := map[string]interface{}{
		"uid":         uid,
		"message":     text,
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

type Conversation struct {
	running bool
	pause   chan bool
}

func NewConversation() *Conversation {
	return &Conversation{}
}

func (conv *Conversation) IsRunning() bool {
	return conv.running
}

func (conv *Conversation) SetRunning(running bool) {
	conv.running = running
}

func (conv *Conversation) SendEvent(event string, args []string, channelID string) error {
	return nil
}

func (conv *Conversation) Run(container *Container, channelID string) error {
	// Wake word detector
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
	// Speech to text synthesiser
	l := leopard.NewLeopard(config.PorcupineToken)
	err = l.Init()
	if err != nil {
		return err
	}
	defer l.Delete()
	// Backend communication (for OpenAI)
	sck, err := NewWorkerSocket()
	if err != nil {
		return err
	}

	decoder, err := gopus.NewDecoder(SampleRate, Channels)
	if err != nil {
		return err
	}

	pcm_chan := make(chan []int16)

	go func() {
		for {
			select {
			case <-container.ctx.Done():
				return

			case packet := <-container.vc.OpusRecv:
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

			}
		}
	}()

	buffer := make([]int16, 0)
	input_len := 320
	output_len := 512
	for {
		select {
		case <-container.ctx.Done():
			return nil

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

			if err = PlayText("hey", personality, container); err != nil {
				log.Println(err)
				continue
			}
			transcript, err := listenForAudio(&l, pcm_chan, container)
			if err != nil {
				log.Println(err)
				continue
			}
			response, err := GetAiText(sck, transcript, container.vc.ChannelID, personality)
			if err != nil {
				log.Println(err)
				continue
			}
			go PlayText(response, personality, container)

		case <-time.After(time.Minute):
			log.Println("Exit listen due to inactivity")
			return nil
		}

	}
}

func listenForAudio(l *leopard.Leopard, pcm_chan chan []int16, container *Container) (string, error) {
	var data []int16
	for {
		select {
		case <-container.ctx.Done():
			return "", errors.New("Received cancel signal")

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
