package command

import (
	"bluebot/config"
	"log"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/fogleman/gg"
)

/*
	Posts an image to the channel with text overlaid (given in the command)
	This is a base function that's used to create a command for each image setting
	provided in the images.json config.
*/
func HandleImage(
	session *discordgo.Session,
	msg *discordgo.MessageCreate,
	setting *config.ImageSetting,
	args []string,
) error {
	if len(args) < 1 {
		session.ChannelMessageSend(msg.ChannelID, "Need something to write")
		return nil
	}

	log.Println(setting)
	img, err := gg.LoadImage(config.Cfg.ImagePath + "/" + setting.Filename)
	if err != nil {
		return err
	}

	dc := gg.NewContext(img.Bounds().Dx(), img.Bounds().Dy())
	if err := dc.LoadFontFace(config.Cfg.ImageFontPath, 40); err != nil {
		return err
	}
	// Create the image
	dc.SetRGB(0, 0, 0)
	dc.DrawImage(img, 0, 0)
	dc.DrawStringAnchored(
		strings.Join(args, " "), float64(setting.TextX), float64(setting.TextY), 0.5, 0.5,
	)
	dc.SavePNG(config.Cfg.DataPath + "/out.png")

	r, err := os.Open(config.Cfg.DataPath + "/out.png")
	if err != nil {
		return err
	}
	session.ChannelFileSend(msg.ChannelID, "pic.png", r)
	return nil
}
