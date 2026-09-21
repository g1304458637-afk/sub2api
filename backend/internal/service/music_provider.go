package service

import (
	"context"
	"net/http"
	"strings"
	"time"
)

// MusicGenerationRequest is the normalized async music generation input.
type MusicGenerationRequest struct {
	Model        string
	Prompt       string
	Lyrics       string
	Instrumental bool
	Seconds      int
	Format       string
}

// MusicGenerationResult is what an adapter returns once the upstream finished.
// The gateway self-hosts the audio afterwards (object storage or inline base64)
// because upstream CDN links expire.
type MusicGenerationResult struct {
	AudioURL    string
	AudioBase64 string
	ContentType string
	DurationSec float64
	Format      string
	Model       string
}

// MusicProvider is one upstream music generation adapter.
type MusicProvider interface {
	Generate(ctx context.Context, req MusicGenerationRequest) (*MusicGenerationResult, error)
}

// MusicProviderError carries an upstream failure message into the task error
// surfaced to the polling client.
type MusicProviderError struct {
	Message string
}

func (e *MusicProviderError) Error() string {
	if e == nil || strings.TrimSpace(e.Message) == "" {
		return "music generation failed"
	}
	return e.Message
}

// MusicProviderFactory builds a provider bound to one upstream account's
// credentials. httpClient is injectable for tests; nil means the default.
type MusicProviderFactory func(baseURL, apiKey string, httpClient *http.Client) MusicProvider

// musicProviderRegistry maps model-name prefixes to adapters.
var musicProviderRegistry = []struct {
	prefix  string
	factory MusicProviderFactory
}{
	{prefix: "suno", factory: NewSunoAdapter},
}

// MusicProviderForModel resolves the adapter for a requested model by prefix
// (e.g. "suno*", "suno-v4"). ok is false for unknown models — the caller must
// fail the task with a clear error instead of guessing an adapter.
func MusicProviderForModel(model string) (MusicProviderFactory, bool) {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return nil, false
	}
	for _, entry := range musicProviderRegistry {
		if strings.HasPrefix(model, entry.prefix) {
			return entry.factory, true
		}
	}
	return nil, false
}

// defaultMusicProviderHTTPClient bounds a single upstream HTTP call; the overall
// generation window (including polling) is bounded by the caller's context.
func defaultMusicProviderHTTPClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}
