package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"html"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/bwmarrin/discordgo"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"
)

var globalConf *config
var artStyles []string = []string{
	"outline", "up", "italic-outline", "slate", "mauve", "graydient",
	"red-blue", "radial", "purple", "green-marble", "rainbow", "aqua",
	"paper-bag", "sunset", "tilt", "blues", "yellow-dash", "chrome",
	"marble-slab", "gray-block", "superhero", "horizon",
	"random",
}

const httpHead string = `<!doctype html>
<head>
<title>discord-wordart</title>
<link rel="stylesheet" type="text/css" href="/static/css/style.css">
<script src="https://cdnjs.cloudflare.com/ajax/libs/jquery/3.5.1/jquery.min.js" integrity="sha512-bLT0Qm9VnAYZDflyKcBaQ2gg0hSYNQrJ8RilYldYQ1FxQYoCLtUjuuRuZo+fjqhx/qtq/1itJ0C2ejDxltZVFg==" crossorigin="anonymous"></script>
<script type="text/javascript" src="/static/js/script.js"></script>
</head><body>`
const httpFoot string = "</body></html>"

type config struct {
	ClientID     string
	ClientSecret string
	BotToken     string
	Port         int
}

func loadConfig(fn string) (*config, error) {
	cf := &config{
		Port: 8356,
	}
	if _, err := toml.DecodeFile(fn, cf); err != nil {
		return nil, err
	}
	return cf, nil
}

func isStyle(s string) bool {
	for _, style := range artStyles {
		if s == style {
			return true
		}
	}
	return false
}

func writeWordArt(style, text string, w io.Writer) {
	w.Write([]byte("<div id=\"wordart\" class=\"wordart-container\"><div class=\"wordart " + style + "\"><span class=\"text\" data-text=\"" + text + "\">" + text + "</span></div></div>"))
}

func webWordart(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	artStyle := vars["style"]
	textB64 := vars["text"]

	textbs, err := base64.URLEncoding.DecodeString(textB64)
	if err != nil {
		log.WithError(err).Error("Failed to decode base-64 string!")
		return
	}
	text := string(textbs)

	w.Write([]byte(httpHead))

	if artStyle == "random" {
		artStyle = artStyles[rand.Intn(len(artStyles)-1)]
	}

	if artStyle == "all" {
		for _, style := range artStyles {
			writeWordArt(style, text, w)
		}
	} else {
		if isStyle(artStyle) {
			writeWordArt(artStyle, text, w)
		} else {
			writeWordArt("rainbow", "Invalid style, dumbass!", w)
		}
	}

	w.Write([]byte(httpFoot))
}

func main() {
	log.Info("Starting discord-wordart...")

	conf, err := loadConfig("config.conf")
	if err != nil {
		log.WithError(err).Fatal("Failed to load configuration!")
	}
	globalConf = conf

	r := mux.NewRouter()
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("web"))))
	r.HandleFunc("/{style}/{text}", webWordart)
	webSrv := &http.Server{
		Handler:      r,
		Addr:         "127.0.0.1:" + strconv.Itoa(conf.Port),
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	discord, err := discordgo.New("Bot " + conf.BotToken)
	if err != nil {
		log.WithError(err).Fatal("Failed to connect to Discord session!")
	}

	discord.AddHandler(messageCreate)

	discord.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsGuildMembers)
	discord.State.TrackChannels = true
	discord.State.TrackMembers = true
	discord.State.TrackRoles = true

	err = discord.Open()
	if err != nil {
		log.WithError(err).Fatal("Failed opening Discord session!")
	}

	log.Info("Discord client running, starting web server")
	log.Info("Web server listening on 127.0.0.1:" + strconv.Itoa(conf.Port))
	if err := webSrv.ListenAndServe(); err != nil {
		log.WithError(err).Error("Web server died")
	}

	discord.Close()
}

func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	content, err := m.ContentWithMoreMentionsReplaced(s)
	if err != nil {
		log.WithError(err).Error("Failed to replace mentions in content!")
		content = m.Content
	}

	if !strings.HasPrefix(content, "~wa") {
		return
	}

	cmds := strings.SplitN(content, " ", 2)
	cmds[0] = strings.ToLower(cmds[0])
	if len(cmds) <= 1 {
		// TODO: help text
		helpMsg := "Available styles:\n"
		i := 0
		for _, s := range artStyles {
			if i > 0 {
				helpMsg += ", "
			}
			helpMsg += s
			i++
		}

		helpMsg += "\n\nTo use a style, run `~wa<style> <message>` where `<style>` is the style you wish to use."

		s.ChannelMessageSend(m.ChannelID, helpMsg)
		return
	}

	cmd := cmds[0]

	// find the style chosen
	chosenStyle := "random"
	if len(cmd) > 3 {
		chosenStyle = cmd[3:]
	}

	// build the art
	text := html.EscapeString(cmds[1])
	log.Info("Make random wordart from: ", text)
	wa, err := doWordArt(chosenStyle, text)
	if err != nil {
		log.WithError(err).Error("Failed to make word art!")
		return
	}

	//  build the message
	ms := &discordgo.MessageSend{
		Embed: &discordgo.MessageEmbed{
			Image: &discordgo.MessageEmbedImage{
				URL: "attachment://wordart.png",
			},
		},
		Files: []*discordgo.File{
			{
				Name:   "wordart.png",
				Reader: bytes.NewReader(wa),
			},
		},
	}

	// send it
	s.ChannelMessageSendComplex(m.ChannelID, ms)
	log.Info("WordArt generated, len = ", len(wa))
}

func doWordArt(style, text string) ([]byte, error) {
	b64 := base64.URLEncoding.EncodeToString([]byte(text))
	// we limit this to 2 million chars (~2MB) since Chrome can't handle more than that :grimacing:
	if len(b64) > 2000000 {
		b64 = b64[:2000000]
	}

	url := "http://localhost:" + strconv.Itoa(globalConf.Port) + "/" + style + "/" + b64
	log.Info(url)

	chromectx, cancel := chromedp.NewContext(context.Background())
	defer cancel()

	var buf []byte

	/*tasks := chromedp.Tasks{
		chromedp.Navigate(url),
		chromedp.WaitVisible("#wordart", chromedp.ByID),
		chromedp.Screenshot("#wordart", &buf, chromedp.NodeVisible, chromedp.ByID),
	}*/

	tasks := chromedp.Tasks{
		chromedp.Navigate(url),
		chromedp.ActionFunc(func(ctx context.Context) error {
			// get layout metrics
			_, _, contentSize, err := page.GetLayoutMetrics().Do(ctx)
			if err != nil {
				return err
			}

			width, height := int64(math.Ceil(contentSize.Width)), int64(math.Ceil(contentSize.Height))

			// force viewport emulation
			err = emulation.SetDeviceMetricsOverride(width, height, 1, false).
				WithScreenOrientation(&emulation.ScreenOrientation{
					Type:  emulation.OrientationTypePortraitPrimary,
					Angle: 0,
				}).
				Do(ctx)
			if err != nil {
				return err
			}

			// capture screenshot
			buf, err = page.CaptureScreenshot().
				WithQuality(90).
				WithClip(&page.Viewport{
					X:      contentSize.X,
					Y:      contentSize.Y,
					Width:  contentSize.Width,
					Height: contentSize.Height,
					Scale:  1,
				}).Do(ctx)
			if err != nil {
				return err
			}
			return nil
		}),
	}

	if err := chromedp.Run(chromectx, tasks); err != nil {
		return nil, err
	}

	return buf, nil
}
