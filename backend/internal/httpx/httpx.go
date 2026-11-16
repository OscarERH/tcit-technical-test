// Package httpx holds the small JSON helpers shared by every HTTP handler.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Error is an error that knows which HTTP status it maps to.
type Error struct {
	Status  int
	Message string
	Details map[string]string
}

func (e *Error) Error() string { return e.Message }

func NewError(status int, message string) *Error {
	return &Error{Status: status, Message: message}
}

type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("writing response", "error", err)
	}
}

// WriteError renders err as JSON. Anything that is not an *Error is treated as
// an internal failure: it is logged and hidden from the client.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var httpErr *Error
	if !errors.As(err, &httpErr) {
		slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
		httpErr = NewError(http.StatusInternalServerError, "Internal server error")
	}
	WriteJSON(w, httpErr.Status, errorBody{Error: errorPayload{
		Message: httpErr.Message,
		Details: httpErr.Details,
	}})
}

// DecodeJSON reads a size-limited JSON body into dst.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		return NewError(http.StatusBadRequest, "Invalid JSON body")
	}
	return nil
}

// Handler is an http.HandlerFunc that can return an error.
type Handler func(w http.ResponseWriter, r *http.Request) error

func (h Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := h(w, r); err != nil {
		WriteError(w, r, err)
	}
}
