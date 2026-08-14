package handlers

import (
	"net/http"
	"shopTemplate/app/config"
	"strconv"

	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/views/products"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
)

func HandleCategoryShow(kit *kit.Kit) error {
	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return kit.Redirect(http.StatusSeeOther, "/products")
	}

	var category models.Category
	if err := db.Get().First(&category, id).Error; err != nil {
		return kit.Redirect(http.StatusSeeOther, "/products")
	}

	if category.Slug != nil && *category.Slug == "home" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	// Build breadcrumbs by traversing up the parent categories.
	var breadcrumbs []models.Category
	breadcrumbs = []models.Category{category}
	parentID := category.ParentID

	for parentID != nil {
		var parent models.Category
		if err := db.Get().First(&parent, *parentID).Error; err != nil {
			break
		}
		breadcrumbs = append([]models.Category{parent}, breadcrumbs...)
		parentID = parent.ParentID
	}

	var items []models.Product
	if err := db.Get().Joins("JOIN product_categories ON product_categories.product_id = products.id").Where("product_categories.category_id = ?", id).Preload("Categories").Find(&items).Error; err != nil {
		return err
	}

	cfg := config.FromContext(kit.Request.Context())
	return RenderWithLayout(kit, products.CategoryShow(category, items, breadcrumbs, cfg))
}
