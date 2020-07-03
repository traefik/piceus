package plugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"
)

// Client for the plugin service.
type Client struct {
	baseURL     string
	httpClient  *http.Client
	accessToken string
}

// New creates a plugin service client.
func New(baseURL string, accessToken string) *Client {
	return &Client{
		baseURL:     baseURL,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		accessToken: accessToken,
	}
}

// Create creates a plugin.
func (c *Client) Create(p Plugin) error {
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("failed to parse base URL: %w", err)
	}

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshall: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL.String(), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.accessToken != "" {
		req.Header.Add("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call API: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Update updates a plugin.
func (c *Client) Update(p Plugin) error {
	if p.ID == "" {
		return errors.New("missing plugin ID")
	}

	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("failed to parse base URL: %w", err)
	}

	endpoint, err := baseURL.Parse(path.Join(baseURL.Path, p.ID))
	if err != nil {
		return fmt.Errorf("failed to parse endpoint URL: %w", err)
	}

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshall: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, endpoint.String(), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.accessToken != "" {
		req.Header.Add("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call API: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("%d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// GetByName gets a plugin by name.
func (c *Client) GetByName(name string) (*Plugin, error) {
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}

	query := baseURL.Query()
	query.Set("name", name)
	baseURL.RawQuery = query.Encode()

	req, err := http.NewRequest(http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if c.accessToken != "" {
		req.Header.Add("Authorization", "Bearer "+c.accessToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call API: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%d: %s", resp.StatusCode, string(body))
	}

	var plgs []Plugin
	err = json.Unmarshal(body, &plgs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarchall data: %w", err)
	}

	if len(plgs) != 1 {
		return nil, fmt.Errorf("failed to get plugin: %s", name)
	}

	return &plgs[0], nil
}
