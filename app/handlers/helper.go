package handlers

import (
	"time"

	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/views/layouts"

	"github.com/a-h/templ"
	"github.com/anthdm/superkit/kit"
	"github.com/gorilla/csrf"
)

func RenderWithLayout(kit *kit.Kit, content templ.Component) error {
	var user models.AuthUser
	if authedUser, ok := kit.Auth().(models.AuthUser); ok {
		user = authedUser
	}

	categories := helpers.GetCategoryTree()
	cart := helpers.GetCart(kit)
	csrfToken := csrf.Token(kit.Request)

	return kit.Render(layouts.App(user, config.FromContext(kit.Request.Context()), categories, cart.Total, content, csrfToken))
}

func RenderAdminWithLayout(kit *kit.Kit, sidebar []config.SidebarGroup, activePath string, content templ.Component) error {
	var user models.AuthUser
	if authedUser, ok := kit.Auth().(models.AuthUser); ok {
		user = authedUser
	}

	cfg := config.FromContext(kit.Request.Context())
	categories := helpers.GetCategoryTree()
	cart := helpers.GetCart(kit)
	csrfToken := csrf.Token(kit.Request)

	// Fetch affiliate's expires_at for subscription badge
	var expiresAt *time.Time
	if user.ID > 0 {
		var u models.User
		if err := db.Get().Where("email = ?", user.Email).First(&u).Error; err == nil && u.AffiliateID != "" {
			var aff models.Affiliate
			if err := db.Get().Where("affiliate_id = ?", u.AffiliateID).First(&aff).Error; err == nil {
				expiresAt = aff.ExpiresAt
			}
		}
	}

	return kit.Render(layouts.AdminPage(user, cfg, categories, cart.Total, expiresAt, sidebar, activePath, content, csrfToken))
}
