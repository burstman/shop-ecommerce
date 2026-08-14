package auth

import (
	"github.com/anthdm/superkit/kit"
	"github.com/go-chi/chi/v5"
)

func InitializeRoutes(router chi.Router) {
	authConfig := kit.AuthenticationConfig{
		AuthFunc:    AuthenticateUser,
		RedirectURL: "/login",
	}

	router.Get("/email/verify", kit.Handler(HandleEmailVerify))
	router.Post("/resend-email-verification", kit.Handler(HandleResendVerificationCode))

	router.Get("/reset-password", kit.Handler(HandleResetPasswordIndex))
	router.Post("/reset-password", kit.Handler(HandleResetPasswordCreate))

	router.Group(func(auth chi.Router) {
		auth.Use(kit.WithAuthentication(authConfig, false))
		auth.Get("/login", kit.Handler(HandleLoginIndex))
		auth.Post("/login", kit.Handler(HandleLoginCreate))
		auth.Delete("/logout", kit.Handler(HandleLoginDelete))

		auth.Get("/auth/google", kit.Handler(HandleGoogleLogin))
		auth.Get("/auth/google/callback", kit.Handler(HandleGoogleCallback))
		auth.Get("/auth/facebook", kit.Handler(HandleFacebookLogin))
		auth.Get("/auth/facebook/callback", kit.Handler(HandleFacebookCallback))

		auth.Get("/signup", kit.Handler(HandleSignupIndex))
		auth.Post("/signup", kit.Handler(HandleSignupCreate))

	})

	router.Group(func(auth chi.Router) {
		auth.Use(kit.WithAuthentication(authConfig, true))
		auth.Get("/profile", kit.Handler(HandleProfileShow))
		auth.Put("/profile", kit.Handler(HandleProfileUpdate))
	})
}
