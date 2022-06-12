package config

import (
	"io/ioutil"
	"log"
	"os"
)

var (
	GoogleKeyPath    string = "/etc/bluebot/google_token.json"
	AudioPath        string = "/var/lib/bluebot/tmp"
	DiscordTokenPath string = "/etc/bluebot/token.txt"
	CivListPath      string = "/var/lib/bluebot/civ_list.csv"
	LogfilePath      string = "/var/log/bluebot/logfile.log"
)

// Read the token as a string from file
func ReadDiscordToken() (string, error) {
	token, err := ioutil.ReadFile(DiscordTokenPath)
	if err != nil {
		return "", err
	}
	return string(token), err
}

func SetupLogging() error {
	file, err := os.OpenFile(LogfilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	log.SetOutput(file)
	return nil
}
