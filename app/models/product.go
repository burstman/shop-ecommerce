package models

import "gorm.io/gorm"

type Bundle struct {
	Quantity           int `json:"quantity"`
	DiscountPercentage int `json:"discount_percentage"`
}

type Product struct {
	gorm.Model
	Name           string
	Price          Currency `gorm:"type:numeric(12,2)"`
	PromotionPrice Currency `gorm:"type:numeric(12,2)"`
	Description    string
	Stock          int
	Image          string
	Images         []string   `gorm:"type:jsonb;serializer:json"`
	Categories     []Category `gorm:"many2many:product_categories;"`
	Category       string     `gorm:"-"` // Deprecated: Kept for backward compatibility with views
	Bundles        []Bundle   `gorm:"type:jsonb;serializer:json"`
	BundlesEnabled bool       `gorm:"default:true"`
}
