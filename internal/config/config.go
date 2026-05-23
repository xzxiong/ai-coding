package config

import (
	"os"
)

type Config struct {
	ListenAddr    string
	OpenAIBaseURL string
	OpenAIAPIKey  string
	DefaultModel  string
	DataFile      string
	Debug         bool
}

func Load() *Config {
	return &Config{
		ListenAddr:    getEnv("LISTEN_ADDR", ":9465"),
		OpenAIBaseURL: getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:  getEnv("OPENAI_API_KEY", ""),
		DefaultModel:  getEnv("DEFAULT_MODEL", "gpt-4o"),
		DataFile:      getEnv("DATA_FILE", "usage.db"),
		Debug:         os.Getenv("DEBUG") == "1" || os.Getenv("DEBUG") == "true",
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
