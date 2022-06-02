package util

import "github.com/bwmarrin/discordgo"

type HandlerFunc func(*discordgo.Session, *discordgo.MessageCreate, []string) error
