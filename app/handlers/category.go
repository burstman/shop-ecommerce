package handlers

import (
	"net/http"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/views/components"
	conf "shopTemplate/app/views/configuration"
	"strconv"

	"github.com/a-h/templ"
	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
)

func HandleAdminCategoriesIndex(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	categories := helpers.GetCategoryTree()

	var allCategories []models.Category
	if err := db.Get().Order("name asc").Find(&allCategories).Error; err != nil {
		return err
	}

	activePath := "/admin/categories"
	sidebar := config.GetAdminSidebarGroups()
	content := conf.CategoriesIndex(categories, allCategories)
	return RenderAdminWithLayout(kit, sidebar, activePath, content)
}

func HandleAdminCategoryCreate(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	name := kit.Request.FormValue("name")
	parentIDStr := kit.Request.FormValue("parent_id")

	category := models.Category{
		Name: name,
	}

	var parentIDPtr *uint
	if parentIDStr != "" && parentIDStr != "0" {
		parentID, err := strconv.Atoi(parentIDStr)
		if err == nil {
			var parentCategory models.Category
			if err := db.Get().First(&parentCategory, parentID).Error; err == nil {
				if parentCategory.IsLocked {
					// TODO: Add a flash message to inform the user why it failed.
					return kit.Redirect(http.StatusSeeOther, "/admin/categories")
				}
			}

			uid := uint(parentID)
			category.ParentID = &uid
			parentIDPtr = &uid
		}
	}

	var count int64
	tx := db.Get().Model(&models.Category{})
	if parentIDPtr == nil {
		tx.Where("parent_id IS NULL")
	} else {
		tx.Where("parent_id = ?", parentIDPtr)
	}
	if err := tx.Count(&count).Error; err != nil {
		return err
	}
	category.Position = int(count)

	if err := db.Get().Create(&category).Error; err != nil {
		return err
	}

	return kit.Redirect(http.StatusSeeOther, "/admin/categories")
}

func HandleAdminCategoryReorder(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	if err := kit.Request.ParseForm(); err != nil {
		return kit.Text(http.StatusBadRequest, "Invalid form data")
	}

	categoryIDs := kit.Request.Form["category_ids"]
	if len(categoryIDs) == 0 {
		return kit.Text(http.StatusBadRequest, "No category IDs provided")
	}

	for i, idStr := range categoryIDs {
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}
		var cat models.Category
		if err := db.Get().First(&cat, id).Error; err != nil || cat.IsLocked {
			continue
		}
		if err := db.Get().Model(&models.Category{}).Where("id = ?", id).Update("position", i).Error; err != nil {
			return err
		}
	}

	// Fetch updated categories to update the navigation bar out-of-band
	categories := helpers.GetCategoryTree()

	cfg := config.FromContext(kit.Request.Context())
	cart := helpers.GetCart(kit)

	// Render the Navigation component with OOB swap targeting the #main-navigation element
	attrs := templ.Attributes{"hx-swap-oob": "true"}
	return kit.Render(components.Navigation(user, cfg, categories, cart.Total, attrs))
}

func HandleAdminCategoryDelete(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return kit.Redirect(http.StatusSeeOther, "/admin/categories")
	}

	var category models.Category
	if err := db.Get().First(&category, id).Error; err != nil {
		// Category not found
		return kit.Redirect(http.StatusSeeOther, "/admin/categories")
	}

	if category.IsLocked {
		// TODO: Add a flash message to inform the user why it failed.
		return kit.Redirect(http.StatusForbidden, "/admin/categories")
	}

	// GORM's delete with a struct will trigger soft delete if the model has DeletedAt field
	// Using Unscoped().Delete() for a hard delete.
	// The constraint `OnDelete:CASCADE` in the model will handle subcategories.
	if err := db.Get().Unscoped().Delete(&models.Category{}, id).Error; err != nil {
		return err
	}

	return kit.Redirect(http.StatusSeeOther, "/admin/categories")
}
