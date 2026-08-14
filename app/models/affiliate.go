package models

import (
	"time"
	"gorm.io/gorm"
)

type Affiliate struct {
	gorm.Model
	AffiliateID     string     `gorm:"uniqueIndex;size:20"`
	Name            string     `gorm:"size:100"`
	Email           string     `gorm:"size:255"`
	PasswordHash    string     `gorm:"size:255"`
	Rate            float64
	Active          bool       `gorm:"default:true"`
	ShopURL         string     `gorm:"size:255"`
	APIKey          string     `gorm:"uniqueIndex;size:128"`
	AuthorizedEmail string     `gorm:"size:255"`
	ExpiresAt       *time.Time `gorm:"type:timestamp"`
}
