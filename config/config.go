package config

import (
	"io/ioutil"
	"log"
)

// Read the token as a string from file
func ReadToken() (string, error) {
	token, err := ioutil.ReadFile("/etc/bluebot/token.txt")
	if err != nil {
		log.Println("Failed to open token file")
		return "", err
	}
	return string(token), err
}
