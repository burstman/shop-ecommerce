package handlers

import (
	"net/http"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/views/admin"
	"shopTemplate/plugins/auth"
	"strconv"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
)

func HandleAuthentication(kit *kit.Kit) (kit.Auth, error) {
	return auth.AuthenticateUser(kit)
}

func HandleAdminUsersIndex(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	affID := config.AffiliateIDFromContext(kit.Request.Context())
	searchQuery := kit.Request.URL.Query().Get("search")

	var users []models.User
	query := db.Get().Where("affiliate_id = ?", affID).Order("created_at desc")

	if searchQuery != "" {
		searchPattern := "%" + searchQuery + "%"
		query = query.Where("first_name LIKE ? OR last_name LIKE ? OR email LIKE ?", searchPattern, searchPattern, searchPattern)
	}

	if err := query.Find(&users).Error; err != nil {
		return err
	}

	if kit.Request.Header.Get("HX-Request") == "true" {
		return kit.Render(admin.UsersTable(users))
	}
	return RenderWithLayout(kit, admin.UsersIndex(users))
}

func HandleAdminUserEdit(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, _ := strconv.Atoi(idStr)

	var targetUser models.User
	if err := db.Get().First(&targetUser, id).Error; err != nil {
		return err
	}

	return kit.Render(admin.UserEditModal(targetUser))
}

func HandleAdminUserUpdate(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusForbidden, "/")
	}

	idStr := chi.URLParam(kit.Request, "id")
	id, _ := strconv.Atoi(idStr)

	var targetUser models.User
	if err := db.Get().First(&targetUser, id).Error; err != nil {
		return err
	}

	return db.Get().Model(&targetUser).Updates(map[string]any{
		"first_name": kit.Request.FormValue("first_name"),
		"last_name":  kit.Request.FormValue("last_name"),
		"role":       kit.Request.FormValue("role"),
	}).Error
}
