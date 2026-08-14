package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"strings"
	"time"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

const configFilePath = "app/config/config.json"

type contextKey string

const (
	affiliateKey  contextKey = "affiliate"
	affiliateIDKey contextKey = "affiliate_id"
)

func WithAffiliate(ctx context.Context, affiliate *models.Affiliate) context.Context {
	ctx = context.WithValue(ctx, affiliateKey, affiliate)
	ctx = context.WithValue(ctx, affiliateIDKey, affiliate.AffiliateID)
	return ctx
}

func AffiliateFromContext(ctx context.Context) *models.Affiliate {
	v, _ := ctx.Value(affiliateKey).(*models.Affiliate)
	return v
}

func AffiliateIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(affiliateIDKey).(string)
	return v
}

var (
	cfg     *Config
	once    sync.Once
	mu      sync.Mutex
	affMu   sync.RWMutex
	affMap  = map[string]uint{}   // shop_url -> affiliate ID
	cfgMu   sync.RWMutex
	cfgCache = map[string]*Config{} // affiliate_id -> config
)

type Config struct {
	Site              SiteConfig          `json:"site"`
	Notification      NotificationConfig  `json:"notification"`
	FacebookPixel     FacebookPixelConfig `json:"facebook_pixel"`
	Hero              HeroConfig          `json:"hero"`
	Sections          []SectionConfig     `json:"sections"`
	Theme             ThemeConfig         `json:"theme"`
	StorefrontSidebar []MenuItem          `json:"storefront_sidebar"`
	Footer            FooterConfig        `json:"footer"`
	Payment           PaymentConfig       `json:"payment"`
	Chat              ChatConfig          `json:"chat"`
}

type NotificationConfig struct {
	AdminEmailRecipient string `json:"admin_email_recipient"`
	TelegramBotToken    string `json:"telegram_bot_token"`
	TelegramChatID      string `json:"telegram_chat_id"`
}

type FacebookPixelConfig struct {
	PixelID            string `json:"pixel_id"`
	TrackPurchaseValue bool   `json:"track_purchase_value"`
	AccessToken        string `json:"access_token"`
	DomainVerification string `json:"domain_verification"`
	TestEventCode      string `json:"test_event_code"`
}



type SiteConfig struct {
	Name          string          `json:"name"`
	SupportEmail  string          `json:"support_email"`
	NameBgColor   string          `json:"name_bg_color"`
	NameTextColor string          `json:"name_text_color"`
	Logo          string          `json:"logo"`
	Currency      string          `json:"currency"`
	AffiliateID   string          `json:"affiliate_id"`
	ShowQuickView bool            `json:"show_quick_view"`
	ShowOrderNow  bool            `json:"show_order_now"`
	ShowAddToCart bool            `json:"show_add_to_cart"`
	Bundles       []models.Bundle `json:"bundles"`
}

type HeroConfig struct {
	Enabled    bool        `json:"enabled"`
	Title      string      `json:"title"`
	Subtitle   string      `json:"subtitle"`
	ButtonText string      `json:"button_text"`
	ButtonLink string      `json:"button_link"`
	Slides     []HeroSlide `json:"slides"`
}

type HeroSlide struct {
	Image      string `json:"image"`
	Title      string `json:"title,omitempty"`
	Subtitle   string `json:"subtitle,omitempty"`
	ButtonText string `json:"button_text,omitempty"`
	ButtonLink string `json:"button_link,omitempty"`
	ProductID  *uint  `json:"product_id,omitempty"`
}

type CategorySectionItem struct {
	Title      string `json:"title"`
	CategoryID string `json:"category_id"`
	Image      string `json:"image"`
}

type SectionConfig struct {
	Type           string                `json:"type"`
	Title          string                `json:"title"`
	Limit          int                   `json:"limit"`
	Enabled        bool                  `json:"enabled"`
	ProductIDs     []string              `json:"product_ids,omitempty"`
	CategoryID     string                `json:"category_id,omitempty"`
	Image          string                `json:"image,omitempty"`
	CategoryItems  []CategorySectionItem `json:"category_items,omitempty"`
	TitleBgColor   string                `json:"title_bg_color"`
	TitleTextColor string                `json:"title_text_color"`
}

