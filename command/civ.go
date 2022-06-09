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
	MaxTier int
	MinTier int
	Players []string
)

func HandleCiv(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if len(args) > 0 && args[0] == "tiers" {
		return setTiers(session, msg, args)
	} else {
		return generateCivs(session, msg, args)
	}
}

func generateCivs(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	file, err := os.Open(config.CivListPath)
	if err != nil {
		return err
	}
	reader := csv.NewReader(file)
	civs, err := reader.ReadAll()
	if err != nil {
		return err
	}
	civs = civs[1:]

	source := rand.NewSource(time.Now().UnixNano())
	r1 := rand.New(source)
	output := ""
	for _, player := range args {
		tier := -1
		num := 0
		for tier < MinTier || tier > MaxTier {
			num = r1.Intn(len(civs))
			tier, err = strconv.Atoi(civs[num][2])
			if err != nil {
				return err
			}
			civs = append(civs[:num], civs[num+1:]...)
		}
		output += player + ": " + civs[num][1] + ", Tier: " + civs[num][2] + "\n"
	}

	session.ChannelMessageSend(msg.ChannelID, output)
	return nil
}

func setTiers(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	tiers := strings.Split(args[1], "-")
	if len(tiers) != 2 {
		session.ChannelMessageSend(msg.ChannelID, "Wrong number of tiers")
		return nil
	}
	tier1, err := strconv.Atoi(tiers[0])
	if err != nil || tier1 < 1 || tier1 > 8 {
		session.ChannelMessageSend(msg.ChannelID, "Invalid tier")
		return nil
	}
	tier2, err := strconv.Atoi(tiers[1])
	if err != nil || tier2 < 1 || tier2 > 8 {
		session.ChannelMessageSend(msg.ChannelID, "Invalid tier")
		return nil
	}

	if tier1 > tier2 {
		MinTier, MaxTier = tier1, tier2
	} else {
		MinTier, MaxTier = tier2, tier1
	}
	session.ChannelMessageSend(
		msg.ChannelID, fmt.Sprintf("Min and max tiers set to %d and %d", MinTier, MaxTier),
	)
	return nil
}
