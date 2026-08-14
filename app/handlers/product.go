package handlers

import (
	"errors"
	"fmt"
	"log"
	"log/slog"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
	viewerrors "shopTemplate/app/views/errors"
	"shopTemplate/app/views/products"

	"github.com/anthdm/superkit/kit"
	"github.com/anthdm/superkit/validate"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/csrf"
	"gorm.io/gorm"
)

func HandleAdminProductsIndex(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	q := kit.Request.URL.Query().Get("q")
	pageStr := kit.Request.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	perPage := 10

	query := db.Get().Model(&models.Product{})
	if q != "" {
		query = query.Where("name ILIKE ?", "%"+q+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return err
	}
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	offset := (page - 1) * perPage

	var productsList []models.Product
	if err := query.Preload("Categories").Order("created_at desc").Limit(perPage).Offset(offset).Find(&productsList).Error; err != nil {
		return err
	}

	activePath := "/admin/products"
	sidebar := config.GetAdminSidebarGroups()
	cfg := config.Get()

	isHX := kit.Request.Header.Get("HX-Request") != ""
	if isHX {
		return kit.Render(products.AdminList(productsList, page, totalPages, cfg, q))
	}

	content := products.AdminList(productsList, page, totalPages, cfg, q)
	return RenderAdminWithLayout(kit, sidebar, activePath, content)
}

// HandleProductsIndex renders the public product listing page.
func HandleProductsIndex(kit *kit.Kit) error {
	pageStr := kit.Request.URL.Query().Get("page")
	page, _ := strconv.Atoi(pageStr)
	if page < 1 {
		page = 1
	}
	perPage := 12

	var total int64
	err := db.Get().Model(&models.Product{}).Count(&total).Error
	if err != nil {
		return err
	}
	totalPages := int(math.Ceil(float64(total) / float64(perPage)))
	offset := (page - 1) * perPage

	var productsList []models.Product
	if err := db.Get().Preload("Categories").Order("created_at desc").Limit(perPage).Offset(offset).Find(&productsList).Error; err != nil {
		return err
	}

	cfg := config.Get()
	return RenderWithLayout(kit, products.Index(productsList, page, totalPages, cfg))
}

func HandleProductNew(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	categories := helpers.GetCategoryTree()

	activePath := "/products/new"
	sidebar := config.GetAdminSidebarGroups()
	cfg := config.Get()
	csrfToken := csrf.Token(kit.Request)
	content := products.New(categories, products.CreateForm{
		Errors:     make(validate.Errors),
		Categories: categories,
	}, cfg, csrfToken)
	return RenderAdminWithLayout(kit, sidebar, activePath, content)
}

func HandleProductCreate(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	// Parse form (max 10MB)
	err := kit.Request.ParseMultipartForm(10 << 20)
	if err != nil {
		return err
	}

	// 1. Handle Images
	errors := make(validate.Errors)
	name := kit.Request.FormValue("name")
	priceStr := kit.Request.FormValue("price")
	description := kit.Request.FormValue("description")
	promotionPriceStr := kit.Request.FormValue("promotion_price")
	stockStr := kit.Request.FormValue("stock")
	
	files := kit.Request.MultipartForm.File["images"]
	categoryIDs := kit.Request.MultipartForm.Value["categories"]
	
	if name == "" {
		errors.Add("name", "Name is required")
	}
	var price float64
	if priceStr == "" {
		errors.Add("price", "Price is required")
	} else {
		var err error
		price, err = strconv.ParseFloat(priceStr, 64)
		if err != nil {
			errors.Add("price", "Price must be a valid number")
		}
	}
	var promotionPrice float64
	if promotionPriceStr != "" {
		p, err := strconv.ParseFloat(promotionPriceStr, 64)
		if err != nil {
			errors.Add("promotion_price", "Promotion price must be a valid number")
		} else {
			promotionPrice = p
		}
	}
	
	if len(files) == 0 {
		errors.Add("images", "At least one image is required")
	} else if len(files) > 10 {
		errors.Add("images", "Maximum 10 images allowed")
	}
	var stock int
	if stockStr != "" {
		stock, err = strconv.Atoi(stockStr)
		if err != nil {
			errors.Add("stock", "Stock must be a valid integer")
		}
	}

	hasCategory := false
	for _, id := range categoryIDs {
		if id != "" {
			hasCategory = true
			break
		}
	}
	if !hasCategory {
		errors.Add("categories", "Please select at least one category")
	}

	if len(errors) > 0 {
		categories := helpers.GetCategoryTree()
		cfg := config.Get()
		activePath := "/products/new"
		sidebar := config.GetAdminSidebarGroups()
		form := products.CreateForm{
			Values: map[string]string{
				"name":            name,
				"price":           priceStr,
				"description":     description,
				"promotion_price": promotionPriceStr,
				"stock":           stockStr,
			},
			Categories:          categories,
			SelectedCategoryIDs: categoryIDs,
			Errors:              errors,
		}
		csrfToken := csrf.Token(kit.Request)
		content := products.New(categories, form, cfg, csrfToken)
		return RenderAdminWithLayout(kit, sidebar, activePath, content)
	}

	// Handle Images Upload
	var imageURLs []string
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			return err
		}
		aff := config.AffiliateFromContext(kit.Request.Context())
		affID := "default"
		if aff != nil {
			affID = aff.AffiliateID
		}
		url, err := helpers.UploadImage(file, header, affID, "products", "plant")
		file.Close()
		if err != nil {
			return err
		}
		imageURLs = append(imageURLs, url)
	}

	// 2. Save Product to DB
	product := models.Product{
		Name:           name,
		Description:    description,
		Price:          models.NewCurrency(price),
		PromotionPrice: models.NewCurrency(promotionPrice),
		Stock:          stock,
		Images:         imageURLs,
		Image:          imageURLs[0], // Keep for backward compatibility
		Bundles:        parseBundles(kit),
		BundlesEnabled: kit.Request.FormValue("bundles_enabled") == "on",
	}

	if err := db.Get().Create(&product).Error; err != nil {
		return err
	}

	// Associate categories
	if len(categoryIDs) > 0 {
		var categoriesToAssign []models.Category
		if err := db.Get().Find(&categoriesToAssign, categoryIDs).Error; err == nil {
			if err := db.Get().Model(&product).Association("Categories").Replace(categoriesToAssign); err != nil {
				return err
			}
		}
	}

	// Redirect to refresh the page (or you could return an OOB swap for the grid)
	return kit.Redirect(http.StatusSeeOther, "/admin/products")
}

