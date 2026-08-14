package models

import "gorm.io/gorm"

type Translation struct {
	gorm.Model
	Lang  string `gorm:"uniqueIndex:idx_lang_key"`
	Key   string `gorm:"uniqueIndex:idx_lang_key"`
	Value string
}
