package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// APIError an API error.
type APIError struct {
	StatusCode int
	Message    string
}

func (a *APIError) Error() string {
	return fmt.Sprintf("%d: %s", a.StatusCode, a.Message)
}

// Client for the plugin service.
type Client struct {
	s3Client   *s3.Client
	baseURL    string
	httpClient *http.Client
	plugins    map[string]Plugin
	s3Bucket   string
	s3Key      string
}

// New creates a plugin service client.
func New(ctx context.Context, baseURL, s3Bucket, s3Key string) *Client {
	awscfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to load AWS SDK configuration")
	}

	s3Client := s3.NewFromConfig(awscfg)
	s3Object, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(s3Key),
	})
	if err != nil {
		log.Error().Err(err).
			Str("s3 bucket", s3Bucket).
			Str("s3 key", s3Key).
			Msg("cannot get s3 file")
	}

	defer func() { _ = s3Object.Body.Close() }()
	plugins := make(map[string]Plugin)

	decoder := json.NewDecoder(s3Object.Body)
	if err := decoder.Decode(&plugins); err != nil {
		log.Error().Err(err).
			Str("s3 bucket", s3Bucket).
			Str("s3 key", s3Key).
			Msg("cannot decode s3 file")
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   10 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		s3Bucket: s3Bucket,
		s3Client: s3Client,
		s3Key:    s3Key,
		plugins:  plugins,
	}
}

// Create creates a plugin.
func (c *Client) Create(ctx context.Context, p Plugin) error {
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("failed to parse base URL: %w", err)
	}

	data, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshall: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL.String(), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call API: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)

		return &APIError{
			Message:    string(body),
			StatusCode: resp.StatusCode,
		}
	}

	c.plugins[p.Name] = p
	return nil
}

// Update updates a plugin.
func (c *Client) Update(ctx context.Context, p Plugin) error {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint.String(), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to call API: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		return &APIError{
			Message:    string(body),
			StatusCode: resp.StatusCode,
		}
	}

	_, ok := c.plugins[p.Name]
	if !ok {
		return fmt.Errorf("failed to find plugins %q to update", p.Name)
	}
	c.plugins[p.Name] = p

	return nil
}

// GetByName gets a plugin by name.
func (c *Client) GetByName(ctx context.Context, name string) (*Plugin, error) {
	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base URL: %w", err)
	}

	query := baseURL.Query()
	query.Set("name", name)
	baseURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call API: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode/100 != 2 {
		return nil, &APIError{
			Message:    string(body),
			StatusCode: resp.StatusCode,
		}
	}

	var plgs []Plugin
	err = json.Unmarshal(body, &plgs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarchall data: %w", err)
	}

	if len(plgs) != 1 {
		return nil, fmt.Errorf("failed to get plugin: %s", name)
	}

	plugin, ok := c.plugins[name]
	if !ok {
		return nil, fmt.Errorf("failed to find plugins by name %q", name)
	}

	return plugin, nil
}

// Flush writes plugin structure to s3.
func (c *Client) Flush(ctx context.Context) error {
	b := new(bytes.Buffer)
	encoder := json.NewEncoder(b)
	if err := encoder.Encode(c.plugins); err != nil {
		log.Error().Err(err).Msg("cannot encode plugins")
	}

	// To check: nothing to close / shutdown / free ?
	_, err := c.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(c.s3Bucket),
		Key:    aws.String(c.s3Key),
		Body:   b,
	})
	if err != nil {
		log.Error().Err(err).
			Str("s3 bucket", c.s3Bucket).
			Str("s3 key", c.s3Key).
			Msg("cannot put plugins data on s3")
	}

	return nil
}
