package services

import (
	"errors"
	"shopTemplate/app/models"
)

// PaymentResponse defines the standard result of any payment attempt.
type PaymentResponse struct {
	Success       bool
	RedirectURL   string // Used for online gateways like Flouci
	TransactionID string
	Message       string
}

// PaymentProvider is the interface that every payment method must implement.
type PaymentProvider interface {
	ID() string          // e.g., "cod", "flouci"
	DisplayName() string // e.g., "Cash on Delivery"
	// Process handles the actual logic.
	// Note: models.Order is assumed to exist in your models package.
	Process(order *models.Order) (*PaymentResponse, error)
}

// PaymentRegistry manages the available providers.
type PaymentRegistry struct {
	providers map[string]PaymentProvider
}

func NewPaymentRegistry() *PaymentRegistry {
	return &PaymentRegistry{
		providers: make(map[string]PaymentProvider),
	}
}

func (r *PaymentRegistry) Register(provider PaymentProvider) {
	r.providers[provider.ID()] = provider
}

func (r *PaymentRegistry) Get(id string) (PaymentProvider, error) {
	p, ok := r.providers[id]
	if !ok {
		return nil, errors.New("payment provider not found")
	}
	return p, nil
}

// Example COD implementation
type CODProvider struct{}

func (p CODProvider) ID() string          { return "cod" }
func (p CODProvider) DisplayName() string { return "Paiement à la livraison" }
func (p CODProvider) Process(order *models.Order) (*PaymentResponse, error) {
	return &PaymentResponse{Success: true, Message: "Order confirmed for COD"}, nil
}
