package main

import (
	"log"
	"net/http"

	"github.com/xzxiong/ai-coding/internal/config"
	"github.com/xzxiong/ai-coding/internal/handler"
)

func main() {
	cfg := config.Load()

	mux := http.NewServeMux()
	mux.Handle("/v1/messages", handler.NewMessagesHandler(cfg))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	log.Printf("Starting server on %s", cfg.ListenAddr)
	log.Printf("Proxying to %s", cfg.OpenAIBaseURL)

	if err := http.ListenAndServe(cfg.ListenAddr, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
