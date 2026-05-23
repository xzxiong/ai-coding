package main

import (
	"log"
	"net/http"

	"github.com/xzxiong/ai-coding/internal/config"
	"github.com/xzxiong/ai-coding/internal/dashboard"
	"github.com/xzxiong/ai-coding/internal/handler"
	"github.com/xzxiong/ai-coding/internal/storage"
)

func main() {
	cfg := config.Load()

	store, err := storage.New(cfg.DataFile)
	if err != nil {
		log.Fatalf("Failed to load usage data: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/messages", handler.NewMessagesHandler(cfg, handler.WithStore(store)))
	mux.Handle("/dashboard/", dashboard.NewHandler(store))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	log.Printf("Starting server on %s", cfg.ListenAddr)
	log.Printf("Proxying to %s", cfg.OpenAIBaseURL)
	log.Printf("Dashboard at http://localhost%s/dashboard/", cfg.ListenAddr)

	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
