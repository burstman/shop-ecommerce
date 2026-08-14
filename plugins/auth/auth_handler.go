package auth

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
	"strconv"
	"time"

	"github.com/anthdm/superkit/kit"
	v "github.com/anthdm/superkit/validate"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"
	"golang.org/x/oauth2/google"
	"gorm.io/gorm"
)

const (
	userSessionName = "user-session"
)

var authSchema = v.Schema{
	"email":    v.Rules(v.Email),
	"password": v.Rules(v.Required),
}

func HandleLoginIndex(kit *kit.Kit) error {
	if kit.Auth().Check() {
		redirectURL := kit.Getenv("SUPERKIT_AUTH_REDIRECT_AFTER_LOGIN", "/profile")
		return kit.Redirect(http.StatusSeeOther, redirectURL)
	}
	categories := helpers.GetCategoryTree()
	cart := helpers.GetCart(kit)
	user := models.AuthUser{}
	if u, ok := kit.Auth().(models.AuthUser); ok {
		user = u
	}
	data := LoginIndexPageData{
		AuthError: kit.Request.URL.Query().Get("error"),
	}
	return kit.Render(LoginIndex(user, data, categories, cart.Total))
}

func HandleLoginCreate(kit *kit.Kit) error {
	var values LoginFormValues
	errors, ok := v.Request(kit.Request, &values, authSchema)
	if !ok {
		return kit.Render(LoginForm(values, errors, ""))
	}

	var user User
	err := db.Get().Find(&user, "email = ?", values.Email).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			errors.Add("credentials", services.GetI18n().T(kit.Request.Context(), "invalid_credentials"))
			return kit.Render(LoginForm(values, errors, ""))
		}
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(values.Password))
	if err != nil {
		errors.Add("credentials", services.GetI18n().T(kit.Request.Context(), "invalid_credentials"))
		return kit.Render(LoginForm(values, errors, ""))
	}

	skipVerify := kit.Getenv("SUPERKIT_AUTH_SKIP_VERIFY", "false")
	if skipVerify != "true" {
		if !user.EmailVerifiedAt.Valid {
			errors.Add("verified", services.GetI18n().T(kit.Request.Context(), "verify_email_desc"))
			return kit.Render(LoginForm(values, errors, ""))
		}
	}

	sessionExpiryStr := kit.Getenv("SUPERKIT_AUTH_SESSION_EXPIRY_IN_HOURS", "48")
	sessionExpiry, err := strconv.Atoi(sessionExpiryStr)
	if err != nil {
		sessionExpiry = 48
	}
	session := Session{
		UserID:    user.ID,
		Token:     uuid.New().String(),
		ExpiresAt: time.Now().Add(time.Hour * time.Duration(sessionExpiry)),
	}
	if err = db.Get().Create(&session).Error; err != nil {
		return err
	}

	sess := kit.GetSession(userSessionName)
	sess.Values["sessionToken"] = session.Token
	sess.Save(kit.Request, kit.Response)
	redirectURL := kit.Getenv("SUPERKIT_AUTH_REDIRECT_AFTER_LOGIN", "/profile")

	return kit.Redirect(http.StatusSeeOther, redirectURL)
}

func HandleLoginDelete(kit *kit.Kit) error {
	sess := kit.GetSession(userSessionName)
	defer func() {
		sess.Values = map[any]any{}
		sess.Save(kit.Request, kit.Response)
	}()
	err := db.Get().Delete(&Session{}, "token = ?", sess.Values["sessionToken"]).Error
	if err != nil {
		return err
	}
	kit.Response.Header().Set("HX-Redirect", "/")
	return nil
}

