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

var botToken = "MyAwesomeBotToken"

func getRedirectURL(shortURL string) ([]string, error) {

	if _, err := url.Parse(shortURL); err != nil {
		return []string{}, err
	}

	result := append(make([]string, 0), shortURL)
	client := http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}

		newRaw := req.Response.Header["Location"][0]
		valid, err := isChangeValid(result[len(result)-1], newRaw)
		if err != nil {
			return http.ErrUseLastResponse
		}
		if valid {
			result = append(result, newRaw)
		}
		return nil
	}}
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

			urlMap[s], err = getRedirectURL(s)
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
		if len(longURL) > 0 {
			result += fmt.Sprintf("✅ %s\n", strings.Join(longURL, " ➡️ "))
		} else {
			result += fmt.Sprintf("❌ %s\n", s)
		}
	}
	result = strings.TrimSuffix(result, "\n")
	return result, nil
}

func main() {
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatal(err)
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
