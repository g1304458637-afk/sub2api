package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func newTestSunoAdapter(server *httptest.Server, interval time.Duration) *SunoAdapter {
	return &SunoAdapter{
		baseURL:      strings.TrimRight(server.URL, "/"),
		apiKey:       "sk-test",
		httpClient:   server.Client(),
		pollInterval: interval,
	}
}

func TestMusicProviderForModelRegistry(t *testing.T) {
	factory, ok := MusicProviderForModel("suno-v4")
	require.True(t, ok)
	require.NotNil(t, factory)

	_, ok = MusicProviderForModel("SUNO")
	require.True(t, ok)

	_, ok = MusicProviderForModel("udio-v2")
	require.False(t, ok)

	_, ok = MusicProviderForModel("")
	require.False(t, ok)
}

func TestSunoAdapterGenerateSuccessAfterPolling(t *testing.T) {
	var gotAuth string
	var generateBody map[string]any
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/generate":
			raw, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(raw, &generateBody))
			// callBackUrl deliberately omitted.
			require.NotContains(t, string(raw), "callBackUrl")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":200,"data":{"taskId":"task-1"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/generate/record-info":
			require.Equal(t, "task-1", r.URL.Query().Get("taskId"))
			polls++
			payload := `{"code":200,"data":{"status":"PENDING","response":{"sunoData":[]}}}`
			if polls >= 2 {
				payload = `{"code":200,"data":{"status":"SUCCESS","response":{"sunoData":[{"audio_url":"https://cdn.example.test/a.mp3","duration":182.5,"model_name":"suno-v4"}]}}}`
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(payload))
		default:
			t.Errorf("unexpected upstream call %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	adapter := newTestSunoAdapter(server, time.Millisecond)

	result, err := adapter.Generate(context.Background(), MusicGenerationRequest{
		Model:        "suno-v4",
		Prompt:       "a calm piano piece",
		Instrumental: true,
	})
	require.NoError(t, err)
	require.Equal(t, "Bearer sk-test", gotAuth)
	require.Equal(t, false, generateBody["customMode"])
	require.Equal(t, true, generateBody["instrumental"])
	require.Equal(t, "suno-v4", generateBody["model"])
	require.Equal(t, "a calm piano piece", generateBody["prompt"])
	require.GreaterOrEqual(t, polls, 2)
	require.Equal(t, "https://cdn.example.test/a.mp3", result.AudioURL)
	require.Equal(t, 182.5, result.DurationSec)
	require.Equal(t, "audio/mpeg", result.ContentType)
	require.Equal(t, "mp3", result.Format)
}

func TestSunoAdapterFailsOnUpstreamFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/generate" {
			_, _ = w.Write([]byte(`{"code":200,"data":{"taskId":"task-2"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"status":"SENSITIVE_WORD_ERROR","failMsg":"prompt contains sensitive words"}}`))
	}))
	t.Cleanup(server.Close)
	adapter := newTestSunoAdapter(server, time.Millisecond)

	_, err := adapter.Generate(context.Background(), MusicGenerationRequest{Model: "suno-v4", Prompt: "x"})
	require.Error(t, err)
	var providerErr *MusicProviderError
	require.ErrorAs(t, err, &providerErr)
	require.Contains(t, err.Error(), "sensitive words")
}

func TestSunoAdapterFailsOnNon200EnvelopeCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":401,"message":"bad credentials"}`))
	}))
	t.Cleanup(server.Close)
	adapter := newTestSunoAdapter(server, time.Millisecond)

	_, err := adapter.Generate(context.Background(), MusicGenerationRequest{Model: "suno-v4", Prompt: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "bad credentials")
}

func TestSunoAdapterTimesOutWhenUpstreamKeepsPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/generate" {
			_, _ = w.Write([]byte(`{"code":200,"data":{"taskId":"task-3"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"data":{"status":"PENDING"}}`))
	}))
	t.Cleanup(server.Close)
	adapter := newTestSunoAdapter(server, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := adapter.Generate(ctx, MusicGenerationRequest{Model: "suno-v4", Prompt: "x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "timed out")
}

func TestSunoAdapterDeadlineDuringHTTP(t *testing.T) {
	for _, stage := range []string{"create", "poll"} {
		t.Run(stage, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if stage == "poll" && r.URL.Path == sunoGeneratePath {
					_, _ = w.Write([]byte(`{"code":200,"data":{"taskId":"slow-task"}}`))
					return
				}
				<-r.Context().Done()
			}))
			t.Cleanup(server.Close)
			adapter := newTestSunoAdapter(server, time.Millisecond)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			_, err := adapter.Generate(ctx, MusicGenerationRequest{Model: "suno-v4", Prompt: "test"})
			require.ErrorContains(t, err, "music generation timed out")
			var providerErr *MusicProviderError
			require.ErrorAs(t, err, &providerErr)
		})
	}
}
