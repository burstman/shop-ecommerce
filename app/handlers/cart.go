package handlers

import (
	"encoding/gob"
	"net/http"
	"shopTemplate/app/db"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	cartView "shopTemplate/app/views/cart"
	"shopTemplate/app/services"
	"strings"
	"strconv"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
)

func init() {
	gob.Register(&models.Cart{})
	gob.Register(&models.CartItem{})
}

func saveCart(kit *kit.Kit, cart *models.Cart) {
	cart.Total = 0
	for _, item := range cart.Items {
		cart.Total += item.Quantity
	}
	sess := kit.GetSession("session")
	sess.Values["cart"] = cart
	sess.Save(kit.Request, kit.Response)
}

func HandleCartShow(kit *kit.Kit) error {
	cart := helpers.GetCart(kit)
	return RenderWithLayout(kit, cartView.Index(cart))
}

func HandleCartAdd(kit *kit.Kit) error {
	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return kit.Text(http.StatusBadRequest, "Invalid product ID")
	}

	cart := helpers.GetCart(kit)
	qty := 1
	if q, err := strconv.Atoi(kit.Request.FormValue("quantity")); err == nil && q > 0 {
		qty = q
	}

	if item, ok := cart.Items[uint(id)]; ok {
		item.Quantity += qty
	} else {
		var product models.Product
		if err := db.Get().First(&product, id).Error; err != nil {
			return kit.Text(http.StatusNotFound, "Product not found")
		}
		cart.Items[uint(id)] = &models.CartItem{
			Product:  product,
			Quantity: qty,
		}
	}

	saveCart(kit, cart)

	// Trigger Facebook CAPI AddToCart event in a goroutine to avoid blocking the user
	go func(id int, qty int) {
		var product models.Product
		if err := db.Get().First(&product, id).Error; err == nil {
			capiSvc := services.NewFacebookCAPIService()
			url := kit.Request.Host + kit.Request.URL.String()
			ip := kit.Request.RemoteAddr
			if forwarded := kit.Request.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = strings.Split(forwarded, ",")[0]
			}
			ua := kit.Request.UserAgent()
			price := product.Price.ToFloat()
			if product.PromotionPrice > 0 {
				price = product.PromotionPrice.ToFloat()
			}
			price = price * float64(qty)
			capiSvc.SendAddToCartEvent(uint(id), product.Name, price, url, ip, ua)
		}
	}(id, qty)

	redirect := kit.Request.URL.Query().Get("redirect")
	if redirect != "" {
		kit.Response.Header().Set("HX-Redirect", redirect)
		return nil
	}

	return kit.Render(cartView.AddToCartResponse(cart.Total, "Added to cart!"))
}

// HandleCartRemove removes a product from the shopping cart.
func HandleCartRemove(kit *kit.Kit) error {
	idStr := chi.URLParam(kit.Request, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return kit.Text(http.StatusBadRequest, "Invalid product ID")
	}

	cart := helpers.GetCart(kit)
	delete(cart.Items, uint(id))

	saveCart(kit, cart)

	if kit.Request.Header.Get("HX-Request") == "true" {
		kit.Response.Header().Set("HX-Redirect", kit.Request.Referer())
		return nil
	}

	return kit.Redirect(http.StatusSeeOther, "/cart")
}
