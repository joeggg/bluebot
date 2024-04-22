package jytdl

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const chunk = 10_000_000
const attempts = 10

func GetAudio(videoID string, filename string, targetFormat string) error {
	// Try a few times as sometimes fails
	var err error
	for i := 0; i < attempts; i++ {
		if err = getAudio(videoID, filename, targetFormat); err == nil {
			return nil
		}
	}
	return err
}

func getAudio(videoID string, filename string, targetFormat string) error {
	// Get innertube formats
	formats, err := GetFormats(videoID)
	if err != nil {
		return err
	}
	// Find correct format
	format := extractFormat(formats, targetFormat)
	if format == nil {
		return errors.New("no format could be found")
	}
	downloadAudio(format, filename)
	return nil
}

func extractFormat(formats *[]Format, target string) *Format {
	for _, format := range *formats {
		if strings.Contains(format.MIMEType, target) &&
			format.AudioQuality == "AUDIO_QUALITY_MEDIUM" {
			return &format
		}
	}
	return nil
}

func downloadAudio(format *Format, filename string) error {
	// Create empty file with correct extension
	if !strings.Contains(filename, ".") {
		filename += format.Extension()
	}
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	req, err := http.NewRequest("GET", format.URL, nil)
	if err != nil {
		return err
	}
	// Iterate over chunks of audio split across requests
	var pos int64
	for pos < format.ContentLength {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", pos, pos+chunk-1))

		resp, err := client.Do(req)
		if err != nil {
			return err
		}

		if _, err := io.Copy(file, resp.Body); err != nil {
			return err
		}
		if err := resp.Body.Close(); err != nil {
			return err
		}

		pos += chunk
	}
	return nil
}
