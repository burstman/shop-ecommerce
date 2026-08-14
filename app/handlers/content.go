package handlers

import (
	"net/http"
	"shopTemplate/app/config"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/views/content"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
)

func HandleContentPage(kit *kit.Kit) error {
	user, _ := kit.Auth().(models.AuthUser)
	cfg := config.FromContext(kit.Request.Context())
	categories := helpers.GetCategoryTree()
	cart := helpers.GetCart(kit)

	slug := chi.URLParam(kit.Request, "slug")

	switch slug {
	case "livraison":
		return kit.Render(content.Livraison(user, cfg, categories, cart.Total))
	case "a-propos":
		return kit.Render(content.APropos(user, cfg, categories, cart.Total))
	default:
		http.NotFound(kit.Response, kit.Request)
		return nil
	}
}
