package main

import (
	tgbot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
)

type Bot struct {
	api *tgbot.BotAPI
}

func InitBot(token string) *Bot {
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN is required")
	}
	api, err := tgbot.NewBotAPI(token)
	if err != nil {
		log.Fatalf("new bot api: %v", err)
	}
	api.Debug = false
	return &Bot{api: api}
}

func (b *Bot) Shutdown() {
	// nothing for now
}

func (b *Bot) Send(msg tgbot.Chattable) {
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("send error: %v", err)
	}
}

func (b *Bot) SetWebhook(url string) error {
	_, err := b.api.Request(tgbot.Request{Method: "setWebhook", Params: map[string]interface{}{"url": url}})
	return err
}
