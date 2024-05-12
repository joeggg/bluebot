package config

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"math/big"
	"os"
	"strings"

	porcupine "github.com/Picovoice/porcupine/binding/go/v2"
	"gopkg.in/yaml.v3"
)

var Cfg = &Config{}

var DiscordToken string

var PorcupineToken string

var Phrases = make(map[string][]string, 0)

var VoicePresets = make(map[string]*VoicePreset, 0)

var ImageSettings = make(map[string]*ImageSetting, 0)

type Config struct {
	AudioPath          string `yaml:"AudioPath"`
	CivListPath        string `yaml:"CivListPath"`
	CivSelections      int    `yaml:"CivSelections"`
	GoogleKeyPath      string `yaml:"GoogleKeyPath"`
	DiscordTokenPath   string `yaml:"DiscordTokenPath"`
	PorcupineTokenPath string `yaml:"PorcupineTokenPath"`
	ImageFontPath      string `yaml:"ImageFontPath"`
	ImagePath          string `yaml:"ImagePath"`
	SelfImagePath      string `yaml:"SelfImagePath"`
	ImageSettingsPath  string `yaml:"ImageSettingsPath"`
	LogFilePath        string `yaml:"LogFilePath"`
	SettingsDurationS  int    `yaml:"SettingsDurationS"`
	VoicePresetsPath   string `yaml:"VoicePresetsPath"`
}

type ImageSetting struct {
	Filename string `json:"filename"`
	TextX    int    `json:"text_x"`
	TextY    int    `json:"text_y"`
}

type VoicePreset struct {
	Name     string                   `json:"name"`
	Language string                   `json:"language"`
	Pitch    float64                  `json:"pitch"`
	Rate     float64                  `json:"rate"`
	Gender   string                   `json:"gender"`
	WakeWord porcupine.BuiltInKeyword `json:"wake_word"`
}

func LoadConfig() error {
	file, err := os.OpenFile(os.Getenv("CONFIG"), os.O_RDONLY, 0444)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)

	if err = decoder.Decode(Cfg); err != nil {
		return err
	} else if DiscordToken, err = ReadStringFromFile(Cfg.DiscordTokenPath); err != nil {
		return err
	} else if PorcupineToken, err = ReadStringFromFile(Cfg.PorcupineTokenPath); err != nil {
		return err
	} else if err = loadVoicePresets(); err != nil {
		return err
	} else if err = loadImageSettings(); err != nil {
		return err
	} else if err = loadPhrases(); err != nil {
		return err
	} else {
		return nil
	}
}

func loadVoicePresets() error {
	data, err := os.ReadFile(Cfg.VoicePresetsPath)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &VoicePresets)
	if err != nil {
		return err
	}
	return nil
}

func loadImageSettings() error {
	data, err := os.ReadFile(Cfg.ImageSettingsPath)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, &ImageSettings)
	if err != nil {
		return err
	}
	return nil
}

func loadPhrases() error {
	var err error
	categories := []string{
		"say", "wrongcommand", "taxes", "first_greet", "normal_greet", "busy_greet",
	}
	for _, category := range categories {
		Phrases[category], err = loadPhraseList(category)
		if err != nil {
			return err
		}
	}
	return nil
}

func loadPhraseList(filename string) ([]string, error) {
	var phrases map[string][]string
	file, err := os.ReadFile(Cfg.AudioPath + "/../phrases/" + filename + ".json")
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(file, &phrases)
	if err != nil {
		return nil, err
	}
	return phrases["data"], nil
}

// Get a random phrase from a chosen phrase list category
func GetPhrase(category string) string {
	list, ok := Phrases[category]
	if !ok {
		return "Hello!"
	}
	max := big.NewInt(int64(len(list)))
	sel, _ := rand.Int(rand.Reader, max)
	return list[sel.Int64()]
}

// Read the token as a string from file
func ReadStringFromFile(filename string) (string, error) {
	token, err := os.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(token)), err
}

func SetupLogging() error {
	file, err := os.OpenFile(Cfg.LogFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	log.SetOutput(file)
	return nil
}