func HandleEmailVerify(kit *kit.Kit) error {
	tokenStr := kit.Request.URL.Query().Get("token")
	if len(tokenStr) == 0 {
		return kit.Render(EmailVerificationError(services.GetI18n().T(kit.Request.Context(), "invalid_verification_token")))
	}

	token, err := jwt.ParseWithClaims(
		tokenStr, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
			return []byte(os.Getenv("SUPERKIT_SECRET")), nil
		}, jwt.WithLeeway(5*time.Second))
	if err != nil {
		return kit.Render(EmailVerificationError(services.GetI18n().T(kit.Request.Context(), "invalid_verification_token")))
	}
	if !token.Valid {
		return kit.Render(EmailVerificationError(services.GetI18n().T(kit.Request.Context(), "invalid_verification_token")))
	}

	claims, ok := token.Claims.(*jwt.RegisteredClaims)
	if !ok {
		return kit.Render(EmailVerificationError(services.GetI18n().T(kit.Request.Context(), "invalid_verification_token")))
	}
	if claims.ExpiresAt.Time.Before(time.Now()) {
		return kit.Render(EmailVerificationError(services.GetI18n().T(kit.Request.Context(), "verification_token_expired")))
	}

	userID, err := strconv.Atoi(claims.Subject)
	if err != nil {
		return kit.Render(EmailVerificationError(services.GetI18n().T(kit.Request.Context(), "verification_token_expired")))
	}

	var user User
	err = db.Get().First(&user, userID).Error
	if err != nil {
		return err
	}

	if user.EmailVerifiedAt.Time.After(time.Time{}) {
		return kit.Render(EmailVerificationError(services.GetI18n().T(kit.Request.Context(), "email_already_verified")))
	}

	now := sql.NullTime{Time: time.Now(), Valid: true}
	user.EmailVerifiedAt = now
	err = db.Get().Save(&user).Error
	if err != nil {
		return err
	}

	return kit.Redirect(http.StatusSeeOther, "/login")
}

func AuthenticateUser(kit *kit.Kit) (kit.Auth, error) {
	auth := models.AuthUser{}
	sess := kit.GetSession(userSessionName)
	token, ok := sess.Values["sessionToken"]
	if !ok {
		return auth, nil
	}

	var session Session
	err := db.Get().
		Preload("User").
		Find(&session, "token = ? AND expires_at > ?", token, time.Now()).Error
	if err != nil || session.ID == 0 {
		return auth, nil
	}

	return models.AuthUser{
		LoggedIn:  true,
		ID:        session.User.ID,
		Email:     session.User.Email,
		Role:      session.User.Role,
		FirstName: session.User.FirstName,
		LastName:  session.User.LastName,
	}, nil
}

func HandleGoogleLogin(kit *kit.Kit) error {
	state := uuid.New().String()
	sess := kit.GetSession(userSessionName)
	sess.Values["oauthState"] = state
	sess.Save(kit.Request, kit.Response)

	config := getGoogleConfig(kit)
	url := config.AuthCodeURL(state)
	return kit.Redirect(http.StatusTemporaryRedirect, url)
}

