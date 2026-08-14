package handlers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"strings"
	"time"

	"github.com/anthdm/superkit/kit"
)

type apiContextKey string

const affiliateAPICtxKey apiContextKey = "affiliate_api"

func AffiliateAPIMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		token := strings.TrimPrefix(auth, "Bearer ")

		var affiliate models.Affiliate
		if err := db.Get().Where("api_key = ? AND active = ?", token, true).First(&affiliate).Error; err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), affiliateAPICtxKey, &affiliate)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func getAPIAffiliate(r *http.Request) *models.Affiliate {
	a, _ := r.Context().Value(affiliateAPICtxKey).(*models.Affiliate)
	return a
}

type orderItemJSON struct {
	ProductName string  `json:"product_name"`
	Quantity    int     `json:"quantity"`
	Price       float64 `json:"price"`
}

type orderJSON struct {
	ID               uint            `json:"id"`
	FirstName        string          `json:"first_name"`
	LastName         string          `json:"last_name"`
	Email            string          `json:"email"`
	Total            float64         `json:"total"`
	Commission       float64         `json:"commission"`
	CommissionStatus string          `json:"commission_status"`
	OrderStatus      string          `json:"order_status"`
	CreatedAt        string          `json:"created_at"`
	Items            []orderItemJSON `json:"items,omitempty"`
}

type commissionJSON struct {
	TotalOrders       int64   `json:"total_orders"`
	TotalRevenue      float64 `json:"total_revenue"`
	TotalCommission   float64 `json:"total_commission"`
	PendingCommission float64 `json:"pending_commission"`
	PaidCommission    float64 `json:"paid_commission"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func HandleAPIAffiliateOrders(kit *kit.Kit) error {
	affiliate := getAPIAffiliate(kit.Request)
	if affiliate == nil {
		writeJSON(kit.Response, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil
	}

	var orders []models.Order
	db.Get().Preload("Items").
		Where("affiliate_id = ?", affiliate.ID).
		Order("created_at desc").
		Find(&orders)

	result := make([]orderJSON, 0, len(orders))
	for _, o := range orders {
		items := make([]orderItemJSON, 0, len(o.Items))
		for _, item := range o.Items {
			items = append(items, orderItemJSON{
				ProductName: item.ProductName,
				Quantity:    item.Quantity,
				Price:       item.Price.ToFloat(),
			})
		}
		result = append(result, orderJSON{
			ID:               o.ID,
			FirstName:        o.FirstName,
			LastName:         o.LastName,
			Email:            o.Email,
			Total:            o.Total.ToFloat(),
			Commission:       o.PlatformCommission.ToFloat(),
			CommissionStatus: o.CommissionStatus,
			OrderStatus:      o.Status,
			CreatedAt:        o.CreatedAt.Format("2006-01-02T15:04:05Z"),
			Items:            items,
		})
	}

	writeJSON(kit.Response, http.StatusOK, map[string]any{"orders": result})
	return nil
}

func HandleAPIAffiliateCommission(kit *kit.Kit) error {
	affiliate := getAPIAffiliate(kit.Request)
	if affiliate == nil {
		writeJSON(kit.Response, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil
	}

	type result struct {
		TotalOrders       int64
		TotalRevenue      float64
		TotalCommission   float64
		PendingCommission float64
		PaidCommission    float64
	}

	rows, err := db.Get().Raw(`
		SELECT
			COUNT(*)                                                                   AS total_orders,
			COALESCE(SUM(total), 0)                                                    AS total_revenue,
			COALESCE(SUM(platform_commission), 0)                                      AS total_commission,
			COALESCE(SUM(platform_commission) FILTER (WHERE commission_status = 'pending'), 0) AS pending_commission,
			COALESCE(SUM(platform_commission) FILTER (WHERE commission_status = 'paid'), 0)    AS paid_commission
		FROM orders
		WHERE affiliate_id = ?
	`, affiliate.ID).Rows()
	if err != nil {
		slog.Error("failed to query commission", "err", err)
		writeJSON(kit.Response, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return nil
	}
	defer rows.Close()

	var r result
	if rows.Next() {
		rows.Scan(&r.TotalOrders, &r.TotalRevenue, &r.TotalCommission, &r.PendingCommission, &r.PaidCommission)
	}

	writeJSON(kit.Response, http.StatusOK, commissionJSON{
		TotalOrders:       r.TotalOrders,
		TotalRevenue:      r.TotalRevenue,
		TotalCommission:   r.TotalCommission,
		PendingCommission: r.PendingCommission,
		PaidCommission:    r.PaidCommission,
	})
	return nil
}

func HandleAPIAffiliateExtend(kit *kit.Kit) error {
	affiliate := getAPIAffiliate(kit.Request)
	if affiliate == nil {
		writeJSON(kit.Response, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil
	}

	daysStr := kit.Request.FormValue("days")
	if daysStr == "" {
		writeJSON(kit.Response, http.StatusBadRequest, map[string]string{"error": "days parameter is required"})
		return nil
	}

	days, err := strconv.Atoi(daysStr)
	if err != nil || days <= 0 {
		writeJSON(kit.Response, http.StatusBadRequest, map[string]string{"error": "days must be a positive integer"})
		return nil
	}

	now := time.Now()
	var newExpiry time.Time
	if affiliate.ExpiresAt != nil && affiliate.ExpiresAt.After(now) {
		newExpiry = affiliate.ExpiresAt.AddDate(0, 0, days)
	} else {
		newExpiry = now.AddDate(0, 0, days)
	}

	if err := db.Get().Model(&models.Affiliate{}).Where("id = ?", affiliate.ID).Update("expires_at", newExpiry).Error; err != nil {
		slog.Error("failed to extend affiliate expiry", "err", err)
		writeJSON(kit.Response, http.StatusInternalServerError, map[string]string{"error": "failed to extend"})
		return nil
	}

	writeJSON(kit.Response, http.StatusOK, map[string]any{
		"expires_at": newExpiry.Format(time.RFC3339),
		"days_added": days,
	})
	return nil
}
