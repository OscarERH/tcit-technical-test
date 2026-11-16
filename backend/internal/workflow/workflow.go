// Package workflow is the client of the Symfony Workflow service, which owns
// the post lifecycle (which transitions are allowed from each status).
package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/oscarruiz/tcit-posts/backend/internal/httpx"
	"github.com/oscarruiz/tcit-posts/backend/internal/posts"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *Client) Transitions(ctx context.Context) ([]posts.Transition, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/workflow/posts", nil)
	if err != nil {
		return nil, err
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("workflow definition: unexpected status %d", res.StatusCode)
	}

	var definition struct {
		Transitions []posts.Transition `json:"transitions"`
	}
	if err := json.NewDecoder(res.Body).Decode(&definition); err != nil {
		return nil, fmt.Errorf("workflow definition: %w", err)
	}
	return definition.Transitions, nil
}

type applyRequest struct {
	Status     string `json:"status"`
	Transition string `json:"transition"`
}

type applyResponse struct {
	Status string `json:"status"`
	Error  struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) Apply(ctx context.Context, status, transition string) (string, error) {
	body, err := json.Marshal(applyRequest{Status: status, Transition: transition})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/workflow/posts/apply", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return "", &httpx.Error{Status: http.StatusBadGateway, Message: "Workflow service is unavailable"}
	}
	defer res.Body.Close()

	var parsed applyResponse
	decodeErr := json.NewDecoder(res.Body).Decode(&parsed)

	switch {
	case res.StatusCode == http.StatusOK && decodeErr == nil && parsed.Status != "":
		return parsed.Status, nil
	case res.StatusCode == http.StatusConflict || res.StatusCode == http.StatusBadRequest:
		// The workflow rejected the transition: relay its explanation.
		message := parsed.Error.Message
		if message == "" {
			message = "Transition is not allowed"
		}
		return "", &httpx.Error{Status: res.StatusCode, Message: message}
	default:
		return "", &httpx.Error{
			Status:  http.StatusBadGateway,
			Message: fmt.Sprintf("Workflow service failed (status %d)", res.StatusCode),
		}
	}
}
