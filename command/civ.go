package command

import (
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var (
	TierBounds [2]int
	Players    [2]string
)

func HandleCiv(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
	if len(args) > 0 && args[0] == "tiers" {
		return setTiers(session, msg, args)
	} else {
		return generateCivs(session, msg, args)
	}
}

func generateCivs(session *discordgo.Session, msg *discordgo.MessageCreate, args []string) error {
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

	if tier1 < tier2 {
		TierBounds[0], TierBounds[1] = tier1, tier2
	} else {
		TierBounds[0], TierBounds[1] = tier2, tier1
	}
	return nil
}
