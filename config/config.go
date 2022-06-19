package config

import (
	"crypto/rand"
	"encoding/json"
	"io/ioutil"
	"log"
	"math/big"
	"os"

	"gopkg.in/yaml.v3"
)

var Cfg = Config{}
var Phrases = map[string][]string{}

type Config struct {
	AudioPath         string `yaml:"AudioPath"`
	LogFilePath       string `yaml:"LogFilePath"`
	CivListPath       string `yaml:"CivListPath"`
	CivSelections     int    `yaml:"CivSelections"`
	GoogleKeyPath     string `yaml:"GoogleKeyPath"`
	DiscordTokenPath  string `yaml:"DiscordTokenPath"`
	SettingsDurationS int    `yaml:"SettingsDurationS"`
}

func LoadConfig() error {
	file, err := os.OpenFile(os.Getenv("CONFIG"), os.O_RDONLY, 0444)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	err = decoder.Decode(&Cfg)
	if err != nil {
		return err
	}
	err = loadPhrases()
	if err != nil {
		return err
	}
	return nil
}

func loadPhrases() error {
	var err error
	Phrases["say"], err = loadPhraseList("say")
	if err != nil {
		return err
	}
	Phrases["wrongcommand"], err = loadPhraseList("wrongcommand")
	if err != nil {
		return err
	}
	return nil
}

func loadPhraseList(filename string) ([]string, error) {
	var phrases map[string][]string
	file, err := ioutil.ReadFile(Cfg.AudioPath + "/../phrases/" + filename + ".json")
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
func ReadDiscordToken() (string, error) {
	token, err := ioutil.ReadFile(Cfg.DiscordTokenPath)
	if err != nil {
		return "", err
	}
	return string(token), err
}

func SetupLogging() error {
	file, err := os.OpenFile(Cfg.LogFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	log.SetOutput(file)
	return nil
}