type ThemeConfig struct {
	PrimaryColor             string `json:"primary_color"`
	SecondaryColor           string `json:"secondary_color"`
	HeaderBgColor            string `json:"header_bg_color"`
	HeaderBgOpacity          int    `json:"header_bg_opacity"`
	PageBgColor              string `json:"page_bg_color"`
	FooterBgColor            string `json:"footer_bg_color"`
	ContentBgGradientEnabled bool   `json:"content_bg_gradient_enabled"`
	ContentBgGradientStart   string `json:"content_bg_gradient_start"`
	ContentBgGradientEnd     string `json:"content_bg_gradient_end"`
	ContentBgGradientAngle   int    `json:"content_bg_gradient_angle"`
	ContentBgColorEnabled    bool   `json:"content_bg_color_enabled"`
	ContentBgColor           string `json:"content_bg_color"`
}

type FooterConfig struct {
	Address     string       `json:"address"`
	Phone       string       `json:"phone"`
	Email       string       `json:"email"`
	MapEmbedURL string       `json:"map_embed_url"`
	Copyright   string       `json:"copyright"`
	SocialLinks []SocialLink `json:"social_links"`
	LinkColumns []LinkColumn `json:"link_columns"`
	Hours       []StoreHour  `json:"hours"`
}

type StoreHour struct {
	Day       string `json:"day"`
	IsClosed  bool   `json:"is_closed"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
}

type ChatConfig struct {
	PrimaryColor      string `json:"primary_color"`
	HeaderTextColor   string `json:"header_text_color"`
	ClientBubbleColor string `json:"client_bubble_color"`
	ClientTextColor   string `json:"client_text_color"`
	AdminBubbleColor  string `json:"admin_bubble_color"`
	AdminTextColor    string `json:"admin_text_color"`
	EnablePopup       bool   `json:"enable_popup"`
	PopupTimeout      int    `json:"popup_timeout"`
}

type PaymentConfig struct {
	EnableCOD       bool   `json:"enable_cod"`
	EnableFlouci    bool   `json:"enable_flouci"`
	FlouciPublicKey string `json:"flouci_public_key"`
	FlouciAppToken  string `json:"flouci_app_token"`
}

type SocialLink struct {
	Platform string `json:"platform"`
	URL      string `json:"url"`
	Icon     string `json:"icon"`
}

type LinkColumn struct {
	Title string     `json:"title"`
	Links []MenuItem `json:"links"`
}

type MenuItem struct {
	Title string `json:"title"`
	Link  string `json:"link"`
	Icon  string `json:"icon,omitempty"`
	Badge string `json:"badge,omitempty"`
}

type SidebarGroup struct {
	Label string     `json:"label"`
	Items []MenuItem `json:"items"`
}

func GetAdminSidebarGroups() []SidebarGroup {
	return []SidebarGroup{
		{
			Label: "Storefront",
			Items: []MenuItem{
				{Title: "Hero Section", Link: "/admin/hero", Icon: "image"},
				{Title: "Homepage Sections", Link: "/admin/sections", Icon: "layout-grid"},
				{Title: "Theme", Link: "/admin/theme", Icon: "palette"},
				{Title: "Notifications", Link: "/admin/notifications", Icon: "bell"},
			},
		},
		{
			Label: "Catalog",
			Items: []MenuItem{
				{Title: "Categories", Link: "/admin/categories", Icon: "folder-tree"},
				{Title: "Products", Link: "/admin/products", Icon: "shopping-bag"},
			},
		},
		{
			Label: "Operations",
			Items: []MenuItem{
				{Title: "Orders", Link: "/admin/orders", Icon: "clipboard-list"},
				{Title: "Payment Methods", Link: "/admin/payment", Icon: "credit-card"},
			},
		},
		{
			Label: "Engagement",
			Items: []MenuItem{
				{Title: "Facebook Pixel", Link: "/admin/facebook_pixel", Icon: "facebook"},
				{Title: "Chat Settings", Link: "/admin/chat_settings", Icon: "message-square"},
				{Title: "Social Links", Link: "/admin/social_links", Icon: "share-2"},
			},
		},
		{
			Label: "Settings",
			Items: []MenuItem{
				{Title: "Site Settings", Link: "/admin/site", Icon: "settings"},
			},
		},
	}
}

// kept for any code still referencing the flat list
func GetAdminSidebar() []MenuItem {
	var flat []MenuItem
	for _, g := range GetAdminSidebarGroups() {
		flat = append(flat, g.Items...)
	}
	return flat
}

func defaultConfig() *Config {
	return &Config{
		Site: SiteConfig{
			Name:          "BEST SHOP",
			SupportEmail:  "support@bestshop.com",
			NameBgColor:   "#2E7D32",
			NameTextColor: "#FFFFFF",
			Logo:          "",
			Currency:      "TND",
			ShowQuickView: true,
			ShowOrderNow:  true,
			ShowAddToCart: true,
			Bundles: []models.Bundle{
				{Quantity: 2, DiscountPercentage: 10},
				{Quantity: 3, DiscountPercentage: 15},
			},
		},
		Notification: NotificationConfig{
			AdminEmailRecipient: "admin@bestshop.com",
			TelegramBotToken:    "",
			TelegramChatID:      "",
		},
		FacebookPixel: FacebookPixelConfig{
			PixelID:            "",
			TrackPurchaseValue: true,
			AccessToken:        "",
			DomainVerification: "",
			TestEventCode:      "",
		},
		Hero: HeroConfig{
			Enabled:    true,
			Title:      "Nature's Masterpiece",
			Subtitle:   "Discover our indoor plants",
			ButtonText: "Shop Now",
			ButtonLink: "/",
			Slides:     []HeroSlide{},
		},
		Sections: []SectionConfig{
			{
				Type:           "featured_products",
				Title:          "Featured Collection",
				Limit:          4,
				Enabled:        true,
				ProductIDs:     []string{},
				TitleBgColor:   "#2E7D32",
				TitleTextColor: "#FFFFFF",
			},
			{
				Type:           "best_sellers",
				Title:          "Best Sellers",
				Limit:          4,
				Enabled:        true,
				ProductIDs:     []string{},
				TitleBgColor:   "#2E7D32",
				TitleTextColor: "#FFFFFF",
			},
			{
				Type:           "new_arrivals",
				Title:          "New Arrivals",
				Limit:          4,
				Enabled:        true,
				ProductIDs:     []string{},
				TitleBgColor:   "#2E7D32",
				TitleTextColor: "#FFFFFF",
			},
		},
		Theme: ThemeConfig{
			PrimaryColor:             "#2E7D32",
			SecondaryColor:           "#F5F5F5",
			HeaderBgColor:            "#FFFFFF",
			HeaderBgOpacity:          100,
			PageBgColor:              "#FFFFFF",
			FooterBgColor:            "#F9FAFB",
			ContentBgGradientEnabled: false,
			ContentBgGradientStart:   "#FFFFFF",
			ContentBgGradientEnd:     "#F3F4F6",
			ContentBgGradientAngle:   180,
			ContentBgColorEnabled:    false,
			ContentBgColor:           "#FFFFFF",
		},
		StorefrontSidebar: []MenuItem{},
		Footer: FooterConfig{
			Address:     "123 Rue de la Liberté, Tunis, Tunisie",
			Phone:       "+216 XX XXX XXX",
			Email:       "contact@bestshop.com",
			MapEmbedURL: "https://www.google.com/maps/embed?pb=!1m18!1m12!1m3!1d3192.0!2d10.0!3d36.0!2m3!1f0!2f0!3f0!3m2!1i1024!2i768!4f13.1!3m3!1m2!1s0x0%3A0x0!2zMzbCsDAwJzAwLjAiTiAxMMKwMDAnMDAuMCJF!5e0!3m2!1sfr!2stn!4v1",
			Copyright:   fmt.Sprintf("© %d BEST SHOP. All rights reserved.", time.Now().Year()),
			SocialLinks: []SocialLink{
				{Platform: "Facebook", URL: "https://facebook.com", Icon: "facebook"},
				{Platform: "Instagram", URL: "https://instagram.com", Icon: "instagram"},
				{Platform: "TikTok", URL: "https://tiktok.com", Icon: "tiktok"},
				{Platform: "WhatsApp", URL: "https://wa.me/21600000000", Icon: "whatsapp"},
			},
			LinkColumns: []LinkColumn{
				{
					Title: "Informations",
					Links: []MenuItem{
						{Title: "Livraison", Link: "/content/livraison"},
						{Title: "À Propos De Nous", Link: "/content/a-propos"},
						{Title: "Magasin", Link: "/magasins"},
					},
				},
			},
			Hours: []StoreHour{
				{Day: "Lun.", IsClosed: true},
				{Day: "Mar.", OpenTime: "08h00", CloseTime: "17h00"},
				{Day: "Mer.", OpenTime: "08h00", CloseTime: "17h00"},
				{Day: "Jeu.", OpenTime: "08h00", CloseTime: "17h00"},
				{Day: "Ven.", OpenTime: "08h00", CloseTime: "17h00"},
				{Day: "Sam.", OpenTime: "08h00", CloseTime: "17h00"},
				{Day: "Dim.", OpenTime: "08h00", CloseTime: "17h00"},
			},
		},
		Chat: ChatConfig{
			PrimaryColor:      "#2E7D32",
			HeaderTextColor:   "#FFFFFF",
			ClientBubbleColor: "#2E7D32",
			ClientTextColor:   "#FFFFFF",
			AdminBubbleColor:  "#FFFFFF",
			AdminTextColor:    "#1F2937",
			EnablePopup:       true,
			PopupTimeout:      8,
		},
		Payment: PaymentConfig{
			EnableCOD:       true,
			EnableFlouci:    false,
			FlouciPublicKey: "",
			FlouciAppToken:  "",
		},
	}
}

// Get loads the configuration from the global singleton.
// It is primarily used for background tasks (events, setup) that lack an HTTP request context.
// For request-scoped config, use FromContext(ctx) instead.
func Get() *Config {
	once.Do(func() {
		// 1. Try to load from Postgres settings table
		var setting models.Setting
		err := db.Get().Where("key = ?", "app_config").First(&setting).Error
		if err == nil && setting.Value != "" {
			if err := json.Unmarshal([]byte(setting.Value), &cfg); err == nil {
				slog.Info("configuration loaded from postgres")
				backfill(cfg)
				return
			}
		}

		// 2. Migration Fallback: Try to read from the legacy JSON file
		if file, err := os.ReadFile(configFilePath); err == nil {
			if err := json.Unmarshal(file, &cfg); err == nil {
				slog.Info("configuration migrated from local file to postgres")
				backfill(cfg)
				save(cfg) // Persist legacy file to DB
				return
			}
		}

		// 3. Final Fallback: Use hardcoded defaults
		slog.Warn("no configuration found in DB or file, using hardcoded defaults")
		cfg = defaultConfig()
		save(cfg)
	})
	return cfg
}

// LookupAffiliateByShopURL finds an affiliate whose shop_url matches the request host.
// It tries the raw host first, then with https:// and http:// prefixes.
func LookupAffiliateByShopURL(host string) *models.Affiliate {
	var aff models.Affiliate

	if host == "" {
		return nil
	}

	if err := db.Get().Where("shop_url = ?", host).First(&aff).Error; err == nil {
		return &aff
	}

	if err := db.Get().Where("shop_url = ?", "https://"+host).First(&aff).Error; err == nil {
		return &aff
	}

	if err := db.Get().Where("shop_url = ?", "http://"+host).First(&aff).Error; err == nil {
		return &aff
	}

	return nil
}

func configKey(affiliateID string) string {
	if affiliateID == "" {
		return "app_config"
	}
	return "app_config:" + affiliateID
}

// LoadByAffiliateID loads the configuration for a specific affiliate.
// It caches the config in memory keyed by affiliate ID.
func LoadByAffiliateID(affiliateID string) *Config {
	if affiliateID == "" {
		return Get()
	}

	cfgMu.RLock()
	if c, ok := cfgCache[affiliateID]; ok {
		cfgMu.RUnlock()
		return c
	}
	cfgMu.RUnlock()

	key := configKey(affiliateID)
	var setting models.Setting
	err := db.Get().Where("key = ?", key).First(&setting).Error
	if err != nil || setting.Value == "" {
		// Fall back to the legacy global config if it exists
		global := Get()
		c := *global // shallow copy (OK since Config has no pointer fields beyond slices)
		c.Site.AffiliateID = affiliateID
		backfill(&c)

		if saveErr := saveWithKey(key, &c); saveErr != nil {
			slog.Error("failed to save config for affiliate", "affiliate_id", affiliateID, "err", saveErr)
		}

		cfgMu.Lock()
		cfgCache[affiliateID] = &c
		cfgMu.Unlock()
		return &c
	}

	var c Config
	if err := json.Unmarshal([]byte(setting.Value), &c); err != nil {
		slog.Error("failed to unmarshal config for affiliate", "affiliate_id", affiliateID, "err", err)
		return Get()
	}
	backfill(&c)

	cfgMu.Lock()
	cfgCache[affiliateID] = &c
	cfgMu.Unlock()
	return &c
}

// FromContext returns the config for the current shop from the request context.
// Falls back to Get() if no shop context is available.
func FromContext(ctx context.Context) *Config {
	if affID := AffiliateIDFromContext(ctx); affID != "" {
		return LoadByAffiliateID(affID)
	}
	return Get()
}

func backfill(c *Config) {
	if len(c.StorefrontSidebar) == 0 {
		c.StorefrontSidebar = defaultConfig().StorefrontSidebar
	}
	if c.Footer.Copyright == "" || strings.Contains(c.Footer.Copyright, "Botanica") {
		c.Footer.Copyright = fmt.Sprintf("© %d %s. All rights reserved.", time.Now().Year(), c.Site.Name)
	}
	if c.Site.AffiliateID == "" {
		c.Site.AffiliateID = "AFF-001"
	}
	// Ensure Hours have defaults
	if len(c.Footer.Hours) == 0 {
		c.Footer.Hours = defaultConfig().Footer.Hours
	}
	// Remove the old "Shop" default column, keep only Informations + any user custom columns
	var cleaned []LinkColumn
	for _, col := range c.Footer.LinkColumns {
		if col.Title != "Shop" {
			cleaned = append(cleaned, col)
		}
	}
	c.Footer.LinkColumns = cleaned
	// Ensure the Informations link column exists
	hasInfo := false
	for _, col := range c.Footer.LinkColumns {
		if col.Title == "Informations" {
			hasInfo = true
			break
		}
	}
	if !hasInfo {
		def := defaultConfig()
		infoCol := def.Footer.LinkColumns[0]
		c.Footer.LinkColumns = append([]LinkColumn{infoCol}, c.Footer.LinkColumns...)
	}
}

// save writes the config to the legacy "app_config" key.
func save(c *Config) error {
	return saveWithKey("app_config", c)
}

func saveWithKey(key string, c *Config) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return db.Get().Exec(`
		INSERT INTO settings (key, value, created_at, updated_at) 
		VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP) 
		ON CONFLICT (key) 
		DO UPDATE SET value = EXCLUDED.value, updated_at = EXCLUDED.updated_at`,
		key, string(data)).Error
}

// Save writes the configuration for the current shop and updates in-memory caches.
func Save(newCfg *Config) error {
	mu.Lock()
	defer mu.Unlock()

	affID := newCfg.Site.AffiliateID
	key := configKey(affID)
	if err := saveWithKey(key, newCfg); err != nil {
		return err
	}
	cfgMu.Lock()
	cfgCache[affID] = newCfg
	cfgMu.Unlock()
	cfg = newCfg
	return nil
}
