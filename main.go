package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"reverse-short-url/biliutil"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"mvdan.cc/xurls/v2"
)

var (
	help     = flag.Bool("h", false, "This help.")
	botToken = flag.String("b", "", "Telegram bot token.")
	proxyURL = flag.String("p", "", "Proxy for web requests. (e.g., http://127.0.0.1:8080, socks5://127.0.0.1:1080)")
)

const bilibiliHost = "www.bilibili.com"

var botAPI *tgbotapi.BotAPI
var httpTransport *http.Transport

func getRedirectURL(shortURL string) ([]string, error) {

	if _, err := url.Parse(shortURL); err != nil {
		return []string{}, err
	}

	result := append(make([]string, 0), shortURL)
	client := http.Client{
		Timeout:   15 * time.Second,
		Transport: httpTransport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}

			newRaw := req.URL.String()
			valid, err := isChangeValid(result[len(result)-1], newRaw)
			if err != nil {
				return http.ErrUseLastResponse
			}
			if valid {
				result = append(result, newRaw)
			}
			return nil
		},
	}
	req, err := http.NewRequest(http.MethodGet, shortURL, nil)
	if err != nil {
		return []string{}, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return []string{}, err
	}
	defer resp.Body.Close()

	if len(result) > 1 {
		return result, nil
	}
	return []string{}, fmt.Errorf("url: %s has no redirect", shortURL)
}

func isChangeValid(oldRaw string, newRaw string) (bool, error) {
	oldURL, err := url.Parse(oldRaw)
	if err != nil {
		return false, err
	}

	newURL, err := url.Parse(newRaw)
	if err != nil {
		return false, err
	}

	if oldURL.Path == newURL.Path {
		return false, nil
	}
	return true, nil
}

func findURL(text string) ([]string, error) {
	urls := xurls.Relaxed().FindAllString(text, -1)
	for i, u := range urls {
		if !strings.HasPrefix(u, "http") {
			urls[i] = "http://" + u
		}
	}

	if len(urls) == 0 {
		return []string{}, fmt.Errorf("url not found")
	}
	return urls, nil
}

func checkBiliBV(rawurl string) (bv string, av string, err error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return "", "", err
	}
	if u.Host != bilibiliHost {
		return "", "", fmt.Errorf("not bilibili")
	}
	pathElements := strings.Split(u.Path, "/")
	for _, e := range pathElements {
		if strings.HasPrefix(e, "BV") {
			bv = e
			break
		}
	}
	if bv != "" {
		av = "av" + strconv.FormatInt(biliutil.Decode(bv), 10)
	}

	if bv != "" && av != "" {
		return bv, av, nil
	} else {
		return "", "", fmt.Errorf("convert bv to av failed")
	}
}

func ReverseShortURL(msg string) (string, error) {
	shortURLs, err := findURL(msg)
	if err != nil {
		return "", err
	}

	urlMap := make(map[string][]string)

	group := sync.WaitGroup{}
	for _, u := range shortURLs {

		group.Add(1)
		go func(list map[string][]string, s string) {

			list[s], err = getRedirectURL(s)
			if err != nil {
				log.Print(err)
			}
			group.Done()

		}(urlMap, u)
	}
	group.Wait()

	var result string
	for _, s := range shortURLs {
		longURL := urlMap[s]
		lastURL := s
		if len(longURL) > 0 {
			result += fmt.Sprintf("✅ %s\n", strings.Join(longURL, "\n➡️ "))
			lastURL = longURL[len(longURL)-1]
		} else {
			result += fmt.Sprintf("❌ %s\n", s)
		}
		//bilibili bv to av
		if bv, av, err := checkBiliBV(lastURL); err == nil {
			result += fmt.Sprintf("🆎 %s ➡️ %s\n", bv, av)
		}
		result += "\n"
	}
	result = strings.TrimSuffix(result, "\n\n")
	return result, nil
}

func handleUpdates(updates tgbotapi.UpdatesChannel) {
	for update := range updates {
		if update.Message == nil {
			continue
		}

		if (len(update.Message.Text) == 0 && len(update.Message.Caption) == 0) || strings.HasPrefix(update.Message.Text, "/") {
			continue
		}

		go func(receive *tgbotapi.Message) {
			text := receive.Text
			if len(text) == 0 {
				text = receive.Caption
			}
			reply, err := ReverseShortURL(text)
			if err == nil {
				msg := tgbotapi.NewMessage(receive.Chat.ID, reply)
				msg.ReplyToMessageID = receive.MessageID
				msg.DisableWebPagePreview = true
				botAPI.Send(msg)
			}

		}(update.Message)
	}
}

func ServeBot() *tgbotapi.BotAPI {
	bot, err := tgbotapi.NewBotAPI(*botToken)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	go handleUpdates(updates)

	return bot
}

func checkParams() error {
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}
	if *botToken == "" {
		return fmt.Errorf("invalid bot token")
	}
	if *proxyURL != "" {
		_, err := url.Parse(*proxyURL)
		if err != nil {
			return fmt.Errorf("invalid proxy url")
		}
		httpTransport = &http.Transport{
			Proxy: func(r *http.Request) (*url.URL, error) {
				return url.Parse(*proxyURL)
			},
		}
	} else {
		httpTransport = &http.Transport{}
	}
	return nil
}

func main() {
	if err := checkParams(); err != nil {
		fmt.Println("Error: " + err.Error())
		flag.Usage()
		os.Exit(1)
	}

	botAPI = ServeBot()
	defer botAPI.StopReceivingUpdates()

	osSignals := make(chan os.Signal, 1)
	signal.Notify(osSignals, os.Interrupt, syscall.SIGTERM)
	<-osSignals
}
