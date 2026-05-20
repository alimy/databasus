package verification_runs

import (
	"databasus-backend/internal/features/notifiers"
)

type NotificationSender interface {
	SendNotification(notifier *notifiers.Notifier, title, message string)
}
