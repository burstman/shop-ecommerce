package models

import (
	"time"

	"gorm.io/gorm"
)

type ChatSession struct {
	gorm.Model
	Identifier   string `gorm:"uniqueIndex"` // A UUID stored in client's local storage/cookie
	CustomerName string
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
