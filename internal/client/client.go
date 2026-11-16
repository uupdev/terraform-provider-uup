// Package client talks to the You Up? public API, `/v1`.
//
// It knows the API's envelope (`{ data, next }` lists, `{ error: { code,
// message } }` refusals) and its rate limits, and nothing about Terraform.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultBaseURL is where `/v1` lives.
const DefaultBaseURL = "https://api.uup.dev/v1"

// pageMax is the largest page `/v1` answers.
const pageMax = 200

// retries is how many times a request refused with 429 is sent again.
const retries = 4

// Client calls `/v1` with one API token.
type Client struct {
	baseURL   string
	token     string
	userAgent string
	http      *http.Client
}

// New returns a Client for the API at baseURL, sending token.
func New(baseURL, token, userAgent string) *Client {
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		token:     token,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 60 * time.Second},
	}
}

// Error is a refusal from the API.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

func (e *Error) Error() string {
	text := fmt.Sprintf("%s (HTTP %d, %s)", e.Message, e.Status, e.Code)
	if e.Field != "" {
		text += ", field " + e.Field
	}
	return text
}

// IsNotFound says whether err is the API's 404.
func IsNotFound(err error) bool {
	var refused *Error
	return errors.As(err, &refused) && refused.Status == http.StatusNotFound
}

// Do sends one request. body, when not nil, goes as JSON; out, when not nil,
// takes the JSON answer. A 429 waits for `Retry-After` and tries again.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encoding the request: %w", err)
		}
		payload = encoded
	}

	target := c.baseURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if payload != nil {
			reader = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, reader)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		res, err := c.http.Do(req)
		if err != nil {
			return err
		}
		answer, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return err
		}

		if res.StatusCode == http.StatusTooManyRequests && attempt < retries {
			if err := wait(ctx, retryAfter(res.Header.Get("Retry-After"))); err != nil {
				return err
			}
			continue
		}

		if res.StatusCode >= 400 {
			return refusal(res.StatusCode, answer)
		}

		if out != nil && len(answer) > 0 {
			if err := json.Unmarshal(answer, out); err != nil {
				return fmt.Errorf("reading the answer to %s %s: %w", method, path, err)
			}
		}
		return nil
	}
}

func refusal(status int, answer []byte) error {
	var envelope struct {
		Error *Error `json:"error"`
	}
	if json.Unmarshal(answer, &envelope) == nil && envelope.Error != nil {
		envelope.Error.Status = status
		return envelope.Error
	}
	return &Error{Status: status, Code: "unknown", Message: strings.TrimSpace(string(answer))}
}

func retryAfter(header string) time.Duration {
	seconds, err := strconv.Atoi(header)
	if err != nil || seconds < 1 {
		return 5 * time.Second
	}
	return time.Duration(seconds) * time.Second
}

func wait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// page is one page of a list.
type page[T any] struct {
	Data []T    `json:"data"`
	Next string `json:"next"`
}

// list reads every page of a list.
func list[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var all []T
	cursor := ""
	for {
		query := url.Values{"limit": {strconv.Itoa(pageMax)}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var one page[T]
		if err := c.Do(ctx, http.MethodGet, path, query, nil, &one); err != nil {
			return nil, err
		}
		all = append(all, one.Data...)
		if one.Next == "" {
			return all, nil
		}
		cursor = one.Next
	}
}

func esc(id string) string {
	return url.PathEscape(id)
}
