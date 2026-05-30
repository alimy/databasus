package mailpit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"databasus-backend/internal/config"
)

const requestTimeout = 5 * time.Second

type Message struct {
	ID      string    `json:"ID"`
	From    Address   `json:"From"`
	To      []Address `json:"To"`
	Subject string    `json:"Subject"`
	Snippet string    `json:"Snippet"`
}

type Address struct {
	Name    string `json:"Name"`
	Address string `json:"Address"`
}

type messagesResponse struct {
	Messages []Message `json:"messages"`
}

func baseURL() (string, error) {
	port := config.GetEnv().TestMailpitHttpPort
	if port == "" {
		return "", errors.New("TEST_MAILPIT_HTTP_PORT is not configured")
	}

	host := config.GetEnv().TestLocalhost
	if host == "" {
		host = "127.0.0.1"
	}

	return fmt.Sprintf("http://%s:%s", host, port), nil
}

func FetchMessages() ([]Message, error) {
	base, err := baseURL()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v1/messages", nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mailpit fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mailpit fetch returned %s: %s", resp.Status, string(body))
	}

	var payload messagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("mailpit decode: %w", err)
	}

	return payload.Messages, nil
}

func Clear() error {
	base, err := baseURL()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, base+"/api/v1/messages", nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("mailpit clear: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mailpit clear returned %s: %s", resp.Status, string(body))
	}

	return nil
}
