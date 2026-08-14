package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/handlers"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
	"shopTemplate/app/views/errors"
	"shopTemplate/plugins/auth"
	"strings"

	"github.com/anthdm/superkit/kit"
	"github.com/anthdm/superkit/kit/middleware"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/csrf"
)

// Define your global middleware
func InitializeMiddleware(router *chi.Mux) {
	router.Use(chimiddleware.RequestID)
	router.Use(CORSMiddleware)
	router.Use(chimiddleware.RealIP)
	router.Use(chimiddleware.Logger)
	router.Use(PanicRecoverer)
	router.Use(middleware.WithRequest)
	router.Use(StoreDomainMiddleware)
	router.Use(FacebookCAPIMiddleware)
	router.Use(I18nMiddleware)
	router.Use(adminSetupMiddleware)
}

// StoreDomainMiddleware looks up the affiliate by the shop URL
// and stores it in the request context for shop-scoped config access.
// If no affiliate matches, it auto-creates a new one for this host.
func StoreDomainMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/public/") || strings.HasPrefix(r.URL.Path, "/_templ/") {
			next.ServeHTTP(w, r)
			return
		}

		affiliate := config.LookupAffiliateByShopURL(r.Host)
		if affiliate != nil {
			ctx := config.WithAffiliate(r.Context(), affiliate)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Auto-register: create a new affiliate for this host
		var count int64
		db.Get().Model(&models.Affiliate{}).Count(&count)
		affiliateID := fmt.Sprintf("AFF-%03d", count+1)
		keyBytes := make([]byte, 32)
		if _, err := rand.Read(keyBytes); err != nil {
			slog.Error("failed to generate api_key", "err", err)
			next.ServeHTTP(w, r)
			return
		}
		newAff := models.Affiliate{
			AffiliateID: affiliateID,
			Name:        r.Host,
			ShopURL:     r.Host,
			APIKey:      hex.EncodeToString(keyBytes),
			Active:      true,
		}
		if err := db.Get().Create(&newAff).Error; err != nil {
			slog.Error("failed to auto-create affiliate", "host", r.Host, "err", err)
			next.ServeHTTP(w, r)
			return
		}
		slog.Info("auto-created new affiliate", "affiliate_id", affiliateID, "shop_url", newAff.ShopURL)
		ctx := config.WithAffiliate(r.Context(), &newAff)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func CORSMiddleware(next http.Handler) http.Handler {
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, X-Requested-With, HX-Request, HX-Trigger, HX-Current-URL")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func I18nMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := "en" // default

		// Check cookie first
		if cookie, err := r.Cookie("lang"); err == nil {
			lang = cookie.Value
		} else {
		// Check Accept-Language header
		accept := r.Header.Get("Accept-Language")
		if strings.HasPrefix(accept, "fr") {
			lang = "fr"
		} else if strings.HasPrefix(accept, "ar") {
			lang = "ar"
		}
		}

		ctx := context.WithValue(r.Context(), "lang", lang)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func HandleSetLang(kit *kit.Kit) error {
	lang := chi.URLParam(kit.Request, "lang")
	if lang != "en" && lang != "fr" && lang != "ar" {
		lang = "en"
	}

	cookie := &http.Cookie{
		Name:     "lang",
		Value:    lang,
		Path:     "/",
		MaxAge:   86400 * 30, // 30 days
		HttpOnly: true,
	}
	http.SetCookie(kit.Response, cookie)

	referer := kit.Request.Referer()
	if referer == "" {
		referer = "/"
	}
	return kit.Redirect(http.StatusSeeOther, referer)
}

func FacebookCAPIMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only track GET requests for PageView
		// Exclude static assets, API calls, and Admin routes (optional)
		if r.Method == http.MethodGet &&
			!strings.HasPrefix(r.URL.Path, "/public") &&
			!strings.HasPrefix(r.URL.Path, "/api") &&
			!strings.HasPrefix(r.URL.Path, "/admin") &&
			!strings.HasPrefix(r.URL.Path, "/_templ") {

			capiSvc := services.NewFacebookCAPIService()
			url := r.Host + r.URL.String()
			if r.TLS != nil {
				url = "https://" + url
			} else {
				url = "http://" + url
			}

			ip := r.RemoteAddr
			if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
				ip = strings.Split(forwarded, ",")[0]
			}
			ua := r.UserAgent()

			go capiSvc.SendPageViewEvent(url, ip, ua)
		}
		next.ServeHTTP(w, r)
	})
}

