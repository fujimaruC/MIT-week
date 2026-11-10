// Package config loads and validates environment configuration.
// It fails fast: a bad or missing required value stops the server at boot
// rather than surfacing as a confusing runtime error later.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully validated runtime configuration.
type Config struct {
	Host          string
	Port          int
	Environment   string
	PublicAppURL  string
	TrustedProxy  []string
	RouterBaseURL string
	RouterAPIKey  string
	RouterModel   string
	Reasoning     string
	Retrieval     bool
	RetrievalURL  string
	LLMMock       bool
	SigningKey    string
	SafeBrowsing  string
	FactCheckKey  string
	// TavilyKey (TAVILY_API_KEY) searches the allowlisted fact-checkers'
	// sites when no Google Fact Check key is set.
	TavilyKey string
	// TavilyBackupKey (TAVILY_API_KEY_2) is a second account's key, used
	// once the first key's credits run out (Tavily's free plan: 1,000 a month).
	TavilyBackupKey string
	// Sightengine credentials for the optional AI-image detector. Both
	// empty: no detector (the AI pillar uses metadata and the model).
	SightengineUser   string
	SightengineSecret string
	// DomainAge turns the RDAP domain-age lookup on (DOMAIN_AGE_LOOKUP,
	// default true).
	DomainAge   bool
	MaxUploadMB int64
	HardTimeout time.Duration
	// VideoTimeout is the budget of a social video link audit (download,
	// keyframes, model): VIDEO_TIMEOUT_MS, default 60 s.
	VideoTimeout time.Duration
	// LLMAudioInput / FallbackAudioInput: the gateway takes audio parts
	// (LLM_AUDIO_INPUT, LLM_FALLBACK_AUDIO_INPUT; Gemini via 9router does).
	LLMAudioInput      bool
	FallbackAudioInput bool
	CacheTTL           time.Duration
	MetricsAddr        string
	// ClientIPHeader names the header a trusted proxy puts the client address
	// in (Fly-Client-IP on Fly.io, X-Real-IP on Railway).
	ClientIPHeader string
	// ExtraOrigins are additional exact CORS origins (e.g. a custom domain
	// next to the vercel.app one).
	ExtraOrigins []string
	// LLMExtraBody holds gateway-specific fields merged into every model
	// request (LLM_EXTRA_BODY, a JSON object), e.g. OpenRouter fallbacks.
	LLMExtraBody map[string]any
	// Fallback gateway (LLM_FALLBACK_*), tried when the first one fails:
	// base URL, key and model together, or none.
	FallbackBaseURL string
	FallbackAPIKey  string
	FallbackModel   string
	FallbackExtra   map[string]any
	// Telegram bot (PRD P2). Empty token disables it.
	TelegramToken      string
	TelegramSecret     string
	TelegramMode       string
	TelegramWebhookURL string
}

// IsProduction reports whether the server is running in production mode.
func (c *Config) IsProduction() bool { return c.Environment == "production" }

// Addr returns the listen address.
func (c *Config) Addr() string { return fmt.Sprintf("%s:%d", c.Host, c.Port) }

// LLMBudget is how long the model may take inside one request. The default
// 12 s request keeps the 9 s budget; a slower gateway (a 9router combo needs
// 12-17 s per image) gets HARD_TIMEOUT_MS minus the 3 s that the text check
// and scoring need after the model answers.
func (c *Config) LLMBudget() time.Duration {
	const floor, after = 9 * time.Second, 3 * time.Second
	if d := c.HardTimeout - after; d > floor {
		return d
	}
	return floor
}

