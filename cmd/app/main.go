package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"shopTemplate/app"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/services"
	"shopTemplate/public"

	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
)

func main() {
	kit.Setup()

	// Initialize Database explicitly
	if err := db.Connect(); err != nil {
		log.Fatalf("CRITICAL: Failed to connect to database: %v", err)
	}

	// Start the Mes Colis Express socket listener if enabled.
	go func() {
		cfg := config.Get()
		if cfg.Mescolis.Enabled && cfg.Mescolis.APIKey != "" {
			log.Println("starting Mes Colis Express socket listener...")
			socket := services.NewMescolisSocket(cfg.Mescolis.APIKey, services.HandleMescolisEvent)
			socket.Start()
		}
	}()

	router := chi.NewMux()

	app.InitializeMiddleware(router)

	if kit.IsDevelopment() {
		router.Handle("/public/*", disableCache(staticDev()))
	} else if kit.IsProduction() {
		router.Handle("/public/*", disableCache(staticProd()))
	}

	kit.UseErrorHandler(app.ErrorHandler)
	app.InitializeRoutes(router)
	router.NotFound(kit.Handler(app.NotFoundHandler))
	app.RegisterEvents()

	listenAddr := os.Getenv("HTTP_LISTEN_ADDR")
	if listenAddr == "" {
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
