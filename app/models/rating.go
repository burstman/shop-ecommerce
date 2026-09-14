package models

import "gorm.io/gorm"

type Rating struct {
	gorm.Model
	OrderID       uint  `gorm:"index"`
	Order         Order `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	DeliveryStars int   `gorm:"not null"`
	ProductStars  int   `gorm:"not null"`
	Comment       string
	AffiliateID   *uint `gorm:"index"`
}
