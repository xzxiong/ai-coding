package config

import (
	"os"
	"strconv"
)

type Config struct {
	ListenAddr    string
	OpenAIBaseURL string
	OpenAIAPIKey  string
	DefaultModel  string
	DataFile      string
	Debug         bool
	// ContextOverflowTokens is the estimated-token threshold above which an
	// upstream 400 is treated as a context-length overflow (mapped to a
	// prompt-too-long error). Smaller 400s fall through to a transient 502.
	ContextOverflowTokens int
	// StripStopSequences drops the request's stop sequences before forwarding.
	// Some upstream gateways (e.g. new-api/one-api relays) reject any non-empty
	// stop with an opaque 400, which the proxy then surfaces as a 502. Enable
	// this for such upstreams so requests that carry stop_sequences (e.g. Claude
	// Code's security monitor) still succeed.
	StripStopSequences bool
}

func Load() *Config {
	return &Config{
		ListenAddr:            getEnv("LISTEN_ADDR", ":9465"),
		OpenAIBaseURL:         getEnv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:          getEnv("OPENAI_API_KEY", ""),
		DefaultModel:          getEnv("DEFAULT_MODEL", "gpt-4o"),
		DataFile:              getEnv("DATA_FILE", "usage.db"),
		Debug:                 os.Getenv("DEBUG") == "1" || os.Getenv("DEBUG") == "true",
		ContextOverflowTokens: getEnvInt("CONTEXT_OVERFLOW_TOKENS", 200000),
		StripStopSequences:    os.Getenv("STRIP_STOP_SEQUENCES") == "1" || os.Getenv("STRIP_STOP_SEQUENCES") == "true",
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
