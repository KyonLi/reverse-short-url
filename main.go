package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"log"
	"mvdan.cc/xurls"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

var botAPI = "MyAwesomeBotToken"

func getRedirectURL(shortURL string) (string, error) {
	originURL, err := url.Parse(shortURL)
	if err != nil {
		return "", err
	}

	result := ""
	client := http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}

		result = req.Response.Header["Location"][0]
		return nil
	}}
	req, err := http.NewRequest(http.MethodGet, shortURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	location, err := url.Parse(result)
	if err != nil || location.Path == originURL.Path {
		return "", fmt.Errorf("url: %s has no redirect", shortURL)
	}

	return result, nil
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

func ReverseShortURL(msg string) (string, error) {
	shortURLs, err := findURL(msg)
	if err != nil {
		return "", err
	}

	urlMap := make(map[string]string)

	group := sync.WaitGroup{}
	for _, u := range shortURLs {

		group.Add(1)
		go func(list map[string]string, s string) {

			urlMap[s], _ = getRedirectURL(s)
			group.Done()

		}(urlMap, u)
	}
	group.Wait()

	var result string
	for _, s := range shortURLs {
		l := urlMap[s]
		if len(l) != 0 {
			result += fmt.Sprintf("✅ %s ➡️ %s\n", s, l)
		} else {
			result += fmt.Sprintf("❌ %s\n", s)
		}
	}
	result = strings.TrimSuffix(result, "\n")
	return result, nil
}

func main() {
	bot, err := tgbotapi.NewBotAPI(botAPI)
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatalln(err)
	}

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
				bot.Send(msg)
			}

		}(update.Message)
	}
}
