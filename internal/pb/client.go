package pb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Client talks to the shared PocketBase instance as a superuser. The token is
// cached in memory and refreshed on expiry or on a 401 from upstream.
type Client struct {
	base     string
	email    string
	password string
	http     *http.Client

	mu    sync.Mutex
	token string
	at    time.Time
}

const tokenTTL = 6 * time.Hour

func New(base, email, password string) *Client {
	return &Client{
		base:     strings.TrimRight(base, "/"),
		email:    email,
		password: password,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) configured() bool {
	return c.base != "" && c.email != "" && c.password != ""
}

func (c *Client) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Since(c.at) < tokenTTL {
		return c.token, nil
	}
	if !c.configured() {
		return "", fmt.Errorf("pocketbase superuser credentials are not configured")
	}
	body, _ := json.Marshal(map[string]string{"identity": c.email, "password": c.password})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/api/collections/_superusers/auth-with-password", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("pocketbase superuser auth: %s", res.Status)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	c.token, c.at = out.Token, time.Now()
	return c.token, nil
}

func (c *Client) invalidate() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
}

// Do forwards a request to PocketBase. path must start with "/api/".
// A single retry is attempted when the cached token was rejected.
func (c *Client) Do(ctx context.Context, method, path string, query url.Values, body []byte, contentType string) (*http.Response, error) {
	target := c.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}

	send := func() (*http.Response, error) {
		tok, err := c.Token(ctx)
		if err != nil {
			return nil, err
		}
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, target, rdr)
		if err != nil {
			return nil, err
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		req.Header.Set("Authorization", tok)
		return c.http.Do(req)
	}

	res, err := send()
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		res.Body.Close()
		c.invalidate()
		return send()
	}
	return res, nil
}

// GetJSON is a small helper for the setup endpoints.
func (c *Client) GetJSON(ctx context.Context, path string, out any) error {
	res, err := c.Do(ctx, http.MethodGet, path, nil, nil, "")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("pocketbase %s: %s", path, res.Status)
	}
	return json.NewDecoder(res.Body).Decode(out)
}
