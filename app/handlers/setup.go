package handlers

import (
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"
	"shopTemplate/app/views/admin"
	"strconv"
	"strings"
	"time"

	"github.com/anthdm/superkit/kit"
	"golang.org/x/crypto/bcrypt"
)

var randomChars = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

func randomString(length int) (string, error) {
	b := make([]rune, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(randomChars))))
		if err != nil {
			return "", err
		}
		b[i] = randomChars[n.Int64()]
	}
	return string(b), nil
}

var generateSetupPassword = randomString

func generateAffiliateID() (string, error) {
	var maxID *string
	err := db.Get().Model(&models.Affiliate{}).Select("MAX(affiliate_id)").Scan(&maxID).Error
	if err != nil {
		return "", err
	}
	if maxID == nil || *maxID == "" {
		return "AFF-001", nil
	}
	parts := strings.SplitN(*maxID, "-", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid affiliate id format: %s", *maxID)
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid affiliate id number: %s", parts[1])
	}
	return fmt.Sprintf("AFF-%03d", num+1), nil
}

func sendAdminPasswordEmail(to, password, siteName string) error {
	if apiKey := os.Getenv("BREVO_API_KEY"); apiKey != "" {
		subject := "Admin Account Created - " + siteName
		body := fmt.Sprintf("Hello,\n\nYour admin account has been created for %s.\n\nEmail: %s\nPassword: %s\n\nPlease log in and change your password as soon as possible.\n\nThank you,\n%s",
			siteName, to, password, siteName,
		)
		return services.SendEmailViaBrevo(to, subject, body)
	}

	from := os.Getenv("SMTP_FROM")
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")
	username := os.Getenv("SMTP_USER")
	smtpPass := os.Getenv("SMTP_PASS")
	if from == "" || host == "" || port == "" {
		return fmt.Errorf("SMTP not configured: set SMTP_FROM, SMTP_HOST, SMTP_PORT")
	}

	slog.Info("sending admin password email", "host", host, "port", port, "from", from, "to", to, "user", username)

	ips, lookupErr := net.LookupHost(host)
	if lookupErr != nil {
		slog.Error("SMTP host DNS lookup failed", "host", host, "err", lookupErr)
	} else {
		slog.Info("SMTP host resolved", "host", host, "ips", ips)
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Admin Account Created - %s\r\nMIME-version: 1.0\r\nContent-Type: text/plain; charset=\"UTF-8\"\r\n\r\nHello,\n\nYour admin account has been created for %s.\n\nEmail: %s\nPassword: %s\n\nPlease log in and change your password as soon as possible.\n\nThank you,\n%s",
		from, to, siteName, siteName, to, password, siteName,
	)

	addr := host + ":" + port

	done := make(chan error, 1)
	go func() {
		conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
		if err != nil {
			done <- fmt.Errorf("TCP connection failed: %w", err)
			return
		}
		conn.Close()

		if port == "465" {
			tlsConn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", addr, &tls.Config{ServerName: host})
			if err != nil {
				done <- fmt.Errorf("TLS connection failed: %w", err)
				return
			}
			client, err := smtp.NewClient(tlsConn, host)
			if err != nil {
				done <- fmt.Errorf("SMTP client failed: %w", err)
				return
			}
			defer client.Close()
			auth := smtp.PlainAuth("", username, smtpPass, host)
			if err := client.Auth(auth); err != nil {
				done <- fmt.Errorf("SMTP auth failed: %w", err)
				return
			}
			if err := client.Mail(from); err != nil {
				done <- fmt.Errorf("SMTP MAIL FROM failed: %w", err)
				return
			}
			if err := client.Rcpt(to); err != nil {
				done <- fmt.Errorf("SMTP RCPT TO failed: %w", err)
				return
			}
			w, err := client.Data()
			if err != nil {
				done <- fmt.Errorf("SMTP DATA failed: %w", err)
				return
			}
			_, err = w.Write([]byte(msg))
			if err != nil {
				done <- fmt.Errorf("SMTP write failed: %w", err)
				return
			}
			err = w.Close()
			if err != nil {
				done <- fmt.Errorf("SMTP close failed: %w", err)
				return
			}
			done <- nil
		} else {
			auth := smtp.PlainAuth("", username, smtpPass, host)
			done <- smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
		}
	}()

	select {
	case err := <-done:
		if err != nil {
			slog.Error("failed to send admin password email", "host", host, "port", port, "err", err)
			return fmt.Errorf("failed to send email: %w", err)
		}
		return nil
	case <-time.After(15 * time.Second):
		return fmt.Errorf("email send timed out after 15s")
	}
}

func HandleSetupIndex(kit *kit.Kit) error {
	aff := config.AffiliateFromContext(kit.Request.Context())
	slog.Info("setup index", "host", kit.Request.Host, "affiliate_found", aff != nil, "has_password", aff != nil && aff.PasswordHash != "")
	if aff != nil && aff.PasswordHash != "" {
		return kit.Redirect(http.StatusSeeOther, "/login")
	}
	return kit.Render(admin.SetupPage("", "", "", "", false))
}

