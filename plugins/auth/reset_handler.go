package auth

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"net/smtp"
	"os"

	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
	"shopTemplate/app/services"

	"github.com/anthdm/superkit/kit"
	"golang.org/x/crypto/bcrypt"
)

var resetChars = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")

func generateResetPassword() (string, error) {
	b := make([]rune, 12)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(resetChars))))
		if err != nil {
			return "", err
		}
		b[i] = resetChars[n.Int64()]
	}
	return string(b), nil
}

func HandleResetPasswordIndex(kit *kit.Kit) error {
	return kit.Render(ResetPasswordPage("", false, ""))
}

func HandleResetPasswordCreate(kit *kit.Kit) error {
	email := kit.Request.FormValue("email")
	if email == "" {
		return kit.Render(ResetPasswordPage("", false, "Email is required."))
	}

	var user models.User
	if err := db.Get().Where("email = ?", email).First(&user).Error; err != nil {
		// Don't reveal if the email exists or not
		return kit.Render(ResetPasswordPage(email, true, ""))
	}

	newPassword, err := generateResetPassword()
	if err != nil {
		slog.Error("failed to generate reset password", "err", err)
		return kit.Render(ResetPasswordPage(email, false, "An error occurred. Please try again."))
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		slog.Error("failed to hash reset password", "err", err)
		return kit.Render(ResetPasswordPage(email, false, "An error occurred. Please try again."))
	}

	if err := db.Get().Model(&user).Update("password_hash", string(hash)).Error; err != nil {
		slog.Error("failed to update password", "err", err)
		return kit.Render(ResetPasswordPage(email, false, "An error occurred. Please try again."))
	}

	// Send email with new password
	cfg := config.Get()
	if apiKey := os.Getenv("BREVO_API_KEY"); apiKey != "" {
		subject := "Password Reset - " + cfg.Site.Name
		body := fmt.Sprintf("Hello,\n\nYour password has been reset as requested.\n\nNew Password: %s\n\nPlease log in and change your password as soon as possible.\n\nThank you,\n%s",
			newPassword, cfg.Site.Name,
		)
		if err := services.SendEmailViaBrevo(email, subject, body); err != nil {
			slog.Error("failed to send reset password email via Brevo", "err", err)
		}
	} else {
		from := os.Getenv("SMTP_FROM")
		host := os.Getenv("SMTP_HOST")
		port := os.Getenv("SMTP_PORT")
		username := os.Getenv("SMTP_USER")
		password := os.Getenv("SMTP_PASS")
		if from != "" && host != "" && port != "" {
			auth := smtp.PlainAuth("", username, password, host)
			header := fmt.Sprintf("From: %s\r\n", from)
			header += fmt.Sprintf("To: %s\r\n", email)
			header += fmt.Sprintf("Subject: Password Reset - %s\r\n", cfg.Site.Name)
			header += "MIME-version: 1.0\r\n"
			header += "Content-Type: text/plain; charset=\"UTF-8\"\r\n\r\n"
			body := fmt.Sprintf(
				"Hello,\n\n"+
					"Your password has been reset as requested.\n\n"+
					"New Password: %s\n\n"+
					"Please log in and change your password as soon as possible.\n\n"+
					"Thank you,\n%s",
				newPassword, cfg.Site.Name,
			)
			if err := smtp.SendMail(host+":"+port, auth, from, []string{email}, []byte(header+body)); err != nil {
				slog.Error("failed to send reset password email", "err", err)
			}
		}
	}

	slog.Info("password reset successful", "email", email)
	return kit.Render(ResetPasswordPage(email, true, ""))
}
