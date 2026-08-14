package models

import "gorm.io/gorm"

type Order struct {
	gorm.Model
	FirstName string
	LastName  string
	Email     string
	Address   string
	City      string
	Phone     string
	Total              Currency `gorm:"type:numeric(12,2)"`
	PlatformCommission Currency `gorm:"type:numeric(12,2)"`
	CommissionStatus   string   `gorm:"default:pending"` // pending, paid, cancelled
	Status             string   // pending, completed, cancelled
	IsTest             bool     `gorm:"default:false"`
	AffiliateID        *uint    `gorm:"index"`
	Items              []OrderItem
}

type OrderItem struct {
	gorm.Model
	OrderID      uint
	ProductID    uint
	Product      Product
	ProductName  string
	ProductImage string
	Quantity     int
	Price        Currency `gorm:"type:numeric(12,2)"`
}
