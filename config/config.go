package config

import (
	"io/ioutil"
	"log"
)

var GoogleKeyPath string = "/etc/bluebot/google_token.json"

// Read the token as a string from file
func ReadDiscordToken() (string, error) {
	token, err := ioutil.ReadFile("/etc/bluebot/token.txt")
	if err != nil {
		log.Printf("Failed to open Discord token file: %s", err)
		return "", err
	}
	return string(token), err
}
