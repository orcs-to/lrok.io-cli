// Package apiclient is a tiny HTTP client for the lrok control-plane API
// (api.lrok.io). It carries the user's API token in Authorization Bearer.
package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.lrok.io"

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(token string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

type Reservation struct {
	Subdomain   string    `json:"subdomain"`
	CreatedAt   time.Time `json:"createdAt"`
	Description string    `json:"description,omitempty"`
}

func (c *Client) ListReservations() ([]Reservation, error) {
	var out []Reservation
	if err := c.do("GET", "/api/v1/me/reservations", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateReservation(subdomain, description string) (*Reservation, error) {
	body := map[string]string{"subdomain": subdomain, "description": description}
	var out Reservation
	if err := c.do("POST", "/api/v1/me/reservations", body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteReservation(subdomain string) error {
	return c.do("DELETE", "/api/v1/me/reservations/"+subdomain, nil, nil)
}

type Plan struct {
	TunnelQuota      int `json:"tunnelQuota"`
	TunnelUsed       int `json:"tunnelUsed"`
	ReservationQuota int `json:"reservationQuota"` // -1 = unlimited
	ReservationUsed  int `json:"reservationUsed"`
}

func (c *Client) GetPlan() (*Plan, error) {
	var out Plan
	if err := c.do("GET", "/api/v1/me/plan", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Tunnel describes an active tunnel attached to the caller's account.
// Field names match the shape served by the dashboard's existing endpoint.
type Tunnel struct {
	Subdomain string    `json:"subdomain"`
	PublicURL string    `json:"publicUrl"`
	Protocol  string    `json:"protocol,omitempty"`
	StartedAt time.Time `json:"startedAt"`
}

func (c *Client) ListMyTunnels() ([]Tunnel, error) {
	var out []Tunnel
	if err := c.do("GET", "/api/v1/me/tunnels", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) do(method, path string, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(string(respBody))
		return fmt.Errorf("%s %s: %s", method, path, statusMessage(resp.StatusCode, msg))
	}
	if out != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func statusMessage(code int, body string) string {
	if body == "" {
		return fmt.Sprintf("HTTP %d", code)
	}
	if len(body) > 400 {
		body = body[:400] + "..."
	}
	return fmt.Sprintf("HTTP %d: %s", code, body)
}
