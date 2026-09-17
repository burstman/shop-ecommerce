package main

import (
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"shopTemplate/app"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
	"shopTemplate/public"
	"time"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
)

func main() {
	kit.Setup()

	// Initialize Database explicitly
	if err := db.Connect(); err != nil {
		log.Fatalf("CRITICAL: Failed to connect to database: %v", err)
	}

	// Start a Mes Colis Express socket listener for every affiliate (shop)
	// that has Mes Colis enabled. Credentials are per-shop (app_config:AFF-xxx),
	// so the socket must be authenticated with each shop's API key to receive
	// that shop's parcel events.
	go func() {
		var affiliates []models.Affiliate
		if err := db.Get().Find(&affiliates).Error; err != nil {
			slog.Error("failed to load affiliates for mes colis sockets", "err", err)
			return
		}
		started := 0
		for _, aff := range affiliates {
			cfg := config.LoadByAffiliateID(aff.AffiliateID)
			if !cfg.Mescolis.Enabled || cfg.Mescolis.APIKey == "" {
				continue
			}
			log.Printf("starting Mes Colis Express socket listener for shop %s...", aff.AffiliateID)
			socket := services.NewMescolisSocket(cfg.Mescolis.APIKey, services.HandleMescolisEvent)
			go socket.Start()
			started++
		}
		if started == 0 {
			slog.Info("no Mes Colis sockets started (no affiliate with mescolis enabled)")
		}
	}()

	// Flush deferred in-transit WhatsApp notifications at/after 10:00 Tunisia
	// time. Runs regardless of socket state so queued orders still get sent.
	go func() {
		slog.Info("starting in-transit notification scheduler (10:00 Tunisia time)")
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			services.SendPendingInTransitNotifications()
		}
	}()

	router := chi.NewMux()

	app.InitializeMiddleware(router)

	if kit.IsDevelopment() {
		router.Handle("/public/*", disableCache(staticDev()))
	} else if kit.IsProduction() {
		router.Handle("/public/*", staticProd())
	}

	kit.UseErrorHandler(app.ErrorHandler)
	app.InitializeRoutes(router)
	router.NotFound(kit.Handler(app.NotFoundHandler))
	app.RegisterEvents()

	// Render (and most PaaS) inject the port to bind via PORT. Prefer it,
	// then fall back to HTTP_LISTEN_ADDR, then the local default.
	listenAddr := os.Getenv("HTTP_LISTEN_ADDR")
	if port := os.Getenv("PORT"); port != "" {
		listenAddr = ":" + port
	} else if listenAddr == "" {
		listenAddr = ":3000"
	}

	// The URL to display to the user. In development, this is the Templ proxy.
	// In production, it's the application's direct address.
	displayURL := "http://localhost:7331"
	if kit.IsProduction() {
		// For production, display the actual address the server is binding to.
		// If listenAddr is ":3000", it binds to 0.0.0.0:3000, so localhost is fine for display.
		displayURL = fmt.Sprintf("http://localhost%s", listenAddr)
	}

	fmt.Printf("application running in %s\n", kit.Env())
	fmt.Printf("backend listening on: %s\n", listenAddr)
	fmt.Printf("access application via: %s\n", displayURL)

	log.Fatal(http.ListenAndServe(listenAddr, router))
}

func staticDev() http.Handler {
	return http.StripPrefix("/public/", http.FileServerFS(os.DirFS("public")))
}

func staticProd() http.Handler {
	return http.StripPrefix("/public/", http.FileServerFS(public.AssetsFS))
}

func disableCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func init() {
	// godotenv.Load is now handled inside db.Connect()
	// to ensure consistent behavior across entry points.
}
