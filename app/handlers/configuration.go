package handlers

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
	"strconv"
	"strings"
	"regexp"

	"shopTemplate/app/views/configuration"

	"github.com/a-h/templ"
	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
)

func HandleAdminSettings(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	section := chi.URLParam(kit.Request, "section")
	if section == "" {
		section = "site"
	}

	cfg, products, categories, err := getAdminConfigData(kit.Request, section)
	if err != nil {
		return err
	}

	activePath := "/admin/" + section
	sidebar := config.GetAdminSidebarGroups()
	var content templ.Component
	switch section {
	case "notifications":
		content = configuration.Notifications(cfg)
	case "facebook_pixel":
		content = configuration.FacebookPixel(cfg)
	case "site", "hero", "sections", "theme", "payment", "storefront-sidebar", "social_links", "chat_settings":
		csrfToken := csrf.Token(kit.Request)
		content = configuration.Index(cfg, products, categories, section, csrfToken)
	default:
		return kit.Redirect(http.StatusSeeOther, "/admin/site") // Default to site settings if section is unknown
	}
	return RenderAdminWithLayout(kit, sidebar, activePath, content)
}

func getAdminConfigData(r *http.Request, section string) (*config.Config, []models.Product, []models.Category, error) {
	cfg := config.FromContext(r.Context())
	var products []models.Product
	var categories []models.Category

	if section == "sections" || section == "hero" {
		if err := db.Get().Order("name").Find(&products).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	if section == "sections" {
		if err := db.Get().Order("name").Find(&categories).Error; err != nil {
			return nil, nil, nil, err
		}
	}
	return cfg, products, categories, nil
}

func HandleAdminSettingsUpdate(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	section := chi.URLParam(kit.Request, "section")
	kit.Request.ParseMultipartForm(32 << 20) // Parse both multipart and standard form data

	cfg := config.FromContext(kit.Request.Context())
	aff := config.AffiliateFromContext(kit.Request.Context())
	affID := "default"
	if aff != nil {
		affID = aff.AffiliateID
	}

	switch section {
	case "site":
		cfg.Site.Name = kit.Request.FormValue("site_name")
		cfg.Site.SupportEmail = kit.Request.FormValue("support_email")
		cfg.Site.NameBgColor = kit.Request.FormValue("site_name_bg_color")
		cfg.Site.NameTextColor = kit.Request.FormValue("site_name_text_color")
		cfg.Site.Currency = kit.Request.FormValue("site_currency")
		cfg.Site.ShowQuickView = kit.Request.FormValue("show_quick_view") == "on"
		cfg.Site.ShowOrderNow = kit.Request.FormValue("show_order_now") == "on"
		cfg.Site.ShowAddToCart = kit.Request.FormValue("show_add_to_cart") == "on"
		cfg.Footer.Copyright = kit.Request.FormValue("footer_copyright")
		cfg.Footer.Address = kit.Request.FormValue("footer_address")
		cfg.Footer.Phone = kit.Request.FormValue("footer_phone")
		cfg.Footer.Email = kit.Request.FormValue("footer_email")
		cfg.Footer.MapEmbedURL = extractEmbedSrc(kit.Request.FormValue("footer_map_embed"))

		if countStr := kit.Request.FormValue("hours_count"); countStr != "" {
			count, _ := strconv.Atoi(countStr)
			var hours []config.StoreHour
			for j := 0; j < count; j++ {
				prefix := fmt.Sprintf("hour_%d_", j)
				hours = append(hours, config.StoreHour{
					Day:       kit.Request.FormValue(prefix + "day"),
					IsClosed:  kit.Request.FormValue(prefix+"closed") == "on",
					OpenTime:  kit.Request.FormValue(prefix + "open"),
					CloseTime: kit.Request.FormValue(prefix + "close"),
				})
			}
			cfg.Footer.Hours = hours
		}

		var bundles []models.Bundle
		if countStr := kit.Request.FormValue("bundles_count"); countStr != "" {
			count, _ := strconv.Atoi(countStr)
			for j := 0; j < count; j++ {
				prefix := fmt.Sprintf("bundle_%d_", j)
				if kit.Request.FormValue(prefix+"delete") == "on" {
					continue
				}
				qty, _ := strconv.Atoi(kit.Request.FormValue(prefix + "quantity"))
				discount, _ := strconv.Atoi(kit.Request.FormValue(prefix + "discount"))
				if qty > 0 {
					bundles = append(bundles, models.Bundle{
						Quantity:           qty,
						DiscountPercentage: discount,
					})
				}
			}
		}
		if kit.Request.FormValue("add_bundle") == "true" {
			bundles = append(bundles, models.Bundle{Quantity: 2, DiscountPercentage: 10})
		}
		cfg.Site.Bundles = bundles

		// Handle site logo upload
		if file, header, err := kit.Request.FormFile("site_logo"); err == nil {
			defer file.Close()
			imageURL, err := helpers.UploadImage(file, header, affID, "site", "logo")
			if err == nil {
				// Defensive check for existing logo to avoid errors on deletion
				if len(cfg.Site.Logo) > 1 && cfg.Site.Logo[0] == '/' {
					if _, err := os.Stat(cfg.Site.Logo[1:]); err == nil {
						os.Remove(cfg.Site.Logo[1:])
					}
				}
				cfg.Site.Logo = imageURL
			}
		}
	case "notifications":
		cfg.Notification.AdminEmailRecipient = kit.Request.FormValue("admin_email_recipient")
		cfg.Notification.TelegramBotToken = kit.Request.FormValue("telegram_bot_token")
		cfg.Notification.TelegramChatID = kit.Request.FormValue("telegram_chat_id")
	case "facebook_pixel":
		cfg.FacebookPixel.PixelID = kit.Request.FormValue("pixel_id")
		cfg.FacebookPixel.TrackPurchaseValue = kit.Request.FormValue("track_purchase_value") == "on"
		cfg.FacebookPixel.AccessToken = kit.Request.FormValue("access_token")
		cfg.FacebookPixel.DomainVerification = kit.Request.FormValue("domain_verification")
		cfg.FacebookPixel.TestEventCode = kit.Request.FormValue("test_event_code")
		slog.Info("facebook pixel settings updated",
			"pixel_id", cfg.FacebookPixel.PixelID,
			"has_access_token", cfg.FacebookPixel.AccessToken != "",
			"test_event_code", cfg.FacebookPixel.TestEventCode,
		)

	case "payment":
		cfg.Payment.EnableCOD = kit.Request.FormValue("enable_cod") == "on"
		cfg.Payment.EnableFlouci = kit.Request.FormValue("enable_flouci") == "on"
		cfg.Payment.FlouciAppToken = kit.Request.FormValue("flouci_app_token")

	case "social_links":
		for i := range cfg.Footer.SocialLinks {
			name := strings.ToLower(cfg.Footer.SocialLinks[i].Platform)
			cfg.Footer.SocialLinks[i].URL = kit.Request.FormValue("social_" + name + "_url")
		}

	case "chat_settings":
		cfg.Chat.PrimaryColor = kit.Request.FormValue("chat_primary_color")
		cfg.Chat.HeaderTextColor = kit.Request.FormValue("chat_header_text_color")
		cfg.Chat.ClientBubbleColor = kit.Request.FormValue("chat_client_bubble_color")
		cfg.Chat.ClientTextColor = kit.Request.FormValue("chat_client_text_color")
		cfg.Chat.AdminBubbleColor = kit.Request.FormValue("chat_admin_bubble_color")
		cfg.Chat.AdminTextColor = kit.Request.FormValue("chat_admin_text_color")
		cfg.Chat.EnablePopup = kit.Request.FormValue("chat_enable_popup") == "on"
		if timeout, err := strconv.Atoi(kit.Request.FormValue("chat_popup_timeout")); err == nil {
			cfg.Chat.PopupTimeout = timeout
		}

	case "hero":
		cfg.Hero.Enabled = kit.Request.FormValue("hero_enabled") == "on"
		cfg.Hero.Title = kit.Request.FormValue("hero_title")
		cfg.Hero.Subtitle = kit.Request.FormValue("hero_subtitle")
		cfg.Hero.ButtonText = kit.Request.FormValue("hero_button_text")

		// Process Existing Slides
		var updatedSlides []config.HeroSlide
		for i, slide := range cfg.Hero.Slides {
			prefix := fmt.Sprintf("slide_%d_", i)
			if kit.Request.FormValue(prefix+"delete") == "on" {
				if len(slide.Image) > 0 && slide.Image[0] == '/' {
					os.Remove(slide.Image[1:])
				}
				continue
			}
			slide.Title = kit.Request.FormValue(prefix + "title")
			slide.Subtitle = kit.Request.FormValue(prefix + "subtitle")
			slide.ButtonText = kit.Request.FormValue(prefix + "button_text")

			// Handle individual slide image upload from HeroSettings
			if file, header, err := kit.Request.FormFile(prefix + "image"); err == nil {
				defer file.Close()
				imageURL, err := helpers.UploadImage(file, header, affID, "carousel", "slide_"+fmt.Sprint(i))
				if err == nil {
					if len(slide.Image) > 0 && slide.Image[0] == '/' {
						os.Remove(slide.Image[1:])
					}
					slide.Image = imageURL
				}
			}

			productIDStr := kit.Request.FormValue(prefix + "product_id")
			if productIDStr != "" {
				productID, _ := strconv.Atoi(productIDStr)
				pid := uint(productID)
				slide.ProductID = &pid
			} else {
				slide.ProductID = nil
			}
			updatedSlides = append(updatedSlides, slide)
		}

		cfg.Hero.Slides = updatedSlides

		// Handle adding a new slide
		if kit.Request.FormValue("add_slide") == "true" {
			cfg.Hero.Slides = append(cfg.Hero.Slides, config.HeroSlide{
				Image:      "",
				Title:      "New Slide Title",
				Subtitle:   "New Slide Subtitle",
				ButtonText: "Shop Now",
				ButtonLink: "#", // Default link, will be overridden by ProductID if set
			})
		}
	case "sections":
		orderStr := kit.Request.FormValue("sections_order")
		var order []int
		if orderStr != "" {
			for _, part := range strings.Split(orderStr, ",") {
				if idx, err := strconv.Atoi(part); err == nil {
					order = append(order, idx)
				}
			}
		}

		// Fallback if JS didn't run or order data is missing
		if len(order) != len(cfg.Sections) {
			order = make([]int, len(cfg.Sections))
			for i := range cfg.Sections {
				order[i] = i
			}
		}

		newSections := make([]config.SectionConfig, len(cfg.Sections))
		for newIdx, oldIdx := range order {
			prefix := fmt.Sprintf("section_%d_", oldIdx)
			s := cfg.Sections[oldIdx]
			s.Enabled = kit.Request.FormValue(prefix+"enabled") == "on"
			s.Title = kit.Request.FormValue(prefix + "title")
			if limit, err := strconv.Atoi(kit.Request.FormValue(prefix + "limit")); err == nil {
				s.Limit = limit
			}
			s.TitleBgColor = kit.Request.FormValue(prefix + "title_bg_color")
			s.TitleTextColor = kit.Request.FormValue(prefix + "title_text_color")
			s.CategoryID = kit.Request.FormValue(prefix + "category_id")
			s.Type = kit.Request.FormValue(prefix + "type")

			// Handle multiple Category Items for "category_banner" type
			if s.Type == "category_banner" {
				var items []config.CategorySectionItem
				itemCount, _ := strconv.Atoi(kit.Request.FormValue(prefix + "item_count"))
				for j := 0; j < itemCount; j++ {
					itemPrefix := fmt.Sprintf("%sitem_%d_", prefix, j)
					if kit.Request.FormValue(itemPrefix+"delete") == "on" {
						continue
					}
					item := config.CategorySectionItem{
						Title:      kit.Request.FormValue(itemPrefix + "title"),
						CategoryID: kit.Request.FormValue(itemPrefix + "category_id"),
						Image:      kit.Request.FormValue(itemPrefix + "image_path"),
					}

					if file, header, err := kit.Request.FormFile(itemPrefix + "image"); err == nil {
						defer file.Close()
						imageURL, err := helpers.UploadImage(file, header, affID, "sections", fmt.Sprintf("section_%d_item_%d", oldIdx, j))
						if err == nil {
							item.Image = imageURL
						}
					}
					items = append(items, item)
				}

				if kit.Request.FormValue(prefix+"add_item") == "true" {
					items = append(items, config.CategorySectionItem{Title: "New Group"})
				}
				s.CategoryItems = items
			}

			// Handle section image upload
			if file, header, err := kit.Request.FormFile(prefix + "image"); err == nil {
				defer file.Close()
				imageURL, err := helpers.UploadImage(file, header, affID, "sections", fmt.Sprintf("section_%d", oldIdx))
				if err == nil {
					if len(s.Image) > 0 && s.Image[0] == '/' {
						os.Remove(s.Image[1:])
					}
					s.Image = imageURL
				} else {
					slog.Error("failed to upload section image", "err", err)
				}
			}

			s.ProductIDs = kit.Request.MultipartForm.Value[prefix+"product_ids"]
			newSections[newIdx] = s
		}
		cfg.Sections = newSections

	case "theme":
		cfg.Theme.PrimaryColor = kit.Request.FormValue("theme_primary_color")
		cfg.Theme.SecondaryColor = kit.Request.FormValue("theme_secondary_color")
		cfg.Theme.HeaderBgColor = kit.Request.FormValue("theme_header_bg_color")
		if opacity, err := strconv.Atoi(kit.Request.FormValue("theme_header_bg_opacity")); err == nil {
			cfg.Theme.HeaderBgOpacity = opacity
		}
		cfg.Theme.PageBgColor = kit.Request.FormValue("theme_page_bg_color")
		cfg.Theme.FooterBgColor = kit.Request.FormValue("theme_footer_bg_color")
		cfg.Theme.ContentBgColorEnabled = kit.Request.FormValue("theme_content_bg_color_enabled") == "on"
		cfg.Theme.ContentBgColor = kit.Request.FormValue("theme_content_bg_color")
		cfg.Theme.ContentBgGradientEnabled = kit.Request.FormValue("theme_content_bg_gradient_enabled") == "on"
		cfg.Theme.ContentBgGradientStart = kit.Request.FormValue("theme_content_bg_gradient_start")
		cfg.Theme.ContentBgGradientEnd = kit.Request.FormValue("theme_content_bg_gradient_end")
		if angle, err := strconv.Atoi(kit.Request.FormValue("theme_content_bg_gradient_angle")); err == nil {
			cfg.Theme.ContentBgGradientAngle = angle
		}
	}

	config.Save(cfg)

	if kit.Request.Header.Get("HX-Request") == "true" {
		// For HTMX, re-render the Index component to reflect structural changes (adds/deletes)
		// without a full page refresh.
		if section == "notifications" {
			return kit.Render(configuration.Notifications(cfg))
		}
		if section == "facebook_pixel" {
			return kit.Render(configuration.FacebookPixel(cfg))
		}
		if section == "payment" || section == "social_links" || section == "chat_settings" {
			_, products, categories, err := getAdminConfigData(kit.Request, section)
			if err != nil {
				return err
			}
			csrfToken := csrf.Token(kit.Request)
			return kit.Render(configuration.Index(cfg, products, categories, section, csrfToken))
		}

		_, products, categories, err := getAdminConfigData(kit.Request, section)
		if err != nil {
			return err
		}
		csrfToken := csrf.Token(kit.Request)
		return kit.Render(configuration.Index(cfg, products, categories, section, csrfToken))
	}

	return kit.Redirect(http.StatusSeeOther, "/admin/"+section)
}

func HandleAdminNotificationsTest(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	recipient := kit.Request.FormValue("admin_email_recipient")
	if recipient == "" {
		return kit.Text(http.StatusBadRequest, "Please enter a recipient email")
	}

	notifier := services.NewEmailNotifier()
	err := notifier.SendTest(recipient)
	if err != nil {
		return kit.Text(http.StatusInternalServerError, "Failed to send test email: "+err.Error())
	}

	return kit.Text(http.StatusOK, "Test email sent successfully to "+recipient)
}

func HandleAdminTelegramNotificationsTest(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	token := kit.Request.FormValue("telegram_bot_token")
	chatID := kit.Request.FormValue("telegram_chat_id")
	if token == "" || chatID == "" {
		return kit.Text(http.StatusBadRequest, "Please enter both Bot Token and Chat ID")
	}

	notifier := services.NewTelegramNotifier()
	err := notifier.SendTest(token, chatID)
	if err != nil {
		return kit.Text(http.StatusInternalServerError, "Failed to send test message: "+err.Error())
	}

	return kit.Text(http.StatusOK, "Test Telegram message sent successfully")
}

func HandleConfigurationIndex(kit *kit.Kit) error {
	return kit.Redirect(http.StatusSeeOther, "/admin/site")
}

func HandleConfigurationUpdate(kit *kit.Kit) error {
	return kit.Redirect(http.StatusSeeOther, "/admin/site")
}

func HandleAdminSectionAdd(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	cfg := config.FromContext(kit.Request.Context())
	cfg.Sections = append(cfg.Sections, config.SectionConfig{
		Type:       "featured_products",
		Title:      "New Collection",
		Limit:      4,
		Enabled:    true,
		ProductIDs: []string{},
	})
	config.Save(cfg)

	if kit.Request.Header.Get("HX-Request") == "true" {
		_, products, categories, err := getAdminConfigData(kit.Request, "sections")
		if err != nil {
			return err
		}
		csrfToken := csrf.Token(kit.Request)
		return kit.Render(configuration.Index(cfg, products, categories, "sections", csrfToken))
	}

	return kit.Redirect(http.StatusSeeOther, "/admin/sections")
}

func HandleAdminSectionDelete(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	indexStr := chi.URLParam(kit.Request, "index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return kit.Redirect(http.StatusSeeOther, "/admin/sections")
	}

	cfg := config.FromContext(kit.Request.Context())
	if index >= 0 && index < len(cfg.Sections) {
		cfg.Sections = append(cfg.Sections[:index], cfg.Sections[index+1:]...)
		config.Save(cfg)
	}

	if kit.Request.Header.Get("HX-Request") == "true" {
		_, products, categories, err := getAdminConfigData(kit.Request, "sections")
		if err != nil {
			return err
		}
		csrfToken := csrf.Token(kit.Request)
		return kit.Render(configuration.Index(cfg, products, categories, "sections", csrfToken))
	}

	return kit.Redirect(http.StatusSeeOther, "/admin/sections")
}

var srcRe = regexp.MustCompile(`src="([^"]+)"`)

func extractEmbedSrc(v string) string {
	if v == "" {
		return ""
	}
	m := srcRe.FindStringSubmatch(v)
	if len(m) == 2 {
		return m[1]
	}
	return v
}

func HandleAdminSectionDuplicate(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	indexStr := chi.URLParam(kit.Request, "index")
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		return kit.Redirect(http.StatusSeeOther, "/admin/sections")
	}

	cfg := config.FromContext(kit.Request.Context())
	if index >= 0 && index < len(cfg.Sections) {
		original := cfg.Sections[index]
		duplicate := original
		duplicate.Title = duplicate.Title + " (Copy)"

		// Deep copy slices to avoid shared references between original and duplicate
		if original.ProductIDs != nil {
			duplicate.ProductIDs = make([]string, len(original.ProductIDs))
			copy(duplicate.ProductIDs, original.ProductIDs)
		}
		if original.CategoryItems != nil {
			duplicate.CategoryItems = make([]config.CategorySectionItem, len(original.CategoryItems))
			copy(duplicate.CategoryItems, original.CategoryItems)
		}

		// Insert it right after the original
		cfg.Sections = append(cfg.Sections[:index+1], append([]config.SectionConfig{duplicate}, cfg.Sections[index+1:]...)...)
		config.Save(cfg)
	}

	if kit.Request.Header.Get("HX-Request") == "true" {
		_, products, categories, err := getAdminConfigData(kit.Request, "sections")
		if err != nil {
			return err
		}
		csrfToken := csrf.Token(kit.Request)
		return kit.Render(configuration.Index(cfg, products, categories, "sections", csrfToken))
	}

	return kit.Redirect(http.StatusSeeOther, "/admin/sections")
}