// Load reads configuration from the environment and validates it.
// Every error returned here is safe to print at boot: it names the variable,
// never its value.
func Load() (*Config, error) {
	c := &Config{
		Host:          env("SERVER_HOST", "127.0.0.1"),
		Environment:   env("ENVIRONMENT", "development"),
		PublicAppURL:  env("PUBLIC_APP_URL", "http://localhost:5173"),
		RouterBaseURL: env("NINEROUTER_BASE_URL", "http://localhost:20128/v1"),
		RouterAPIKey:  os.Getenv("NINEROUTER_API_KEY"),
		RouterModel:   os.Getenv("NINEROUTER_MODEL"),
		Reasoning:     env("NINEROUTER_REASONING_EFFORT", "low"),
		RetrievalURL:  env("FACT_SEARCH_URL", "https://www.bing.com/news/search"),
		SigningKey:    os.Getenv("REPORT_SIGNING_KEY"),
		// One Google Cloud key may serve both APIs; the specific variable wins.
		SafeBrowsing:      env("GOOGLE_SAFE_BROWSING_KEY", os.Getenv("GOOGLE_API_KEY")),
		FactCheckKey:      env("GOOGLE_FACTCHECK_KEY", os.Getenv("GOOGLE_API_KEY")),
		TavilyKey:         os.Getenv("TAVILY_API_KEY"),
		TavilyBackupKey:   os.Getenv("TAVILY_API_KEY_2"),
		SightengineUser:   os.Getenv("SIGHTENGINE_API_USER"),
		SightengineSecret: os.Getenv("SIGHTENGINE_API_SECRET"),
		MetricsAddr:       os.Getenv("METRICS_ADDR"),
		ClientIPHeader:    os.Getenv("CLIENT_IP_HEADER"),
		TelegramToken:     os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramSecret:    os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		TelegramMode:      env("TELEGRAM_MODE", "webhook"),
		// The public URL Telegram should push updates to, e.g.
		// https://amba-api.fly.dev/telegram/webhook. Empty: register it by hand.
		TelegramWebhookURL: os.Getenv("TELEGRAM_WEBHOOK_URL"),
	}

	var errs []error

	// Railway and most PaaS hosts inject PORT; SERVER_PORT still wins.
	port, err := strconv.Atoi(env("SERVER_PORT", env("PORT", "8080")))
	if err != nil || port < 1 || port > 65535 {
		errs = append(errs, errors.New("SERVER_PORT: must be an integer between 1 and 65535"))
	}
	c.Port = port

	c.LLMMock, err = strconv.ParseBool(env("LLM_MOCK", "true"))
	if err != nil {
		errs = append(errs, errors.New("LLM_MOCK: must be true or false"))
	}
	c.DomainAge, err = strconv.ParseBool(env("DOMAIN_AGE_LOOKUP", "true"))
	if err != nil {
		errs = append(errs, errors.New("DOMAIN_AGE_LOOKUP: must be true or false"))
	}

	c.Retrieval, err = strconv.ParseBool(env("FACT_RETRIEVAL", "true"))
	if err != nil {
		errs = append(errs, errors.New("FACT_RETRIEVAL: must be true or false"))
	}

	switch c.Reasoning {
	case "low", "medium", "high", "minimal":
	default:
		errs = append(errs, errors.New("NINEROUTER_REASONING_EFFORT: must be minimal, low, medium or high"))
	}

	mb, err := strconv.ParseInt(env("MAX_UPLOAD_SIZE_MB", "10"), 10, 64)
	if err != nil || mb < 1 || mb > 25 {
		errs = append(errs, errors.New("MAX_UPLOAD_SIZE_MB: must be an integer between 1 and 25"))
	}
	c.MaxUploadMB = mb

	ms, err := strconv.Atoi(env("HARD_TIMEOUT_MS", "20000"))
	if err != nil || ms < 1000 || ms > 60000 {
		errs = append(errs, errors.New("HARD_TIMEOUT_MS: must be an integer between 1000 and 60000"))
	}
	c.HardTimeout = time.Duration(ms) * time.Millisecond

	vms, err := strconv.Atoi(env("VIDEO_TIMEOUT_MS", "60000"))
	if err != nil || vms < 10000 || vms > 120000 {
		errs = append(errs, errors.New("VIDEO_TIMEOUT_MS: must be an integer between 10000 and 120000"))
	}
	c.VideoTimeout = time.Duration(vms) * time.Millisecond
	c.LLMAudioInput, err = strconv.ParseBool(env("LLM_AUDIO_INPUT", "false"))
	if err != nil {
		errs = append(errs, errors.New("LLM_AUDIO_INPUT: must be true or false"))
	}
	c.FallbackAudioInput, err = strconv.ParseBool(env("LLM_FALLBACK_AUDIO_INPUT", "false"))
	if err != nil {
		errs = append(errs, errors.New("LLM_FALLBACK_AUDIO_INPUT: must be true or false"))
	}

	ttl, err := strconv.Atoi(env("CACHE_TTL_SECONDS", "600"))
	if err != nil || ttl < 0 {
		errs = append(errs, errors.New("CACHE_TTL_SECONDS: must be a non-negative integer"))
	}
	c.CacheTTL = time.Duration(ttl) * time.Second

	if raw := os.Getenv("TRUSTED_PROXY_CIDRS"); strings.TrimSpace(raw) != "" {
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				c.TrustedProxy = append(c.TrustedProxy, p)
			}
		}
	}
	for _, o := range strings.Split(os.Getenv("CORS_EXTRA_ORIGINS"), ",") {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			if !strings.HasPrefix(o, "https://") && !strings.HasPrefix(o, "http://") {
				errs = append(errs, errors.New("CORS_EXTRA_ORIGINS: every origin must start with http:// or https://"))
				continue
			}
			c.ExtraOrigins = append(c.ExtraOrigins, o)
		}
	}
	c.PublicAppURL = strings.TrimRight(c.PublicAppURL, "/")
	if c.ClientIPHeader != "" && len(c.TrustedProxy) == 0 {
		errs = append(errs, errors.New("CLIENT_IP_HEADER: requires TRUSTED_PROXY_CIDRS (use * when the host is only reachable through its proxy)"))
	}
	switch c.TelegramMode {
	case "webhook", "polling", "off":
	default:
		errs = append(errs, errors.New("TELEGRAM_MODE: must be webhook, polling or off"))
	}
	if c.TelegramToken != "" && c.TelegramMode == "webhook" && len(c.TelegramSecret) < 16 {
		errs = append(errs, errors.New("TELEGRAM_WEBHOOK_SECRET: required (>= 16 chars) for webhook mode"))
	}
	if c.TelegramSecret != "" && !validSecretToken(c.TelegramSecret) {
		errs = append(errs, errors.New("TELEGRAM_WEBHOOK_SECRET: only A-Z, a-z, 0-9, _ and - are allowed (Telegram rule), max 256"))
	}
	if c.TelegramWebhookURL != "" && !strings.HasPrefix(c.TelegramWebhookURL, "https://") {
		errs = append(errs, errors.New("TELEGRAM_WEBHOOK_URL: must start with https://"))
	}

	if (c.SightengineUser == "") != (c.SightengineSecret == "") {
		errs = append(errs, errors.New("SIGHTENGINE_API_USER, SIGHTENGINE_API_SECRET: set both or neither"))
	}

	if raw := strings.TrimSpace(os.Getenv("LLM_EXTRA_BODY")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.LLMExtraBody); err != nil || c.LLMExtraBody == nil {
			errs = append(errs, errors.New("LLM_EXTRA_BODY: must be a JSON object, e.g. {\"reasoning\":{\"enabled\":false}}"))
		}
	}
	c.FallbackBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_FALLBACK_BASE_URL")), "/")
	c.FallbackAPIKey = os.Getenv("LLM_FALLBACK_API_KEY")
	c.FallbackModel = strings.TrimSpace(os.Getenv("LLM_FALLBACK_MODEL"))
	someSet := c.FallbackBaseURL != "" || c.FallbackAPIKey != "" || c.FallbackModel != ""
	allSet := c.FallbackBaseURL != "" && c.FallbackAPIKey != "" && c.FallbackModel != ""
	if someSet && !allSet {
		errs = append(errs, errors.New("LLM_FALLBACK_BASE_URL, LLM_FALLBACK_API_KEY, LLM_FALLBACK_MODEL: set all three or none"))
	}
	if raw := strings.TrimSpace(os.Getenv("LLM_FALLBACK_EXTRA_BODY")); raw != "" {
		if err := json.Unmarshal([]byte(raw), &c.FallbackExtra); err != nil || c.FallbackExtra == nil {
			errs = append(errs, errors.New("LLM_FALLBACK_EXTRA_BODY: must be a JSON object"))
		}
	}

	if !c.LLMMock {
		if c.RouterAPIKey == "" {
			errs = append(errs, errors.New("NINEROUTER_API_KEY: required when LLM_MOCK=false"))
		}
		if c.RouterModel == "" {
			errs = append(errs, errors.New("NINEROUTER_MODEL: required when LLM_MOCK=false"))
		}
	}

	// Production must be able to sign share tokens; without a key every
	// /verify would fail closed, so refuse to boot instead.
	if c.IsProduction() && len(c.SigningKey) < 32 {
		errs = append(errs, errors.New("REPORT_SIGNING_KEY: required (>= 32 bytes) in production"))
	}
	if c.IsProduction() && !strings.HasPrefix(c.PublicAppURL, "https://") {
		errs = append(errs, errors.New("PUBLIC_APP_URL: must be an https:// origin in production"))
	}
	// The mock model invents deterministic findings. Real users must never
	// receive them (STATUS.md rule 4), so production refuses to boot with it.
	if c.IsProduction() && c.LLMMock {
		errs = append(errs, errors.New("LLM_MOCK: must be false in production (the mock invents findings)"))
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return c, nil
}

// validSecretToken applies Telegram's secret_token alphabet.
func validSecretToken(s string) bool {
	if len(s) > 256 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
