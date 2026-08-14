package services

import (
	"context"
	"encoding/json"
	"path/filepath"
	"shopTemplate/app/db"
	"shopTemplate/app/i18n"
	"shopTemplate/app/models"
	"sync"
)

type I18nService struct {
	translations map[string]map[string]string
	mu           sync.RWMutex
}

var (
	i18nInstance *I18nService
	i18nOnce     sync.Once
)

func GetI18n() *I18nService {
	i18nOnce.Do(func() {
		i18nInstance = &I18nService{
			translations: make(map[string]map[string]string),
		}
		i18nInstance.loadTranslations()
	})
	return i18nInstance
}

func (s *I18nService) loadTranslations() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Load from Embedded Files (Default)
	entries, _ := i18n.Files.ReadDir(".")
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" {
			lang := entry.Name()[:2]
			data, err := i18n.Files.ReadFile(entry.Name())
			if err != nil {
				continue
			}
			var trans map[string]string
			if err := json.Unmarshal(data, &trans); err == nil {
				s.translations[lang] = trans
			}
		}
	}

	// 2. Load from Database (Overrides)
	var dbTrans []models.Translation
	if err := db.Get().Find(&dbTrans).Error; err == nil {
		for _, t := range dbTrans {
			if s.translations[t.Lang] == nil {
				s.translations[t.Lang] = make(map[string]string)
			}
			s.translations[t.Lang][t.Key] = t.Value
		}
	}
}

func (s *I18nService) T(ctx context.Context, key string) string {
	lang, ok := ctx.Value("lang").(string)
	if !ok || lang == "" {
		lang = "en"
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if trans, ok := s.translations[lang]; ok {
		if val, ok := trans[key]; ok {
			return val
		}
	}

	// Fallback to English
	if trans, ok := s.translations["en"]; ok {
		if val, ok := trans[key]; ok {
			return val
		}
	}

	return key
}
