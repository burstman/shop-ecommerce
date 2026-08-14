package handlers

import (
	"fmt"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/views/components"
	"shopTemplate/app/views/landing"
	"strconv"

	"github.com/anthdm/superkit/kit"
)

func HandleLandingIndex(kit *kit.Kit) error {
	// 1. Fetch all settings
	cfg := config.FromContext(kit.Request.Context())

	// 2. Fetch products for each configured section
	sectionProducts := make(map[int][]models.Product)

	for i, section := range cfg.Sections {
		if !section.Enabled {
			continue
		}

		if section.Type == "category_banner" {
			continue
		}

		// Filter empty IDs
		var validIDs []string
		for _, id := range section.ProductIDs {
			if id != "" {
				validIDs = append(validIDs, id)
			}
		}

		if len(validIDs) == 0 {
			continue
		}

		var products []models.Product
		if err := db.Get().Where("id IN ?", validIDs).Preload("Categories").Find(&products).Error; err != nil {
			return err
		}

		// Sort products according to the order in ProductIDs
		productMap := make(map[uint]models.Product)
		for _, p := range products {
			productMap[p.ID] = p
		}

		var orderedProducts []models.Product
		for _, idStr := range validIDs {
			id, _ := strconv.Atoi(idStr)
			if p, ok := productMap[uint(id)]; ok {
				orderedProducts = append(orderedProducts, p)
			}
		}

		sectionProducts[i] = orderedProducts
	}

	// 4. Prepare Carousel Items
	var carouselItems []components.CarouselItem
	if cfg.Hero.Enabled && len(cfg.Hero.Slides) > 0 {
		for _, slide := range cfg.Hero.Slides {
			title := slide.Title
			if title == "" {
				title = cfg.Hero.Title
			}
			subtitle := slide.Subtitle
			if subtitle == "" {
				subtitle = cfg.Hero.Subtitle
			}
			btnText := slide.ButtonText
			if btnText == "" {
				btnText = cfg.Hero.ButtonText
			}
			btnLink := slide.ButtonLink
			if btnLink == "" {
				btnLink = cfg.Hero.ButtonLink
			}

			// If a product ID is linked, override the ButtonLink
			if slide.ProductID != nil && *slide.ProductID != 0 {
				btnLink = fmt.Sprintf("/products/%d", *slide.ProductID)
			}

			carouselItems = append(carouselItems, components.CarouselItem{
				Image:       slide.Image,
				Title:       title,
				Description: subtitle,
				ButtonText:  btnText,
				ButtonLink:  btnLink,
				ProductID:   slide.ProductID,
			})
		}
	}

	return RenderWithLayout(kit, landing.Index(cfg, sectionProducts, carouselItems))
}