func HandleGoogleCallback(kit *kit.Kit) error {
	sess := kit.GetSession(userSessionName)
	state, ok := sess.Values["oauthState"].(string)
	if !ok || state != kit.Request.FormValue("state") {
		return kit.Text(http.StatusBadRequest, services.GetI18n().T(kit.Request.Context(), "invalid_oauth_state"))
	}

	if err := kit.Request.FormValue("error"); err != "" {
		return kit.Redirect(http.StatusSeeOther, "/login?error=auth_cancelled")
	}

	config := getGoogleConfig(kit)
	tok, err := config.Exchange(kit.Request.Context(), kit.Request.FormValue("code"))
	if err != nil {
		return kit.Redirect(http.StatusSeeOther, "/login?error=auth_failed")
	}

	client := config.Client(kit.Request.Context(), tok)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var gUser struct {
		Email     string `json:"email"`
		FirstName string `json:"given_name"`
		LastName  string `json:"family_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gUser); err != nil {
		return err
	}

	return handleSocialLogin(kit, gUser.Email, gUser.FirstName, gUser.LastName)
}

func HandleFacebookLogin(kit *kit.Kit) error {
	state := uuid.New().String()
	sess := kit.GetSession(userSessionName)
	sess.Values["oauthState"] = state
	sess.Save(kit.Request, kit.Response)

	config := getFacebookConfig(kit)
	url := config.AuthCodeURL(state)
	return kit.Redirect(http.StatusTemporaryRedirect, url)
}

func HandleFacebookCallback(kit *kit.Kit) error {
	sess := kit.GetSession(userSessionName)
	state, ok := sess.Values["oauthState"].(string)
	if !ok || state != kit.Request.FormValue("state") {
		return kit.Text(http.StatusBadRequest, services.GetI18n().T(kit.Request.Context(), "invalid_oauth_state"))
	}

	if err := kit.Request.FormValue("error"); err != "" {
		return kit.Redirect(http.StatusSeeOther, "/login?error=auth_cancelled")
	}

	config := getFacebookConfig(kit)
	tok, err := config.Exchange(kit.Request.Context(), kit.Request.FormValue("code"))
	if err != nil {
		return kit.Redirect(http.StatusSeeOther, "/login?error=auth_failed")
	}

	client := config.Client(kit.Request.Context(), tok)
	resp, err := client.Get("https://graph.facebook.com/me?fields=first_name,last_name,email")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var fUser struct {
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fUser); err != nil {
		return err
	}

	return handleSocialLogin(kit, fUser.Email, fUser.FirstName, fUser.LastName)
}

func handleSocialLogin(kit *kit.Kit, email, firstName, lastName string) error {
	var user User
	err := db.Get().First(&user, "email = ?", email).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	if err == gorm.ErrRecordNotFound {
		affID := config.AffiliateIDFromContext(kit.Request.Context())
		user = User{
			Email:           email,
			FirstName:       firstName,
			LastName:        lastName,
			Role:            "customer",
			EmailVerifiedAt: sql.NullTime{Time: time.Now(), Valid: true},
			AffiliateID:     affID,
		}
		if err := db.Get().Create(&user).Error; err != nil {
			return err
		}
	}

	sessionExpiryStr := kit.Getenv("SUPERKIT_AUTH_SESSION_EXPIRY_IN_HOURS", "48")
	sessionExpiry, err := strconv.Atoi(sessionExpiryStr)
	if err != nil {
		sessionExpiry = 48
	}
	if sessionExpiry == 0 {
		sessionExpiry = 48
	}
	session := Session{
		UserID:    user.ID,
		Token:     uuid.New().String(),
		ExpiresAt: time.Now().Add(time.Hour * time.Duration(sessionExpiry)),
	}
	if err = db.Get().Create(&session).Error; err != nil {
		return err
	}

	sess := kit.GetSession(userSessionName)
	sess.Values["sessionToken"] = session.Token
	sess.Save(kit.Request, kit.Response)
	redirectURL := kit.Getenv("SUPERKIT_AUTH_REDIRECT_AFTER_LOGIN", "/profile")

	return kit.Redirect(http.StatusSeeOther, redirectURL)
}

func getGoogleConfig(kit *kit.Kit) *oauth2.Config {
	scheme := "https"
	if kit.Request.TLS == nil {
		scheme = "http"
	}
	redirectURL := fmt.Sprintf("%s://%s/auth/google/callback", scheme, kit.Request.Host)
	return &oauth2.Config{
		ClientID:     kit.Getenv("GOOGLE_CLIENT_ID", ""),
		ClientSecret: kit.Getenv("GOOGLE_CLIENT_SECRET", ""),
		RedirectURL:  redirectURL,
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint:     google.Endpoint,
	}
}

func getFacebookConfig(kit *kit.Kit) *oauth2.Config {
	scheme := "https"
	if kit.Request.TLS == nil {
		scheme = "http"
	}
	redirectURL := fmt.Sprintf("%s://%s/auth/facebook/callback", scheme, kit.Request.Host)
	return &oauth2.Config{
		ClientID:     kit.Getenv("FACEBOOK_CLIENT_ID", ""),
		ClientSecret: kit.Getenv("FACEBOOK_CLIENT_SECRET", ""),
		RedirectURL:  redirectURL,
		Scopes:       []string{"email", "public_profile"},
		Endpoint:     facebook.Endpoint,
	}
}
