package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"shopTemplate/app/helpers"
	"shopTemplate/app/models"
	"shopTemplate/app/views/legal"

	"github.com/anthdm/superkit/kit"
)

type KeyFunc func(r *http.Request) string

type RateLimiter struct {
	mu       sync.Mutex
	visits   map[string]int
	limit    int
	window   time.Duration
	lastSeen map[string]time.Time
	keyFunc  KeyFunc
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		visits:   make(map[string]int),
		limit:    limit,
		window:   window,
		lastSeen: make(map[string]time.Time),
	}
}

func (rl *RateLimiter) WithKeyFunc(fn KeyFunc) *RateLimiter {
	rl.keyFunc = fn
	return rl
}

func IPKeyFunc(r *http.Request) string {
	return r.RemoteAddr
}

func UserKeyFunc(r *http.Request) string {
	authVal := r.Context().Value(kit.AuthKey{})
	if user, ok := authVal.(models.AuthUser); ok && user.Check() {
		return fmt.Sprintf("user:%d", user.ID)
	}
	return r.RemoteAddr
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if last, ok := rl.lastSeen[key]; ok {
		if now.Sub(last) > rl.window {
			rl.visits[key] = 0
		}
	}
	rl.lastSeen[key] = now
	rl.visits[key]++
	return rl.visits[key] <= rl.limit
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.RemoteAddr
		if rl.keyFunc != nil {
			key = rl.keyFunc(r)
		}
		if !rl.Allow(key) {
			w.Header().Set("Retry-After", strconv.Itoa(int(rl.window.Seconds())))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func HandleHealthCheck(kit *kit.Kit) error {
	return kit.Text(http.StatusOK, "OK")
}

var (
	RateLimitCheckout = NewRateLimiter(5, time.Minute).WithKeyFunc(UserKeyFunc)
	RateLimitLogin    = NewRateLimiter(5, time.Minute)
	RateLimitChat     = NewRateLimiter(10, time.Minute).WithKeyFunc(UserKeyFunc)
	RateLimitCart     = NewRateLimiter(20, time.Minute).WithKeyFunc(UserKeyFunc)
)

func HandlePrivacyPolicy(kit *kit.Kit) error {
	user, _ := kit.Auth().(models.AuthUser)
	categories := helpers.GetCategoryTree()
	cart := helpers.GetCart(kit)
	return kit.Render(legal.PrivacyPolicy(user, categories, cart.Total))
}

func HandleDataDeletion(kit *kit.Kit) error {
	user, _ := kit.Auth().(models.AuthUser)
	categories := helpers.GetCategoryTree()
	cart := helpers.GetCart(kit)
	return kit.Render(legal.DataDeletion(user, categories, cart.Total))
}
