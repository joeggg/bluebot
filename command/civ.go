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
	"github.com/jellydator/ttlcache/v3"
)

var (
	DefaultMaxTier int = 1 // NOTE: tiers are inverse to expected
	DefaultMinTier int = 8
	Settings       *ttlcache.Cache[string, *Setting]
	TTL            = 5 * time.Minute
)

type Setting struct {
	MaxTier int
	MinTier int
	Players []string
}

func HandleCiv(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if Settings == nil {
		CreateSettingsCache()
	}
	if len(args) > 0 && args[0] == "tiers" {
		return setTiers(session, msg, args)
	} else {
		return generateCivs(session, msg, args)
	}
}

func CreateSettingsCache() {
	Settings = ttlcache.New(
		ttlcache.WithTTL[string, *Setting](TTL),
	)
	go Settings.Start()
}

/*
	Generate a random selection of civs for provided or previously saved players within saved
	or default min/max tiers
*/
func generateCivs(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	// Check if settings exists and create new if not
	settings := Settings.Get(msg.ChannelID)
	if settings == nil {
		settings = Settings.Set(msg.ChannelID, &Setting{DefaultMaxTier, DefaultMinTier, args}, TTL)
	} else if len(args) != 0 {
		settings.Value().Players = args
	}
	if len(settings.Value().Players) == 0 {
		session.ChannelMessageSend(msg.ChannelID, "Please provide some players")
		return nil
	}

	civs, err := readCivList()
	if err != nil {
		return err
	}
	// Reduce civ list to only contain selected civs
	for i := 0; i < len(civs); i++ {
		tier, _ := strconv.Atoi(civs[i][2])
		if tier < settings.Value().MaxTier || tier > settings.Value().MinTier {
			civs = append(civs[:i], civs[i+1:]...)
			i--
		}
	}
	if len(civs) < 3*len(settings.Value().Players) {
		session.ChannelMessageSend(msg.ChannelID, "Not enough civs for the criteria given")
		return nil
	}

	// Generate random civs
	source := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(source)
	output := ""
	for _, player := range settings.Value().Players {
		output += player + ": "
		selected := []string{}
		for n := 0; n < 3; n++ {
			i := r1.Intn(len(civs))
			tier, _ := strconv.Atoi(civs[i][2])
			civ := civs[i][1]
			civs = append(civs[:i], civs[i+1:]...)
			selected = append(selected, fmt.Sprintf("%s(%d)", civ, tier))
		}
		output += strings.Join(selected, ", ") + "\n"
	}

	session.ChannelMessageSend(msg.ChannelID, output)
	return nil
}

/*
	Read civ list from CSV
*/
func readCivList() ([][]string, error) {
	file, err := os.Open(config.Cfg.CivListPath)
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

	settings := Settings.Get(msg.ChannelID)
	if settings == nil {
		settings = Settings.Set(msg.ChannelID, &Setting{DefaultMaxTier, DefaultMinTier, []string{}}, TTL)
	}
	if tier1 < tier2 {
		settings.Value().MaxTier = tier1
		settings.Value().MinTier = tier2
	} else {
		settings.Value().MaxTier = tier2
		settings.Value().MinTier = tier1
	}
	session.ChannelMessageSend(msg.ChannelID, fmt.Sprintf(
		"Min and max tiers set to %d and %d",
		settings.Value().MinTier,
		settings.Value().MaxTier,
	),
	)
	return nil
}
