package jytdl

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/http"
)

const (
	// com.google.android.youtube
	// Public youtube player values
	android_version = "18.04.35"
	mweb_version    = "2.20230106.01.00"
	origin          = "https://www.youtube.com"
	api_key         = "AIzaSyAO_FJ2SlqU8Q4STEHLGCilw_Y9_11qcW8"
	client_ID       = "861556708454-d6dlm3lh05idd8npek18k6be8ba3oc68.apps.googleusercontent.com"
	client_secret   = "SboVhoG9s0rNafixCSGGKXAT"
)

var client = http.DefaultClient

type Token struct {
	AccessToken  string
	Error        string
	RefreshToken string
}

type FormatRequest struct {
	ContentCheckOK bool `json:"contentCheckOk,omitempty"`
	Context        struct {
		Client struct {
			Name    string `json:"clientName"`
			Version string `json:"clientVersion"`
		} `json:"client"`
	} `json:"context"`
	Params      []byte `json:"params,omitempty"`
	Query       string `json:"query,omitempty"`
	RacyCheckOK bool   `json:"racyCheckOk,omitempty"`
	VideoID     string `json:"videoId,omitempty"`
}

type FormatResponse struct {
	Microformat struct {
		Player_Microformat_Renderer struct {
			Publish_Date string `json:"publishDate"`
		} `json:"playerMicroformatRenderer"`
	}
	Playability_Status Status `json:"playabilityStatus"`
	Streaming_Data     struct {
		Adaptive_Formats []Format `json:"adaptiveFormats"`
	} `json:"streamingData"`
	Video_Details struct {
		Author            string
		Length_Seconds    int64  `json:"lengthSeconds,string"`
		Short_Description string `json:"shortDescription"`
		Title             string
		Video_ID          string `json:"videoId"`
		View_Count        int64  `json:"viewCount,string"`
	} `json:"videoDetails"`
}

type Format struct {
	AudioQuality  string `json:"audioQuality"`
	Bitrate       int64
	ContentLength int64 `json:"contentLength,string"`
	Height        int
	MIMEType      string `json:"mimeType"`
	QualityLabel  string `json:"qualityLabel"`
	URL           string
	Width         int
}

type Status struct {
	Status string
	Reason string
}

func (f Format) Extension() string {
	mediaType, _, err := mime.ParseMediaType(f.MIMEType)
	if err != nil {
		return ""
	}
	switch mediaType {
	case "audio/mp4":
		return ".m4a"
	case "audio/webm":
		return ".weba"
	case "video/mp4":
		return ".m4v"
	case "video/webm":
		return ".webm"
	}
	return ""
}

func NewFormatRequest(videoID string) *FormatRequest {
	var req FormatRequest
	req.ContentCheckOK = true
	req.Context.Client.Name = "ANDROID"
	req.Context.Client.Version = android_version
	req.RacyCheckOK = true
	req.VideoID = videoID
	return &req
}

func GetFormats(videoID string, token *Token) (*[]Format, error) {
	data := NewFormatRequest(videoID)
	body, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", origin+"/youtubei/v1/player", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Goog-API-Key", api_key)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var formats FormatResponse
	if err := json.NewDecoder(resp.Body).Decode(&formats); err != nil {
		return nil, err
	}
	return &formats.Streaming_Data.Adaptive_Formats, nil
}
