package config

import (
	"io/ioutil"
	"log"
)

var (
	GoogleKeyPath    string = "/etc/bluebot/google_token.json"
	AudioPath        string = "/var/lib/bluebot"
	DiscordTokenPath string = "/etc/bluebot/token.txt"
	CivListPath      string = "./data/civ_list.csv"
)

// Read the token as a string from file
func ReadDiscordToken() (string, error) {
	token, err := ioutil.ReadFile(DiscordTokenPath)
	if err != nil {
		log.Printf("Failed to open Discord token file: %s", err)
		return "", err
	}
	return string(token), err
}
