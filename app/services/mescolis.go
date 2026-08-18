package services

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

const mescolisBaseURL = "https://api.mescolis.tn/api"

// MescolisClient talks to the Mes Colis Express shipping API.
// All requests require the x-access-token header supplied by Mes Colis Express.
type MescolisClient struct {
	APIKey          string
	AllowSubAccount bool
	AccountCode     string
	httpClient      *http.Client
}

func NewMescolisClient(apiKey string, allowSubAccount bool, accountCode string) *MescolisClient {
	return &MescolisClient{
		APIKey:          apiKey,
		AllowSubAccount: allowSubAccount,
		AccountCode:     accountCode,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// CreateParcelRequest is the payload for POST /orders/Create.
type CreateParcelRequest struct {
	ProductName     string  `json:"product_name"`
	ClientName      string  `json:"client_name"`
	Address         string  `json:"address"`
	Gouvernerate    string  `json:"gouvernerate"`
	City            string  `json:"city"`
	Location        string  `json:"location"`
	Tel1            string  `json:"Tel1"`
	Tel2            string  `json:"Tel2,omitempty"`
	Price           float64 `json:"price"`
	Exchange        string  `json:"exchange,omitempty"`
	OpenOrder       string  `json:"open_ordre,omitempty"`
	Note            string  `json:"note,omitempty"`
	AllowSubAccount bool    `json:"allow_sub_account,omitempty"`
	AccountCode     string  `json:"account_code,omitempty"`
}

// CreateParcelResponse is the response of POST /orders/Create.
type CreateParcelResponse struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
	Barcode string `json:"barcode"`
}

// GetOrderResponse is the response of POST /orders/GetOrder.
type GetOrderResponse struct {
	Barcode                string `json:"barcode"`
	Status                 string `json:"status"`
	DeliverymanName        string `json:"deliveryman_name,omitempty"`
	DeliverymanPhoneNumber string `json:"deliveryman_phone_number,omitempty"`
}

// apiError is the standard error body returned by the Mes Colis API.
type apiError struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e apiError) Error() string {
	return fmt.Sprintf("mescolis api error: %s (%s)", e.Message, e.Code)
}

// CreateParcel creates a new parcel and returns its barcode.
func (c *MescolisClient) CreateParcel(req CreateParcelRequest) (*CreateParcelResponse, error) {
	if c.APIKey == "" {
		return nil, errors.New("mescolis api key is not configured")
	}
	if c.AllowSubAccount && req.AccountCode == "" {
		req.AccountCode = c.AccountCode
		req.AllowSubAccount = true
	}

	resp, err := c.do("POST", "/orders/Create", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp)
	}

	var out CreateParcelResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("mescolis: failed to decode create parcel response: %w", err)
	}
	return &out, nil
}

// GetOrderStatus fetches the current status of a parcel by barcode.
func (c *MescolisClient) GetOrderStatus(barcode string) (*GetOrderResponse, error) {
	if c.APIKey == "" {
		return nil, errors.New("mescolis api key is not configured")
	}

	body, err := json.Marshal(map[string]string{"barcode": barcode})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, mescolisBaseURL+"/orders/GetOrder", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-access-token", c.APIKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mescolis: failed to fetch order status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeAPIError(resp)
	}

	var out GetOrderResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("mescolis: failed to decode order status response: %w", err)
	}
	return &out, nil
}

// DeleteParcel deletes a parcel by barcode from the Mes Colis platform.
func (c *MescolisClient) DeleteParcel(barcode string) error {
	if c.APIKey == "" {
		return errors.New("mescolis api key is not configured")
	}

	resp, err := c.do("DELETE", "/orders/DeleteOrder", map[string]string{"barcode": barcode})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp)
	}
	return nil
}

func (c *MescolisClient) do(method, path string, payload any) (*http.Response, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, mescolisBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-access-token", c.APIKey)

	return c.httpClient.Do(req)
}

func decodeAPIError(resp *http.Response) error {
	var e apiError
	data, err := io.ReadAll(resp.Body)
	if err == nil {
		if err := json.Unmarshal(data, &e); err == nil && e.Message != "" {
			return e
		}
		return fmt.Errorf("mescolis api error: %s", string(data))
	}
	return fmt.Errorf("mescolis api error: http %d", resp.StatusCode)
}