// Define your routes in here
func InitializeRoutes(router *chi.Mux) {
	// Admin setup (must be before auth/CSRF groups since no admin exists yet)
	router.Get("/setup", kit.Handler(handlers.HandleSetupIndex))
	router.Post("/setup", kit.Handler(handlers.HandleSetupCreate))

	// Affiliate API (token-based auth, no CSRF needed)
	router.Group(func(app chi.Router) {
		app.Use(handlers.AffiliateAPIMiddleware)
		app.Get("/api/orders", kit.Handler(handlers.HandleAPIAffiliateOrders))
		app.Get("/api/commission", kit.Handler(handlers.HandleAPIAffiliateCommission))
		app.Post("/api/affiliate/extend", kit.Handler(handlers.HandleAPIAffiliateExtend))
	})

	// CSRF protection using gorilla/csrf
	csrfMiddleware := csrf.Protect(
		getCSRFKey(),
		csrf.CookieName("_csrf_token"),
		csrf.FieldName("csrf_token"),
		csrf.RequestHeader("X-CSRF-Token"),
		csrf.Secure(kit.IsProduction()),
		csrf.Path("/"),
		csrf.ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slog.Warn("CSRF validation failed", "path", r.URL.Path, "method", r.Method, "error", csrf.FailureReason(r).Error(), "origin", r.Header.Get("Origin"), "host", r.Host)
			services.ReportWarning(r, csrf.FailureReason(r).Error())
			http.Error(w, "Forbidden - CSRF token invalid", http.StatusForbidden)
		})),
	)

	// Authentication plugin
	//
	// By default the auth plugin is active, to disable the auth plugin
	// you will need to pass your own handler in the `AuthFunc`` field
	// of the `kit.AuthenticationConfig`.
	//  authConfig := kit.AuthenticationConfig{
	//      AuthFunc: YourAuthHandler,
	//      RedirectURL: "/login",
	//  }
	auth.InitializeRoutes(router)
	authConfig := kit.AuthenticationConfig{
		AuthFunc:    handlers.HandleAuthentication,
		RedirectURL: "/login",
	}

	// WebSocket route without the standard HTTP logger middleware to avoid duration noise
	router.With(kit.WithAuthentication(authConfig, false)).Get("/api/chat/ws", kit.Handler(handlers.HandleChatWS))

	// Routes that "might" have an authenticated user
	router.Group(func(app chi.Router) {
		app.Use(kit.WithAuthentication(authConfig, false)) // strict set to false
		app.Use(plaintextCSRFMiddleware)
		app.Use(csrfMiddleware)

		// Routes
		app.Get("/", kit.Handler(handlers.HandleLandingIndex))
		app.Get("/privacy", kit.Handler(handlers.HandlePrivacyPolicy))
		app.Get("/content/{slug}", kit.Handler(handlers.HandleContentPage))
		app.Get("/magasins", kit.Handler(handlers.HandleStoresIndex))
		app.Get("/data-deletion", kit.Handler(handlers.HandleDataDeletion))
		app.Get("/set-lang/{lang}", kit.Handler(HandleSetLang))
		app.Get("/products", kit.Handler(handlers.HandleProductsIndex))
		app.Get("/health", kit.Handler(handlers.HandleHealthCheck))
		app.Get("/products/{id}", kit.Handler(handlers.HandleProductShow))
		app.Get("/categories/{id}", kit.Handler(handlers.HandleCategoryShow))
		app.Get("/products/{id}/quick-view", kit.Handler(handlers.HandleProductQuickView))
		app.With(handlers.RateLimitCart.Middleware).Post("/cart/add/{id}", kit.Handler(handlers.HandleCartAdd))
		app.Delete("/cart/remove/{id}", kit.Handler(handlers.HandleCartRemove))
		app.Get("/cart", kit.Handler(handlers.HandleCartShow))
		app.Get("/checkout", kit.Handler(handlers.HandleCheckoutIndex))
		app.Post("/checkout/abandoned", kit.Handler(handlers.HandleCheckoutAbandoned))
		app.Get("/checkout/success", kit.Handler(handlers.HandleCheckoutSuccess))
		app.With(handlers.RateLimitCheckout.Middleware).Post("/checkout", kit.Handler(handlers.HandleCheckoutCreate))
		app.Get("/api/chat/messages", kit.Handler(handlers.HandleChatFetchMessages))
		app.With(handlers.RateLimitChat.Middleware).Post("/api/chat/send", kit.Handler(handlers.HandleChatSend))
	})

	// Authenticated routes
	//
	// Routes that "must" have an authenticated user or else they
	// will be redirected to the configured redirectURL, set in the
	// AuthenticationConfig.
	router.Group(func(app chi.Router) {
		app.Use(kit.WithAuthentication(authConfig, true)) // strict set to true
		app.Use(plaintextCSRFMiddleware)
		app.Use(csrfMiddleware)

		// Routes
		app.Get("/admin/dashboard", kit.Handler(handlers.HandleAdminDashboard))
		app.Get("/admin/categories", kit.Handler(handlers.HandleAdminCategoriesIndex))
		app.Post("/admin/categories", kit.Handler(handlers.HandleAdminCategoryCreate))
		app.Delete("/admin/categories/{id}", kit.Handler(handlers.HandleAdminCategoryDelete))
		app.Post("/admin/categories/reorder", kit.Handler(handlers.HandleAdminCategoryReorder))
		app.Get("/admin/orders", kit.Handler(handlers.HandleAdminOrdersIndex))
		app.Get("/admin/orders/{id}", kit.Handler(handlers.HandleAdminOrderShow))
		app.Post("/admin/orders/{id}/status", kit.Handler(handlers.HandleAdminOrderUpdateStatus))
		app.Get("/admin/orders/{id}/delete", kit.Handler(handlers.HandleAdminOrderDeleteConfirm))
		app.Get("/admin/orders/{id}/cancel", kit.Handler(handlers.HandleAdminOrderCancelConfirm))
		app.Delete("/admin/orders/{id}", kit.Handler(handlers.HandleAdminOrderDelete))
		app.Get("/admin/products", kit.Handler(handlers.HandleAdminProductsIndex))
		app.Get("/admin/users", kit.Handler(handlers.HandleAdminUsersIndex))
		app.Get("/admin/users/{id}/edit", kit.Handler(handlers.HandleAdminUserEdit))
		app.Put("/admin/users/{id}", kit.Handler(handlers.HandleAdminUserUpdate))
		app.Get("/admin/chats", kit.Handler(handlers.HandleAdminChatIndex))
		app.Get("/admin/chat/{id}", kit.Handler(handlers.HandleAdminChatShow))
		app.Post("/admin/chat/{id}/send", kit.Handler(handlers.HandleAdminChatSend))
		app.Post("/admin/chat/{id}/ban", kit.Handler(handlers.HandleAdminChatBan))
		app.Get("/admin/chats/sidebar", kit.Handler(handlers.HandleAdminChatSidebar))       // New handler for sidebar polling
		app.Get("/admin/chat/{id}/messages", kit.Handler(handlers.HandleAdminChatMessages)) // New handler for message polling
		app.Get("/configuration", kit.Handler(handlers.HandleConfigurationIndex))
		app.Get("/admin/{section}", kit.Handler(handlers.HandleAdminSettings))
		app.Post("/admin/{section}", kit.Handler(handlers.HandleAdminSettingsUpdate))
		app.Post("/admin/notifications/test", kit.Handler(handlers.HandleAdminNotificationsTest))
		app.Post("/admin/notifications/test/telegram", kit.Handler(handlers.HandleAdminTelegramNotificationsTest))
		app.Post("/admin/sections/add", kit.Handler(handlers.HandleAdminSectionAdd))
		app.Post("/admin/sections/{index}/delete", kit.Handler(handlers.HandleAdminSectionDelete))
		app.Post("/admin/sections/{index}/duplicate", kit.Handler(handlers.HandleAdminSectionDuplicate))
		app.Get("/products/new", kit.Handler(handlers.HandleProductNew))
		app.Post("/products", kit.Handler(handlers.HandleProductCreate))
		app.Get("/products/{id}/edit", kit.Handler(handlers.HandleProductEdit))
		app.Put("/products/{id}", kit.Handler(handlers.HandleProductUpdate))
		app.Get("/products/{id}/delete", kit.Handler(handlers.HandleProductDeleteConfirm))
		app.Delete("/products/{id}", kit.Handler(handlers.HandleProductDelete))
		// app.Get("/path", kit.Handler(myHandler.HandleIndex))
	})
}

func adminSetupMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setup" || strings.HasPrefix(r.URL.Path, "/public/") || strings.HasPrefix(r.URL.Path, "/_templ/") {
			next.ServeHTTP(w, r)
			return
		}
		aff := config.AffiliateFromContext(r.Context())
		if aff == nil || aff.PasswordHash == "" {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func plaintextCSRFMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil {
			// Behind a TLS-terminating proxy (nginx, Cloudflare, etc.)
			// r.TLS is nil even though the client-facing connection is HTTPS.
			if r.Header.Get("X-Forwarded-Proto") != "https" {
				r = csrf.PlaintextHTTPRequest(r)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func getCSRFKey() []byte {
	key := os.Getenv("SUPERKIT_SECRET")
	if key == "" {
		key = "fallback-secret-key-change-in-production"
	}
	return []byte(key)
}

// NotFoundHandler that will be called when the requested path could
// not be found.
func NotFoundHandler(kit *kit.Kit) error {
	// Silence warnings for templ live-reload endpoint during development
	if kit.Request.URL.Path == "/_templ/reload" {
		return nil
	}

	slog.Warn("not found", "path", kit.Request.URL.Path)
	services.ReportWarning(kit.Request, "not found: "+kit.Request.URL.Path)
	return kit.Render(errors.Error404())
}

// ErrorHandler that will be called on errors return from application handlers.
func ErrorHandler(kit *kit.Kit, err error) {
	services.ReportError(kit.Request, err)
	slog.Error("internal server error", "err", err.Error(), "path", kit.Request.URL.Path)
	kit.Render(errors.Error500())
}

func PanicRecoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rvr := recover(); rvr != nil {
				services.ReportPanic(r, rvr)
				slog.Error("panic recovered", "err", rvr, "path", r.URL.Path)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
