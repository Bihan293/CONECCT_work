package main

import (
	"os"
	"strconv"
)

type Config struct {
	TelegramToken      string
	WebhookSecret      string
	WebhookURL         string
	DatabaseURL        string
	DesignGroupID      int64
	ProgrammingGroupID int64
	ContentGroupID     int64
	Port               string
}

func LoadConfigFromEnv() Config {
	return Config{
		TelegramToken:      os.Getenv("TELEGRAM_BOT_TOKEN"),
		WebhookSecret:      os.Getenv("WEBHOOK_SECRET"),
		WebhookURL:         os.Getenv("TELEGRAM_WEBHOOK_URL"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		DesignGroupID:      parseEnvInt64("DESIGN_GROUP_ID"),
		ProgrammingGroupID: parseEnvInt64("PROGRAMMING_GROUP_ID"),
		ContentGroupID:     parseEnvInt64("CONTENT_GROUP_ID"),
		Port:               os.Getenv("PORT"),
	}
}

func parseEnvInt64(k string) int64 {
	v := os.Getenv(k)
	if v == "" {
		return 0
	}
	out, _ := strconv.ParseInt(v, 10, 64)
	return out
}
