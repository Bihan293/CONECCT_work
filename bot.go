package main

import (
	tgbot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"log"
)

type Bot struct {
	api *tgbot.BotAPI
}

// InitBot инициализирует бота с токеном
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

// Shutdown пока пустой, оставлен для будущего расширения
func (b *Bot) Shutdown() {
	// nothing for now
}

// Send отправляет любое сообщение в Telegram
func (b *Bot) Send(msg tgbot.Chattable) {
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("send error: %v", err)
	}
}

// SetWebhook устанавливает вебхук для бота
func (b *Bot) SetWebhook(url string) error {
	webhookConfig, err := tgbot.NewWebhook(url)
	if err != nil {
		return err
	}
	_, err = b.api.Request(webhookConfig)
	return err
}
