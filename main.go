package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	cfg := LoadConfigFromEnv()
	bot := InitBot(cfg.TelegramToken)
	defer bot.Shutdown()

	// Init storage (Postgres preferred; fallback to JSON file)
	if cfg.DatabaseURL != "" {
		if err := InitPostgres(cfg.DatabaseURL); err != nil {
			log.Fatalf("failed to init postgres: %v", err)
		}
		log.Println("Using Postgres storage")
	} else {
		if err := InitJSONStorage("storage.json"); err != nil {
			log.Fatalf("failed to init json storage: %v", err)
		}
		log.Println("Using JSON file storage (fallback). For production use Postgres.")
	}

	// attempt to set webhook if WEBHOOK_URL set
	if cfg.WebhookURL != "" && cfg.WebhookSecret != "" {
		wh := cfg.WebhookURL + "/webhook/" + cfg.WebhookSecret
		if err := bot.SetWebhook(wh); err != nil {
			log.Printf("setWebhook warning: %v", err)
		} else {
			log.Printf("webhook set to %s", wh)
		}
	}

	// HTTP Handlers
	http.HandleFunc("/webhook/"+cfg.WebhookSecret, makeWebhookHandler(bot))
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	// Server config
	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{
		Addr:         ":" + port,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("listening on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}
