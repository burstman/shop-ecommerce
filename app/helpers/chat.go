package helpers

import (
	"log/slog"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

// CountUnreadChatMessages returns how many client-sent chat messages have not
// been read by an admin yet.
func CountUnreadChatMessages() int64 {
	var count int64
	if err := db.Get().Model(&models.ChatMessage{}).Where("sender = ? AND is_read = ?", "client", false).Count(&count).Error; err != nil {
		slog.Error("failed to count unread chat messages", "err", err)
	}
	return count
}