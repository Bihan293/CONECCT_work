package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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

	// Set webhook asynchronously to не блокировать main
	if cfg.WebhookURL != "" && cfg.WebhookSecret != "" {
		go func() {
			whURL := cfg.WebhookURL + "/webhook/" + cfg.WebhookSecret
			if err := bot.SetWebhook(whURL); err != nil {
				log.Printf("setWebhook warning: %v", err)
			} else {
				log.Printf("webhook set to %s", whURL)
			}
		}()
	}

	// HTTP Handlers с panic recovery
	http.HandleFunc("/webhook/"+cfg.WebhookSecret, recoveryMiddleware(makeWebhookHandler(bot)))
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
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)
	<-quit
	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}
	log.Println("Server exited gracefully")
}

// recoveryMiddleware ловит паники в HTTP handler и логирует их
func recoveryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("panic recovered: %v", rec)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next(w, r)
	}
}
