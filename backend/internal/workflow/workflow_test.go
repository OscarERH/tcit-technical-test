package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/oscarruiz/tcit-posts/backend/internal/httpx"
	"github.com/oscarruiz/tcit-posts/backend/internal/posts"
	"github.com/oscarruiz/tcit-posts/backend/internal/workflow"
)

func newClient(t *testing.T, handler http.HandlerFunc) *workflow.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return workflow.NewClient(server.URL + "/")
}

func TestTransitionsReadsTheDefinition(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/workflow/posts" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"name":"post_publishing","transitions":[
			{"name":"publish","from":["draft"],"to":"published"},
			{"name":"archive","from":["published"],"to":"archived"}]}`)
	})

	got, err := client.Transitions(context.Background())
	if err != nil {
		t.Fatalf("Transitions: %v", err)
	}
	want := []posts.Transition{
		{Name: "publish", From: []string{"draft"}, To: "published"},
		{Name: "archive", From: []string{"published"}, To: "archived"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestTransitionsFailsOnUpstreamError(t *testing.T) {
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	if _, err := client.Transitions(context.Background()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestApplyReturnsTheNewStatus(t *testing.T) {
	var sent map[string]string
	client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/workflow/posts/apply" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&sent)
		_, _ = io.WriteString(w, `{"status":"published","enabledTransitions":["archive"]}`)
	})

	status, err := client.Apply(context.Background(), "draft", "publish")
	if err != nil || status != "published" {
		t.Fatalf("got (%q, %v)", status, err)
	}
	if sent["status"] != "draft" || sent["transition"] != "publish" {
		t.Fatalf("sent %v", sent)
	}
}

func TestApplyMapsFailures(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantStatus  int
		wantMessage string
	}{
		{"rejected transition relays the reason", http.StatusConflict,
			`{"error":{"message":"Transition \"archive\" is not allowed from status \"draft\""}}`,
			http.StatusConflict, `Transition "archive" is not allowed from status "draft"`},
		{"rejection without a reason gets a default", http.StatusConflict, `{}`,
			http.StatusConflict, "Transition is not allowed"},
		{"unknown transition", http.StatusBadRequest, `{"error":{"message":"Unknown transition \"x\""}}`,
			http.StatusBadRequest, `Unknown transition "x"`},
		{"workflow crash", http.StatusInternalServerError, `<html>boom</html>`,
			http.StatusBadGateway, "Workflow service failed (status 500)"},
		{"success without a status", http.StatusOK, `{}`,
			http.StatusBadGateway, "Workflow service failed (status 200)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})

			_, err := client.Apply(context.Background(), "draft", "archive")

			var httpErr *httpx.Error
			if !errors.As(err, &httpErr) {
				t.Fatalf("error = %v, want *httpx.Error", err)
			}
			if httpErr.Status != tt.wantStatus || httpErr.Message != tt.wantMessage {
				t.Fatalf("got %d %q, want %d %q", httpErr.Status, httpErr.Message, tt.wantStatus, tt.wantMessage)
			}
		})
	}
}

func TestApplyReportsUnreachableServiceAsBadGateway(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	client := workflow.NewClient(server.URL)

	_, err := client.Apply(context.Background(), "draft", "publish")

	var httpErr *httpx.Error
	if !errors.As(err, &httpErr) || httpErr.Status != http.StatusBadGateway {
		t.Fatalf("error = %v, want 502", err)
	}
}
