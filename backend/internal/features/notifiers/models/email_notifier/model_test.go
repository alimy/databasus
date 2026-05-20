package email_notifier

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func Test_SanitizeHeaderValue_StripsCRLFAndNUL(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"plain", "plain"},
		{"with\rcr", "withcr"},
		{"with\nlf", "withlf"},
		{"with\r\ncrlf", "withcrlf"},
		{"with\x00nul", "withnul"},
		{"a\r\nBcc: attacker@evil.com\r\n", "aBcc: attacker@evil.com"},
	}

	for _, c := range cases {
		got := sanitizeHeaderValue(c.input)
		if got != c.expected {
			t.Errorf("sanitizeHeaderValue(%q) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func Test_BuildEmailContent_DropsInjectedHeadersFromTargetEmail(t *testing.T) {
	notifier := &EmailNotifier{
		NotifierID:  uuid.New(),
		TargetEmail: "user@example.com\r\nBcc: attacker@evil.com",
		SMTPHost:    "smtp.example.com",
		SMTPPort:    587,
	}

	content := string(notifier.buildEmailContent("subject", "<p>body</p>", "from@example.com"))

	if strings.Contains(content, "\r\nBcc:") || strings.Contains(content, "\nBcc:") {
		t.Errorf("Bcc header line was injected via TargetEmail: %q", content)
	}

	if !strings.Contains(content, "To: user@example.comBcc: attacker@evil.com\r\n") {
		t.Errorf("expected sanitized To header without CRLF, got: %q", content)
	}
}

func Test_BuildEmailContent_DropsInjectedHeadersFromSMTPHost(t *testing.T) {
	notifier := &EmailNotifier{
		NotifierID:  uuid.New(),
		TargetEmail: "user@example.com",
		SMTPHost:    "smtp.example.com>\r\nX-Injected: 1",
		SMTPPort:    587,
	}

	content := string(notifier.buildEmailContent("subject", "<p>body</p>", "from@example.com"))

	if strings.Contains(content, "\r\nX-Injected:") || strings.Contains(content, "\nX-Injected:") {
		t.Errorf("injected header line leaked via SMTPHost: %q", content)
	}
}