func HandleSetupCreate(kit *kit.Kit) error {
	aff := config.AffiliateFromContext(kit.Request.Context())
	slog.Info("setup create", "host", kit.Request.Host, "affiliate_found", aff != nil, "affiliate_id", func() string { if aff != nil { return aff.AffiliateID }; return "" }())

	name := kit.Request.FormValue("name")
	email := kit.Request.FormValue("email")
	slog.Info("setup create form", "name", name, "email", email)

	if name == "" || email == "" {
		slog.Warn("setup create validation failed", "reason", "missing fields")
		return kit.Render(admin.SetupPage("", "", "", "All fields are required.", false))
	}

	// Check if email is already used by another affiliate
	var existingAff models.Affiliate
	excludeID := ""
	if aff != nil {
		excludeID = aff.AffiliateID
	}
	if err := db.Get().Where("email = ? AND affiliate_id != ?", email, excludeID).First(&existingAff).Error; err == nil {
		slog.Warn("setup create email conflict", "email", email, "existing_id", existingAff.AffiliateID)
		return kit.Render(admin.SetupPage("", "", "", "This email is already registered to another shop. Each shop must use a unique email.", false))
	}

	// Check if the email is authorized
	var authorizedEmails []string
	db.Get().Model(&models.Affiliate{}).Where("authorized_email <> ''").Pluck("authorized_email", &authorizedEmails)
	if len(authorizedEmails) > 0 {
		slog.Info("setup create authorized check", "authorized_list", authorizedEmails)
		authorized := false
		for _, ae := range authorizedEmails {
			if ae == email {
				authorized = true
				break
			}
		}
		if !authorized {
			slog.Warn("setup create unauthorized email", "email", email)
			return kit.Render(admin.SetupPage("", "", "", "Your email is not authorized for setup. Contact the shop owner.", false))
		}
	}

	scheme := "https"
	if kit.Request.TLS == nil {
		scheme = "http"
	}
	shopURL := fmt.Sprintf("%s://%s", scheme, kit.Request.Host)
	slog.Info("setup create shop_url", "shop_url", shopURL)

	password, err := generateSetupPassword(12)
	if err != nil {
		slog.Error("setup create password generation failed", "err", err)
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("setup create bcrypt failed", "err", err)
		return err
	}

	// Update or create affiliate for this host
	if aff == nil {
		affiliateID, err := generateAffiliateID()
		if err != nil {
			slog.Error("setup create affiliate ID generation failed", "err", err)
			return kit.Render(admin.SetupPage("", "", "", "Failed to generate affiliate ID: "+err.Error(), false))
		}
		aff = &models.Affiliate{
			AffiliateID:  affiliateID,
			Name:         name,
			Email:        email,
			PasswordHash: string(hash),
			Rate:         0,
			Active:       true,
			ShopURL:      shopURL,
		}
		slog.Info("setup create creating new affiliate", "affiliate_id", affiliateID, "shop_url", shopURL)
		if err := db.Get().Create(aff).Error; err != nil {
			slog.Error("setup create affiliate creation failed", "err", err)
			return kit.Render(admin.SetupPage("", "", "", "Failed to create affiliate: "+err.Error(), false))
		}
	} else {
		slog.Info("setup create updating existing affiliate", "affiliate_id", aff.AffiliateID)
		db.Get().Model(aff).Updates(map[string]interface{}{
			"name":          name,
			"email":         email,
			"password_hash": string(hash),
		})
	}

	// Create or reuse user for this shop
	var existing models.User
	if err := db.Get().Where("email = ?", email).First(&existing).Error; err != nil {
		user := models.User{
			Email:           email,
			FirstName:       name,
			PasswordHash:    string(hash),
			Role:            "admin",
			EmailVerifiedAt: sql.NullTime{Time: time.Now(), Valid: true},
			AffiliateID:     aff.AffiliateID,
		}
		slog.Info("setup create creating new user", "email", email)
		if err := db.Get().Create(&user).Error; err != nil {
			slog.Error("setup create user creation failed", "err", err)
			return kit.Render(admin.SetupPage("", "", "", "Failed to create admin: "+err.Error(), false))
		}
	} else {
		slog.Info("setup create updating existing user", "email", email, "user_id", existing.ID)
		db.Get().Model(&existing).Update("password_hash", string(hash))
	}

	cfg := config.Get()
	cfg.Site.AffiliateID = aff.AffiliateID
	slog.Info("setup create saving config", "affiliate_id", aff.AffiliateID)
	if err := config.Save(cfg); err != nil {
		slog.Error("setup create config save failed", "err", err)
		return kit.Render(admin.SetupPage("", "", "", "Failed to save affiliate ID to config: "+err.Error(), false))
	}

	slog.Info("setup create password", "email", email, "password", password)

	// Send password via email (required)
	if err := sendAdminPasswordEmail(email, password, cfg.Site.Name); err != nil {
		slog.Error("setup create email failed", "err", err)
		return kit.Render(admin.SetupPage(email, password, shopURL, "", false))
	}
	slog.Info("setup create complete", "email", email)

	return kit.Render(admin.SetupPage(email, "", shopURL, "", true))
}
