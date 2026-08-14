package handlers

import (
	"shopTemplate/app/config"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/views/stores"

	"github.com/anthdm/superkit/kit"
)

func HandleStoresIndex(kit *kit.Kit) error {
	user, _ := kit.Auth().(models.AuthUser)
	cfg := config.FromContext(kit.Request.Context())
	categories := helpers.GetCategoryTree()
	cart := helpers.GetCart(kit)
	return kit.Render(stores.Index(user, cfg, categories, cart.Total))
}
