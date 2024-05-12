package core

import (
	"bluebot/config"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"time"

	"cloud.google.com/go/speech/apiv2"
	"cloud.google.com/go/speech/apiv2/speechpb"
	porcupine "github.com/Picovoice/porcupine/binding/go/v2"
	filters "github.com/mattetti/audio/dsp/filters"
	"github.com/mattetti/audio/dsp/windows"
	"google.golang.org/api/option"
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
	log.Printf("Took %f seconds to get ai text", time.Since(start).Seconds())
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
	// Speech to text
	sptService, err := speech.NewClient(
		conv.container.ctx,
		option.WithCredentialsFile(config.Cfg.GoogleKeyPath),
	)
	defer sptService.Close()
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
			transcript, err := listenForAudioGoogle(sptService, audioIn, conv.container)
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

func listenForAudioGoogle(
	service *speech.Client, pcm_chan chan []int16, container *Container,
) (string, error) {
	stream, err := service.StreamingRecognize(container.ctx)
	if err != nil {
		return "", err
	}
	if err = stream.Send(&speechpb.StreamingRecognizeRequest{
		Recognizer: "projects/primal-turbine-324300/locations/global/recognizers/bluebotg",
		StreamingRequest: &speechpb.StreamingRecognizeRequest_StreamingConfig{
			StreamingConfig: &speechpb.StreamingRecognitionConfig{
				Config: &speechpb.RecognitionConfig{
					DecodingConfig: &speechpb.RecognitionConfig_ExplicitDecodingConfig{
						ExplicitDecodingConfig: &speechpb.ExplicitDecodingConfig{
							Encoding:          speechpb.ExplicitDecodingConfig_LINEAR16,
							SampleRateHertz:   16000,
							AudioChannelCount: int32(Channels),
						},
					},
				},
			},
		},
	}); err != nil {
		return "", err
	}

	go func() {
		for {
			select {
			case <-container.ctx.Done():
				return

			case pcm := <-pcm_chan:
				buf := bytes.Buffer{}
				for _, sample := range pcm {
					binary.Write(&buf, binary.LittleEndian, sample)
				}

				if err = stream.Send(
					&speechpb.StreamingRecognizeRequest{
						Recognizer:       "projects/primal-turbine-324300/locations/global/recognizers/bluebotg",
						StreamingRequest: &speechpb.StreamingRecognizeRequest_Audio{Audio: buf.Bytes()},
					},
				); err != nil {
					log.Printf("Error making audio request: %s", err)
					continue
				}

			case <-time.After(cutoffDelay):
				if err = stream.CloseSend(); err != nil {
					log.Printf("Error closing stream: %s", err)
				}
				return
			}
		}
	}()

	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			return "", nil
		}
		if err != nil {
			log.Printf("Error receiving from stream: %s", err)
			return "", err
		}

		if len(resp.Results) < 1 || len(resp.Results[0].Alternatives) < 1 {
			return "", nil
		}

		result := resp.Results[0].Alternatives[0].Transcript
		log.Println(result)
		return result, nil
	}
}
