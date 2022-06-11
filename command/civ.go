package command

import (
	"bluebot/config"
	"encoding/csv"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

var (
	DefaultMaxTier int = 1 // NOTE: tiers are inverse to expected
	DefaultMinTier int = 8
	Settings           = map[string]*Setting{} // TODO: TTL thread-safe cache?
)

type Setting struct {
	MaxTier int
	MinTier int
	Players []string
}

func HandleCiv(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if len(args) > 0 && args[0] == "tiers" {
		return setTiers(session, msg, args)
	} else {
		return generateCivs(session, msg, args)
	}
}

/*
	Generate a random selection of civs for provided or previously saved players within saved
	or default min/max tiers
*/
func generateCivs(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	// Check if settings exists and create new if not
	if _, ok := Settings[msg.ChannelID]; !ok {
		Settings[msg.ChannelID] = &Setting{DefaultMaxTier, DefaultMinTier, args}
	} else if len(args) != 0 {
		Settings[msg.ChannelID].Players = args
	}
	if len(Settings[msg.ChannelID].Players) == 0 {
		session.ChannelMessageSend(msg.ChannelID, "Please provide some players")
		return nil
	}

	civs, err := readCivList()
	if err != nil {
		return err
	}

	// Generate random civs
	source := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(source)
	output := ""
	for _, player := range Settings[msg.ChannelID].Players {
		tier := -1
		civ := ""
		for tier < Settings[msg.ChannelID].MaxTier || tier > Settings[msg.ChannelID].MinTier {
			idx := r1.Intn(len(civs))
			tier, err = strconv.Atoi(civs[idx][2])
			if err != nil {
				return err
			}
			civ = civs[idx][1]
			civs = append(civs[:idx], civs[idx+1:]...)
			if len(civs) == 0 {
				session.ChannelMessageSend(msg.ChannelID, "Not enough civs for the criteria given")
				return nil
			}
		}
		output += fmt.Sprintf("%s: %s(%d)\n", player, civ, tier)
	}

	session.ChannelMessageSend(msg.ChannelID, output)
	return nil
}

/*
	Read civ list from CSV
*/
func readCivList() ([][]string, error) {
	file, err := os.Open(config.CivListPath)
	if err != nil {
		return nil, err
	}
	reader := csv.NewReader(file)
	civs, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	return civs[1:], nil
}

/*
	Set the tiers for this text channel
*/
func setTiers(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	tiers := strings.Split(args[1], "-")
	if len(tiers) != 2 {
		session.ChannelMessageSend(msg.ChannelID, "Wrong number of tiers")
		return nil
	}
	tier1, err := strconv.Atoi(tiers[0])
	if err != nil || tier1 < DefaultMaxTier || tier1 > DefaultMinTier {
		session.ChannelMessageSend(msg.ChannelID, "Invalid tier")
		return nil
	}
	tier2, err := strconv.Atoi(tiers[1])
	if err != nil || tier2 < DefaultMaxTier || tier2 > DefaultMinTier {
		session.ChannelMessageSend(msg.ChannelID, "Invalid tier")
		return nil
	}

	settings, ok := Settings[msg.ChannelID]
	if !ok {
		settings = &Setting{DefaultMaxTier, DefaultMinTier, []string{}}
	}
	if tier1 < tier2 {
		settings.MaxTier = tier1
		settings.MinTier = tier2
	} else {
		settings.MaxTier = tier2
		settings.MinTier = tier1
	}
	session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf(
		"Min and max tiers set to %d and %d",
		Settings[msg.ChannelID].MinTier,
		Settings[msg.ChannelID].MaxTier,
	),
	)
	return nil
}
