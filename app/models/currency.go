package models

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strconv"
)

// Currency represents a monetary value stored as an integer of minor units (e.g., cents).
// This avoids floating-point precision issues during arithmetic.
type Currency int64

const (
	// Scale defines the number of decimal places.
	// Your database currently uses numeric(12, 2), so we use a scale of 2 (multiplier 100).
	// If you want to support TND millimes properly, change this to 3 and multiplier to 1000.
	Scale      = 2
	multiplier = 100
)

// NewCurrency creates a Currency from a float64.
func NewCurrency(val float64) Currency {
	return Currency(math.Round(val * multiplier))
}

// ParseCurrency parses a string value (e.g., from a web form) into a Currency type.
func ParseCurrency(s string) (Currency, error) {
	if s == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return NewCurrency(f), nil
}

// Multiply multiplies the currency value by a quantity.
func (c Currency) Multiply(q int) Currency {
	return c * Currency(q)
}

// Divide divides the currency value by a divisor with proper rounding.
func (c Currency) Divide(d int) Currency {
	if d == 0 {
		return 0
	}
	return Currency(math.Round(float64(c) / float64(d)))
}

// ToFloat converts the internal integer representation back to a float64.
func (c Currency) ToFloat() float64 {
	return float64(c) / multiplier
}

// String returns the decimal representation (e.g., "29.99").
func (c Currency) String() string {
	return fmt.Sprintf("%.*f", Scale, c.ToFloat())
}

// Format returns the value with a currency symbol (e.g., "29.99 TND").
func (c Currency) Format(symbol string) string {
	return fmt.Sprintf("%s %s", c.String(), symbol)
}

// Scan implements the sql.Scanner interface for GORM compatibility.
func (c *Currency) Scan(value any) error {
	if value == nil {
		*c = 0
		return nil
	}

	var f float64
	switch v := value.(type) {
	case []byte:
		f, _ = strconv.ParseFloat(string(v), 64)
	case string:
		f, _ = strconv.ParseFloat(v, 64)
	case float64:
		f = v
	default:
		return fmt.Errorf("unsupported type for Currency scan: %T", value)
	}
	*c = NewCurrency(f)
	return nil
}

// Value implements the driver.Valuer interface for GORM compatibility.
func (c Currency) Value() (driver.Value, error) {
	return c.ToFloat(), nil
}
