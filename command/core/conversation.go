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
	filters "github.com/mattetti/audio/dsp/filters"
	"github.com/mattetti/audio/dsp/windows"
	"layeh.com/gopus"
)

func GetAiText(sck *WorkerSocket, text string, uid string, personality string) (string, error) {
	start := time.Now()
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
	log.Printf("Took %d seconds to get ai text", time.Since(start))
	return data["response"].(string), nil
}

const (
	downsampledFrameSize = 320
	cutoffDelay          = 500 * time.Millisecond
)

type Conversation struct {
	container *Container
	running   bool
}

func NewConversation(container *Container) VoiceApp {
	return &Conversation{container: container}
}

func (conv *Conversation) Container() *Container {
	return conv.container
}

func (conv *Conversation) SendEvent(event string, args []string, channelID string) error {
	// Slighty dodgy as assumes the app won't have started yet if this is run on startup
	if conv.running {
		conv.container.cancel()
	}
	return nil
}

func (conv *Conversation) Run(channelID string) error {
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
	audioIn := make(chan []int16)
	go readFrames(decoder, audioIn, conv.container)

	buffer := make([]int16, 0)
	outputLen := 512
	conv.running = true
	for {
		select {
		case <-conv.container.ctx.Done():
			return nil

		case pcm := <-audioIn:
			// Convert to 512 length buffer
			amount_needed := outputLen - len(buffer)
			// Use at a maximum the length of the input data
			idx_unil := int16(math.Min(float64(amount_needed), float64(downsampledFrameSize)))
			buffer = append(buffer, pcm[:idx_unil]...)

			// Not filling the buffer means we must have used all the data
			if len(buffer) != outputLen {
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

			if err = PlayText("hey", personality, conv.container); err != nil {
				log.Println(err)
				continue
			}
			transcript, err := listenForAudio(&l, audioIn, conv.container)
			if err != nil {
				log.Println(err)
				continue
			}
			response, err := GetAiText(sck, transcript, conv.container.vc.ChannelID, personality)
			if err != nil {
				log.Println(err)
				continue
			}
			go PlayText(response, personality, conv.container)

		case <-time.After(time.Minute):
			log.Println("Exit listen due to inactivity")
			return nil
		}

	}
}

func readFrames(decoder *gopus.Decoder, audioIn chan []int16, container *Container) {
	filter := filters.FIR{Sinc: &filters.Sinc{
		CutOffFreq: 8000, SamplingFreq: SampleRate, Taps: 20, Window: windows.Blackman,
	}}

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

			floatData := make([]float64, FrameSize)
			for i, sample := range pcm {
				floatData[i] = float64(sample)
			}

			// Anti aliasing filter
			filteredData, err := filter.LowPass(floatData)
			if err != nil {
				log.Println(err)
				continue
			}
			// Downsample to 16K
			data := make([]int16, downsampledFrameSize)
			count := 0
			for i, sample := range filteredData {
				if i%3 == 0 {
					data[count] = int16(sample)
					count++
				}
			}
			audioIn <- data
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

		case <-time.After(cutoffDelay):
			transcript, _, err := l.Process(data)
			if err != nil {
				return "", err
			}

			log.Printf(transcript)
			return transcript, nil
		}
	}
}
