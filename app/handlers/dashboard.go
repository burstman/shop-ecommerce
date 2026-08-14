package handlers

import (
	"net/http"
	"time"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/views/dashboard"

	"github.com/anthdm/superkit/kit"
)

func HandleAdminDashboard(kit *kit.Kit) error {
	user, ok := kit.Auth().(models.AuthUser)
	if !ok || user.Role != "admin" {
		return kit.Redirect(http.StatusSeeOther, "/")
	}

	var totalOrders int64
	db.Get().Model(&models.Order{}).Count(&totalOrders)

	type revenueResult struct {
		Total float64
	}
	var totalRev revenueResult
	db.Get().Model(&models.Order{}).Select("COALESCE(SUM(total), 0) as total").Where("status != ?", "cancelled").Scan(&totalRev)

	type statusCount struct {
		Status string
		Count  int64
	}
	var statusCounts []statusCount
	db.Get().Model(&models.Order{}).Select("status, COUNT(*) as count").Group("status").Scan(&statusCounts)

	ordersByStatus := make(map[string]int64)
	for _, sc := range statusCounts {
		ordersByStatus[sc.Status] = sc.Count
	}

	var recentOrders []models.Order
	db.Get().Preload("Items").Order("created_at desc").Limit(10).Find(&recentOrders)

	totalRevenue := totalRev.Total

	// Fetch affiliate expiry
	var expiresAt *time.Time
	var affiliate models.Affiliate
	if err := db.Get().Where("affiliate_id = ?", "AFF-001").First(&affiliate).Error; err == nil {
		expiresAt = affiliate.ExpiresAt
	}

	data := dashboard.DashboardData{
		TotalOrders:    totalOrders,
		TotalRevenue:   totalRevenue,
		ExpiresAt:      expiresAt,
		OrdersByStatus:  ordersByStatus,
		RecentOrders:    recentOrders,
	}

	cfg := config.FromContext(kit.Request.Context())
	activePath := "/admin/dashboard"
	sidebar := config.GetAdminSidebarGroups()
	content := dashboard.Index(data, cfg)
	return RenderAdminWithLayout(kit, sidebar, activePath, content)
}
