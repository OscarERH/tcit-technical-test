package gemini_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oscarruiz/tcit-posts/backend/internal/gemini"
	"github.com/oscarruiz/tcit-posts/backend/internal/httpx"
)

const brief = "Campaña de invierno para Andes Run"

// answer builds a generateContent response whose parts carry the given texts.
func answer(parts ...map[string]any) string {
	body, _ := json.Marshal(map[string]any{
		"candidates": []any{map[string]any{"content": map[string]any{"parts": parts}}},
	})
	return string(body)
}

func newClient(t *testing.T, handler http.HandlerFunc) *gemini.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return gemini.NewClient("secret-key", "test-model", server.URL+"/v1beta/")
}

func TestExtractSendsBriefAndParsesAnswer(t *testing.T) {
	var (
		gotPath, gotQuery, gotKey string
		gotBody                   map[string]any
	)
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotKey = r.URL.Path, r.URL.RawQuery, r.Header.Get("x-goog-api-key")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		_, _ = io.WriteString(w, answer(map[string]any{"text": `{"name":"Andes Run","description":"Invierno"}`}))
	})

	name, description, err := client.Extract(context.Background(), brief)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if name != "Andes Run" || description != "Invierno" {
		t.Fatalf("got (%q, %q)", name, description)
	}

	if gotPath != "/v1beta/models/test-model:generateContent" {
		t.Errorf("path = %q", gotPath)
	}
	// The key must travel in a header: URLs end up in access logs.
	if gotKey != "secret-key" || strings.Contains(gotQuery, "secret-key") {
		t.Errorf("key header = %q, query = %q", gotKey, gotQuery)
	}

	contents := gotBody["contents"].([]any)[0].(map[string]any)
	if text := contents["parts"].([]any)[0].(map[string]any)["text"]; text != brief {
		t.Errorf("brief sent = %q", text)
	}
	config := gotBody["generationConfig"].(map[string]any)
	if config["responseMimeType"] != "application/json" || config["responseSchema"] == nil {
		t.Errorf("structured output not requested: %v", config)
	}
}

func TestExtractIgnoresThoughtPartsAndJoinsTheRest(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, answer(
			map[string]any{"text": "razonando…", "thought": true},
			map[string]any{"text": `{"name":"A",`},
			map[string]any{"text": `"description":"B"}`},
		))
	})

	name, description, err := client.Extract(context.Background(), brief)
	if err != nil || name != "A" || description != "B" {
		t.Fatalf("got (%q, %q, %v)", name, description, err)
	}
}

func TestExtractMapsFailuresToGatewayErrors(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantStatus  int
		wantMessage string
	}{
		{"retired model", http.StatusNotFound, `{"error":{}}`, http.StatusBadGateway, `model "test-model" is not available`},
		{"quota", http.StatusTooManyRequests, `{}`, http.StatusBadGateway, "quota exceeded"},
		{"overloaded", http.StatusServiceUnavailable, `{}`, http.StatusBadGateway, "overloaded"},
		{"other upstream error", http.StatusInternalServerError, `{}`, http.StatusBadGateway, "status 500"},
		{"unreadable body", http.StatusOK, `not json`, http.StatusBadGateway, "unreadable"},
		{"no candidates", http.StatusOK, `{"candidates":[]}`, http.StatusBadGateway, "no content"},
		{"answer is not the expected JSON", http.StatusOK, answer(map[string]any{"text": "hola"}), http.StatusBadGateway, "unexpected format"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})

			_, _, err := client.Extract(context.Background(), brief)

			var httpErr *httpx.Error
			if !errors.As(err, &httpErr) {
				t.Fatalf("error = %v, want *httpx.Error", err)
			}
			if httpErr.Status != tt.wantStatus || !strings.Contains(httpErr.Message, tt.wantMessage) {
				t.Fatalf("got %d %q, want %d containing %q", httpErr.Status, httpErr.Message, tt.wantStatus, tt.wantMessage)
			}
			// Privacy: neither the brief nor the key may leak into the error.
			if strings.Contains(httpErr.Message, brief) || strings.Contains(httpErr.Message, "secret-key") {
				t.Fatalf("error leaks sensitive data: %q", httpErr.Message)
			}
		})
	}
}

func TestExtractReportsUnreachableGeminiAsTimeout(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close() // nothing listens on this address any more
	client := gemini.NewClient("secret-key", "test-model", server.URL)

	_, _, err := client.Extract(context.Background(), brief)

	var httpErr *httpx.Error
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusGatewayTimeout {
		t.Fatalf("error = %v, want 504", err)
	}
}
