package models

import (
	"database/sql"

	"gorm.io/gorm"
)

// AuthUser represents an user that might be authenticated.
type AuthUser struct {
	ID        uint
	Email     string
	LoggedIn  bool
	FirstName string
	LastName  string
	Role      string
}

// Check should return true if the user is authenticated.
// See handlers/auth.go.
func (user AuthUser) Check() bool {
	return user.ID > 0 && user.LoggedIn
}

func (user AuthUser) checkRole() string {
	return user.Role
}

type User struct {
	gorm.Model
	Email           string `gorm:"unique"`
	FirstName       string
	LastName        string
	Role            string
	PasswordHash    string
	EmailVerifiedAt sql.NullTime
	Password        string `gorm:"-"`
	AffiliateID     string `gorm:"size:20;index"`
}
