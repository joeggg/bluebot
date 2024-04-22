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
	playerParams    = "CgIQBg=="
	version         = "18.11.34"
	android_version = 30
	origin          = "https://www.youtube.com"
	api_key         = "AIzaSyA8eiZmM1FaDVjRy-df2KTyQ_vz_yYM39w"
	userAgent       = "com.google.android.youtube/18.11.34 (Linux; U; Android 11) gzip"
)

var client = http.DefaultClient

type FormatRequest struct {
	ContentCheckOK bool `json:"contentCheckOk,omitempty"`
	Context        struct {
		Client struct {
			Name              string `json:"clientName"`
			Version           string `json:"clientVersion"`
			HL                string `json:"hl"`
			GL                string `json:"gl"`
			AndroidSDKVersion int    `json:"androidSDKVersion,omitempty"`
			UserAgent         string `json:"userAgent,omitempty"`
			TimeZone          string `json:"timeZone"`
			UTCOffset         int    `json:"utcOffsetMinutes"`
		} `json:"client"`
	} `json:"context"`
	PlaybackContext struct {
		ContentPlaybackContext struct {
			HTML5Preference string `json:"html5Preference"`
		}
	}
	Params      string `json:"params,omitempty"`
	Query       string `json:"query,omitempty"`
	RacyCheckOK bool   `json:"racyCheckOk,omitempty"`
	VideoID     string `json:"videoId,omitempty"`
}

type FormatResponse struct {
	Microformat struct {
		PlayerMicroformatRenderer struct {
			Publish_Date string `json:"publishDate"`
		} `json:"playerMicroformatRenderer"`
	}
	PlayabilityStatus Status `json:"playabilityStatus"`
	StreamingData     struct {
		AdaptiveFormats []Format `json:"adaptiveFormats"`
	} `json:"streamingData"`
	VideoDetails struct {
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

	req.Context.Client.Name = "ANDROID"
	req.Context.Client.Version = version
	req.Context.Client.HL = "en"
	req.Context.Client.GL = "US"
	req.Context.Client.TimeZone = "UTC"
	req.Context.Client.AndroidSDKVersion = android_version
	req.Context.Client.UserAgent = userAgent

	req.ContentCheckOK = true
	req.RacyCheckOK = true
	req.VideoID = videoID
	req.Params = playerParams
	req.PlaybackContext.ContentPlaybackContext.HTML5Preference = "HTML5_PREF_WANTS"
	return &req
}

func GetFormats(videoID string) (*[]Format, error) {
	data := NewFormatRequest(videoID)
	body, err := json.MarshalIndent(data, "", " ")
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", origin+"/youtubei/v1/player?key="+api_key, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Youtube-Client-Name", "3")
	req.Header.Set("X-Youtube-Client-Version", version)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Origin", "https://youtube.com")
	req.Header.Set("Sec-Fetch-Mode", "navigate")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var formats FormatResponse
	if err := json.NewDecoder(resp.Body).Decode(&formats); err != nil {
		return nil, err
	}
	return &formats.StreamingData.AdaptiveFormats, nil
}
