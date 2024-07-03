// Package goproxy simple client for go modules proxy
// https://docs.gomods.io/intro/protocol/
package goproxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
	"unicode"
)

const defaultProxyURL = "https://proxy.golang.org"

// VersionInfo is the representation of a version.
type VersionInfo struct {
	Name    string
	Short   string
	Version string
	Time    time.Time
}

// Client is the go modules proxy client.
type Client struct {
	proxyURL   string
	HTTPClient *http.Client
}

// NewClient creates a new Client.
func NewClient(proxyURL string) *Client {
	client := &Client{
		HTTPClient: &http.Client{},
	}

	client.proxyURL = defaultProxyURL
	if proxyURL != "" {
		client.proxyURL = proxyURL
	}

	return client
}

// GetVersions gets all available module versions.
//	<proxy URL>/<module name>/@v/list
func (c *Client) GetVersions(moduleName string) ([]string, error) {
	uri := fmt.Sprintf("%s/%s/@v/list", c.proxyURL, safeModuleName(moduleName))

	resp, err := c.HTTPClient.Get(uri)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("invalid response: %s [%d]", resp.Status, resp.StatusCode)
	}

	var versions []string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		versions = append(versions, line)
	}

	return versions, nil
}

// GetInfo gets information about a module version.
//	<proxy URL>/<module name>/@v/<version>.info
func (c *Client) GetInfo(moduleName string, version string) (*VersionInfo, error) {
	return c.getInfo(fmt.Sprintf("%s/%s/@v/%s.info", c.proxyURL, safeModuleName(moduleName), version))
}

// GetLatest gets information about the latest module version.
//	<proxy URL>/<module name>/@latest
func (c *Client) GetLatest(moduleName string) (*VersionInfo, error) {
	return c.getInfo(fmt.Sprintf("%s/%s/@latest", c.proxyURL, safeModuleName(moduleName)))
}

func (c *Client) getInfo(uri string) (*VersionInfo, error) {
	resp, err := c.HTTPClient.Get(uri)
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("invalid response: %s [%d]", resp.Status, resp.StatusCode)
	}

	info := VersionInfo{}
	err = json.NewDecoder(resp.Body).Decode(&info)
	if err != nil {
		return nil, err
	}

	return &info, nil
}

func safeModuleName(name string) string {
	var to []byte
	for _, r := range name {
		if 'A' <= r && r <= 'Z' {
			to = append(to, '!', byte(unicode.ToLower(r)))
		} else {
			to = append(to, byte(r))
		}
	}

	return string(to)
}
