package services

import (
	"log/slog"
	"shopTemplate/app/models"
)

// OrderNotifier defines the contract for any notification service
type OrderNotifier interface {
	Send(order models.Order) error
	SendAbandoned(order models.Order) error
	Name() string
}

// NotificationService manages multiple notification providers
type NotificationService struct {
	providers []OrderNotifier
}

func NewNotificationService() *NotificationService {
	return &NotificationService{
		providers: []OrderNotifier{},
	}
}

// Register adds a new notification provider to the service
func (s *NotificationService) Register(provider OrderNotifier) {
	s.providers = append(s.providers, provider)
}

// NotifyAll sends the order notification through all registered providers
func (s *NotificationService) NotifyAll(order models.Order) {
	for _, provider := range s.providers {
		go func(p OrderNotifier) {
			if err := p.Send(order); err != nil {
				slog.Error("failed to send notification",
					"provider", p.Name(),
					"orderID", order.ID,
					"err", err,
				)
			} else {
				slog.Info("notification sent successfully",
					"provider", p.Name(),
					"orderID", order.ID,
				)
			}
		}(provider)
	}
}

// NotifyAbandoned sends an alert about an abandoned cart to all providers
func (s *NotificationService) NotifyAbandoned(order models.Order) {
	for _, provider := range s.providers {
		go func(p OrderNotifier) {
			if err := p.SendAbandoned(order); err != nil {
				slog.Error("failed to send abandoned notification",
					"provider", p.Name(),
					"orderID", order.ID,
					"err", err,
				)
			}
		}(provider)
	}
}
