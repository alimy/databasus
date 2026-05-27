package telegram_notifier

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"databasus-backend/internal/util/encryption"
)

type TelegramNotifier struct {
	NotifierID         uuid.UUID `json:"notifierId"         gorm:"primaryKey;column:notifier_id"`
	BotToken           string    `json:"botToken"           gorm:"not null;column:bot_token"`
	TargetChatID       string    `json:"targetChatId"       gorm:"not null;column:target_chat_id"`
	ThreadID           *int64    `json:"threadId"           gorm:"column:thread_id"`
	IsHTTPProxyEnabled bool      `json:"isHttpProxyEnabled" gorm:"column:is_http_proxy_enabled;type:boolean;not null;default:false"`
	HTTPProxyURL       string    `json:"httpProxyUrl"       gorm:"column:http_proxy_url;type:text"`
	HasHTTPProxyURL    bool      `json:"hasHttpProxyUrl"    gorm:"-"`
}

func (t *TelegramNotifier) TableName() string {
	return "telegram_notifiers"
}

func (t *TelegramNotifier) Validate(encryptor encryption.FieldEncryptor) error {
	if t.BotToken == "" {
		return errors.New("bot token is required")
	}

	if t.TargetChatID == "" {
		return errors.New("target chat ID is required")
	}

	if t.IsHTTPProxyEnabled {
		if t.HTTPProxyURL == "" {
			return errors.New("HTTP proxy URL is required")
		}

		proxyURL, err := encryptor.Decrypt(t.HTTPProxyURL)
		if err != nil {
			return fmt.Errorf("failed to decrypt HTTP proxy URL: %w", err)
		}

		parsedProxyURL, err := url.Parse(proxyURL)
		if err != nil || parsedProxyURL.Scheme != "http" || parsedProxyURL.Host == "" {
			return errors.New("HTTP proxy URL must be a valid http:// URL")
		}
	}

	return nil
}

func (t *TelegramNotifier) Send(
	encryptor encryption.FieldEncryptor,
	logger *slog.Logger,
	heading string,
	message string,
) error {
	botToken, err := encryptor.Decrypt(t.BotToken)
	if err != nil {
		return fmt.Errorf("failed to decrypt bot token: %w", err)
	}

	fullMessage := heading
	if message != "" {
		fullMessage = fmt.Sprintf("%s\n\n%s", heading, message)
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)

	data := url.Values{}
	data.Set("chat_id", t.TargetChatID)
	data.Set("text", fullMessage)
	data.Set("parse_mode", "HTML")

	if t.ThreadID != nil && *t.ThreadID != 0 {
		data.Set("message_thread_id", strconv.FormatInt(*t.ThreadID, 10))
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client, err := t.buildHTTPClient(encryptor)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf(
			"telegram API returned non-OK status: %s. Error: %s",
			resp.Status,
			string(bodyBytes),
		)
	}

	return nil
}

func (t *TelegramNotifier) buildHTTPClient(
	encryptor encryption.FieldEncryptor,
) (*http.Client, error) {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	if !t.IsHTTPProxyEnabled {
		return client, nil
	}

	proxyURL, err := encryptor.Decrypt(t.HTTPProxyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt HTTP proxy URL: %w", err)
	}

	parsedProxyURL, err := url.Parse(proxyURL)
	if err != nil || parsedProxyURL.Scheme != "http" || parsedProxyURL.Host == "" {
		return nil, errors.New("HTTP proxy URL must be a valid http:// URL")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(parsedProxyURL)
	client.Transport = transport

	return client, nil
}

func (t *TelegramNotifier) HideSensitiveData() {
	t.BotToken = ""
	t.HasHTTPProxyURL = t.IsHTTPProxyEnabled && t.HTTPProxyURL != ""
	t.HTTPProxyURL = ""
}

func (t *TelegramNotifier) Update(incoming *TelegramNotifier) {
	t.TargetChatID = incoming.TargetChatID
	t.ThreadID = incoming.ThreadID
	t.IsHTTPProxyEnabled = incoming.IsHTTPProxyEnabled

	if !incoming.IsHTTPProxyEnabled {
		t.HTTPProxyURL = ""
	} else if incoming.HTTPProxyURL != "" {
		t.HTTPProxyURL = incoming.HTTPProxyURL
	}

	if incoming.BotToken != "" {
		t.BotToken = incoming.BotToken
	}
}

func (t *TelegramNotifier) EncryptSensitiveData(encryptor encryption.FieldEncryptor) error {
	if t.BotToken != "" {
		encrypted, err := encryptor.Encrypt(t.BotToken)
		if err != nil {
			return fmt.Errorf("failed to encrypt bot token: %w", err)
		}
		t.BotToken = encrypted
	}

	if !t.IsHTTPProxyEnabled {
		t.HTTPProxyURL = ""
	} else if t.HTTPProxyURL != "" {
		encrypted, err := encryptor.Encrypt(t.HTTPProxyURL)
		if err != nil {
			return fmt.Errorf("failed to encrypt HTTP proxy URL: %w", err)
		}
		t.HTTPProxyURL = encrypted
	}

	return nil
}
