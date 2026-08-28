package models

import (
	"time"

	"gorm.io/gorm"
)

type ChatSession struct {
	gorm.Model
	Identifier   string `gorm:"uniqueIndex"` // A UUID stored in client's local storage/cookie, or "whatsapp:216XXXXXXXX"
	CustomerName string
	Channel      string `gorm:"default:web"` // "web" or "whatsapp"
	Phone        string `gorm:"size:20"`     // WhatsApp phone (only for whatsapp channel)
	IsActive     bool          `gorm:"default:true"`
	IsBanned     bool          `gorm:"default:false"`
	Messages     []ChatMessage `gorm:"foreignKey:ChatSessionID"`
}

type ChatMessage struct {
	ID            uint `gorm:"primaryKey"`
	ChatSessionID uint
	Sender        string // "client" or "admin"
	Content       string `gorm:"type:text"`
	CreatedAt     time.Time
	IsRead        bool `gorm:"default:false"`
}
