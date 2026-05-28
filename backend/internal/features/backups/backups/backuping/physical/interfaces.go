package backuping_physical

import (
	"databasus-backend/internal/features/notifiers"
)

// NotificationSender mirrors the logical-side seam so the backuper can be
// constructed against either the real notifiers service or a test stub.
type NotificationSender interface {
	SendNotification(notifier *notifiers.Notifier, title, message string)
}