// HandleProductDeleteConfirm renders a confirmation modal for product deletion.
func HandleProductDeleteConfirm(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return errors.New("invalid product ID")
	}

	var product models.Product
	if err := db.Get().First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kit.Render(viewerrors.Error404())
		}
		return err
	}

	return kit.Render(products.DeleteModal(product))
}

// HandleProductDelete handles the deletion of a product by its ID.
// Only users with the "admin" role are authorized to perform this action.
// The function retrieves the product ID from the URL parameters, validates it,
// and performs a soft delete operation on the product record.
// If the user is unauthorized or the product ID is invalid, appropriate errors are returned.
// On successful deletion, an empty response with HTTP status 200 is returned.
func HandleProductDelete(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		log.Printf("Unauthorized delete attempt by user: %v", user)
		return kit.Redirect(http.StatusForbidden, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return errors.New("invalid product ID")
	}

	var product models.Product
	if err := db.Get().First(&product, id).Error; err != nil {
		return err
	}

	for _, img := range product.Images {
		if len(img) > 1 && img[0] == '/' {
			if err := os.Remove(strings.TrimPrefix(img, "/")); err != nil {
				slog.Warn("failed to remove product image", "path", img, "error", err)
				services.ReportWarning(kit.Request, err.Error())
			}
		}
	}

	if err := db.Get().Delete(&product).Error; err != nil {
		return err
	}

	// Toast message removed as per user request.
	// HTMX on the client side should handle removing the element from the DOM.
	return nil
}

// HandleProductEdit handles the editing of a product by an admin user.
// It checks if the authenticated user has the "admin" role, retrieves the product ID from the URL,
// fetches the corresponding product from the database, and renders the product edit modal.
// If the user is not an admin or the product ID is invalid, it redirects to the home page or returns an error.
func HandleProductEdit(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return errors.New("invalid product ID")
	}

	var product models.Product
	if err := db.Get().First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kit.Render(viewerrors.Error404())
		}
		return err
	}

	allCategories := helpers.GetCategoryTree()

	// Preload existing categories for the product
	if err := db.Get().Model(&product).Association("Categories").Find(&product.Categories); err != nil {
		return err
	}

	cfg := config.Get()
	modal := products.EditModal(product, allCategories, cfg)

	if kit.Request.Header.Get("HX-Request") == "true" {
		return kit.Render(modal)
	}

	activePath := "/admin/products"
	sidebar := config.GetAdminSidebarGroups()
	return RenderAdminWithLayout(kit, sidebar, activePath, modal)
}

