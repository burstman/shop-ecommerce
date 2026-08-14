package services

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"shopTemplate/app/config"
	"shopTemplate/app/db"
	"shopTemplate/app/models"
)

type errorPayload struct {
	Error       string `json:"error"`
	Path        string `json:"path,omitempty"`
	Method      string `json:"method,omitempty"`
	Host        string `json:"host,omitempty"`
	AffiliateID string `json:"affiliate_id,omitempty"`
	Stack       string `json:"stack,omitempty"`
	Timestamp   string `json:"timestamp"`
}

func sendError(r *http.Request, msg string) {
	aff := config.AffiliateFromContext(r.Context())
	if aff == nil {
		return
	}
	dashboardURL, err := db.GetConfig("dashboard_url")
	if err != nil || dashboardURL == "" {
		return
	}

	payload := errorPayload{
		Error:     msg,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stack:     string(debug.Stack()),
	}

	if r != nil {
		payload.Path = r.URL.Path
		payload.Method = r.Method
		payload.Host = r.Host
		payload.AffiliateID = aff.AffiliateID
	}

	go func() {
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", dashboardURL+"/api/error", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if aff.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+aff.APIKey)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
}

type warnPayload struct {
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Method  string `json:"method,omitempty"`
	Host    string `json:"host,omitempty"`
}

func ReportError(r *http.Request, err error) {
	sendError(r, err.Error())
}

func ReportPanic(r *http.Request, rvr any) {
	msg := "unknown panic"
	switch v := rvr.(type) {
	case error:
		msg = v.Error()
	case string:
		msg = v
	}
	sendError(r, msg)
}

func sendWarning(aff *models.Affiliate, message string) {
	if aff == nil {
		return
	}
	dashboardURL, err := db.GetConfig("dashboard_url")
	if err != nil || dashboardURL == "" {
		return
	}

	payload := warnPayload{Message: message}
	url := dashboardURL + "/api/warn"
	slog.Info("sending warning to dashboard", "url", url, "message", message)

	go func() {
		body, _ := json.Marshal(payload)
		req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if aff.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+aff.APIKey)
		}

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
		}
	}()
}

func ReportWarning(r *http.Request, message string) {
	aff := config.AffiliateFromContext(r.Context())
	sendWarning(aff, message)
}

func ReportWarningAffiliate(aff *models.Affiliate, message string) {
	sendWarning(aff, message)
}
