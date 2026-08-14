package auth

import (
	"database/sql"
	"shopTemplate/app/db"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Event name constants
const (
	UserSignupEvent         = "auth.signup"
	ResendVerificationEvent = "auth.resend.verification"
)

// UserWithVerificationToken is a struct that will be sent over the
// auth.signup event. It holds the User struct and the Verification token string.
type UserWithVerificationToken struct {
	User  User
	Token string
}

type Auth struct {
	UserID   uint
	Email    string
	Role     string
	LoggedIn bool
}

func (auth Auth) Check() bool {
	return auth.LoggedIn
}

// func (auth Auth) CheckUserRole() string {
// 	return auth.Role
// }

type User struct {
	gorm.Model

	Email           string
	FirstName       string
	LastName        string
	Role            string
	PasswordHash    string
	EmailVerifiedAt sql.NullTime
	AffiliateID     string
}

func createUserFromFormValues(values SignupFormValues, affiliateID string) (User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(values.Password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	user := User{
		Email:        values.Email,
		FirstName:    values.FirstName,
		LastName:     values.LastName,
		PasswordHash: string(hash),
		Role:         "customer",
		AffiliateID:  affiliateID,
	}
	result := db.Get().Create(&user)
	return user, result.Error
}

type Session struct {
	gorm.Model

	UserID    uint
	Token     string
	IPAddress string
	UserAgent string
	ExpiresAt time.Time
	User      User
}