// HandleProductUpdate handles the update of a product by an admin user.
// It verifies the user's role, parses the product ID from the URL, retrieves the product from the database,
// updates the product's name and price from form values, and optionally updates the product's image if provided.
// The updated product is saved to the database. Non-admin users are redirected to the home page.
// On success, redirects to the plants listing page.
func HandleProductUpdate(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return errors.New("invalid product ID")
	}

	var product models.Product
	if err := db.Get().First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kit.Render(viewerrors.Error404())
		}
		return err
	}

	// Parse form
	if err := kit.Request.ParseMultipartForm(10 << 20); err != nil {
		return err
	}

	product.Name = kit.Request.FormValue("name")
	product.Description = kit.Request.FormValue("description")
	if price, err := strconv.ParseFloat(kit.Request.FormValue("price"), 64); err == nil {
		product.Price = models.NewCurrency(price)
	}
	promPriceStr := kit.Request.FormValue("promotion_price")
	if promPriceStr == "" {
		product.PromotionPrice = 0
	} else if promPrice, err := strconv.ParseFloat(promPriceStr, 64); err == nil {
		product.PromotionPrice = models.NewCurrency(promPrice)
	}
	if stock, err := strconv.Atoi(kit.Request.FormValue("stock")); err == nil {
		product.Stock = stock
	}

	// 1. Unified Image Management
	// First, ensure product.Images is fully populated from both fields
	if product.Image != "" {
		found := false
		for _, img := range product.Images {
			if img == product.Image {
				found = true
				break
			}
		}
		if !found {
			product.Images = append([]string{product.Image}, product.Images...)
		}
	}

	// 2. Handle New Images Upload
	files := kit.Request.MultipartForm.File["images"]
	for _, header := range files {
		if len(product.Images) >= 10 {
			break
		}
		file, err := header.Open()
		if err != nil {
			continue
		}
		aff := config.AffiliateFromContext(kit.Request.Context())
		affID := "default"
		if aff != nil {
			affID = aff.AffiliateID
		}
		url, uploadErr := helpers.UploadImage(file, header, affID, "products", "plant")
		file.Close()
		if uploadErr == nil {
			product.Images = append(product.Images, url)
		}
	}

	// 3. Handle Image Deletions
	deleteImages := kit.Request.MultipartForm.Value["delete_images"]
	if len(deleteImages) > 0 {
		var updatedImages []string
		for _, img := range product.Images {
			shouldDelete := false
			for _, delImg := range deleteImages {
				if img == delImg {
					shouldDelete = true
					break
				}
			}
			if shouldDelete {
				if len(img) > 1 && img[0] == '/' {
					if err := os.Remove(strings.TrimPrefix(img, "/")); err != nil {
						slog.Warn("failed to remove deleted product image", "path", img, "error", err)
						services.ReportWarning(kit.Request, err.Error())
					}
				}
			} else {
				updatedImages = append(updatedImages, img)
			}
		}
		product.Images = updatedImages
	}

	// 4. Sync Singular Image Field
	primaryImage := kit.Request.FormValue("primary_image")
	if primaryImage != "" {
		// Verify the primary image is still in the gallery
		found := false
		for _, img := range product.Images {
			if img == primaryImage {
				found = true
				break
			}
		}
		if found {
			product.Image = primaryImage
		} else if len(product.Images) > 0 {
			product.Image = product.Images[0]
		} else {
			product.Image = ""
		}
	} else if len(product.Images) > 0 {
		product.Image = product.Images[0]
	} else {
		product.Image = ""
	}

	// 5. Update categories
	categoryIDs := kit.Request.Form["categories"]
	if len(categoryIDs) > 0 {
		var categoriesToAssign []models.Category
		if err := db.Get().Find(&categoriesToAssign, categoryIDs).Error; err != nil {
			return err
		}
		if err := db.Get().Model(&product).Association("Categories").Replace(categoriesToAssign); err != nil {
			return err
		}
	}

	product.Bundles = parseBundles(kit)
	product.BundlesEnabled = kit.Request.FormValue("bundles_enabled") == "on"

	if err := db.Get().Save(&product).Error; err != nil {
		return err
	}

	return kit.Redirect(http.StatusSeeOther, "/admin/products")
}

// HandleProductQuickView renders the quick view modal for a product.
func HandleProductQuickView(kit *kit.Kit) error {
	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return errors.New("invalid product ID")
	}

	var product models.Product
	if err := db.Get().Preload("Categories").First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kit.Render(viewerrors.Error404())
		}
		return err
	}
	cfg := config.Get()

	return kit.Render(products.QuickViewModal(product, cfg))
}

// HandleProductShow renders the standalone product detail page.
func HandleProductShow(kit *kit.Kit) error {
	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return errors.New("invalid product ID")
	}

	var product models.Product
	if err := db.Get().Preload("Categories").First(&product, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return kit.Render(viewerrors.Error404())
		}
		return err
	}

	cfg := config.Get()
	return RenderWithLayout(kit, products.Show(product, cfg))
}

func parseBundles(kit *kit.Kit) []models.Bundle {
	var bundles []models.Bundle
	if countStr := kit.Request.FormValue("bundles_count"); countStr != "" {
		count, _ := strconv.Atoi(countStr)
		for j := 0; j < count; j++ {
			prefix := fmt.Sprintf("bundle_%d_", j)
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

	sort.Slice(bundles, func(i, j int) bool {
		return bundles[i].Quantity < bundles[j].Quantity
	})

	return bundles
}
