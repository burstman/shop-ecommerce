package models

import (
	"gorm.io/gorm"
)

type Category struct {
	gorm.Model
	Name          string
	Slug          *string
	ParentID      *uint
	SubCategories []Category `gorm:"foreignkey:ParentID"`
	Position      int
	IsLocked      bool `gorm:"default:false"`
}
