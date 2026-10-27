// Package audit checks that the archive served for each plugin version matches its recorded hash.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/piceus/pkg/catalog"
)

// Verdicts.
const (
	// VerdictOK the served archive matches the recorded hash and validate accepts it.
	VerdictOK = "ok"
	// VerdictMismatch the served archive does not match the recorded hash.
	VerdictMismatch = "mismatch"
	// VerdictValidateFailed the served archive matches the recorded hash but validate rejects it.
	VerdictValidateFailed = "validate_failed"
	// VerdictDownloadError the archive is not served.
	VerdictDownloadError = "download_error"
	// VerdictNetworkError the archive could not be fetched, re-run to confirm.
	VerdictNetworkError = "network_error"
)

const hashHeader = "X-Plugin-Hash"

// Result is the audit of one recorded hash.
type Result struct {
	catalog.Entry

	ServedHash     string `json:"servedHash,omitempty"`
	Size           int64  `json:"size,omitempty"`
	DownloadStatus int    `json:"downloadStatus,omitempty"`
	ValidateStatus int    `json:"validateStatus,omitempty"`
	CacheStatus    string `json:"cacheStatus,omitempty"`
	Verdict        string `json:"verdict"`
	Error          string `json:"error,omitempty"`

	Tag *TagCheck `json:"tag,omitempty"`
}

// Client probes a plugin service through its public endpoints.
type Client struct {
	BaseURL   string
	Username  string
	Password  string
	UserAgent string
	HTTP      *http.Client
	RetryWait time.Duration
}

// NewClient creates a client for the plugin service at baseURL.
func NewClient(baseURL, username, password string) *Client {
	return &Client{
		BaseURL:   strings.TrimSuffix(baseURL, "/"),
		Username:  username,
		Password:  password,
		UserAgent: "piceus-audit",
		HTTP:      &http.Client{Timeout: 5 * time.Minute},
		RetryWait: 2 * time.Second,
	}
}

// Run audits entries with the given concurrency. Results keep the order of entries.
func Run(ctx context.Context, c *Client, entries []catalog.Entry, concurrency int) []Result {
	results := make([]Result, len(entries))

	forEach(ctx, len(entries), concurrency, func(i int) {
		results[i] = probe(ctx, c, entries[i])
	})

	return results
}

func probe(ctx context.Context, c *Client, entry catalog.Entry) Result {
	res := Result{Entry: entry}

	dl, err := c.Download(ctx, entry.Module, entry.Version)

	res.DownloadStatus = dl.Status
	res.CacheStatus = dl.CacheStatus

	switch {
	case err != nil && dl.Status != 0:
		res.Verdict, res.Error = VerdictDownloadError, err.Error()
		return res
	case err != nil:
		res.Verdict, res.Error = VerdictNetworkError, err.Error()
		return res
	}

	res.ServedHash, res.Size = dl.Hash, dl.Size

	// A mismatch is known from the download alone.
	if dl.Hash != entry.Hash {
		res.Verdict = VerdictMismatch
		res.ValidateStatus, _ = c.Validate(ctx, entry.Module, entry.Version, dl.Hash)

		return res
	}

	res.ValidateStatus, err = c.Validate(ctx, entry.Module, entry.Version, dl.Hash)
	if err != nil {
		res.Verdict, res.Error = VerdictNetworkError, err.Error()
		return res
	}

	if res.ValidateStatus == http.StatusOK {
		res.Verdict = VerdictOK
	} else {
		res.Verdict = VerdictValidateFailed
	}

	return res
}

// Download is the outcome of one archive download.
type Download struct {
	Status      int
	CacheStatus string
	Hash        string
	Size        int64
}

// Download fetches the archive and hashes it, retrying on network and server errors.
// On failure, Status is the last HTTP status received, 0 if none.
func (c *Client) Download(ctx context.Context, module, version string) (Download, error) {
	return c.download(ctx, fmt.Sprintf("%s/public/download/%s/%s", c.BaseURL, module, version))
}

// DownloadUncached is Download with a unique query string, so the CDN forwards it to the plugin service.
func (c *Client) DownloadUncached(ctx context.Context, module, version string) (Download, error) {
	return c.download(ctx, fmt.Sprintf("%s/public/download/%s/%s?audit=%s", c.BaseURL, module, version, nonce()))
}

func downloadOnce(ctx context.Context, c *Client, endpoint string) (Download, bool, error) {
	req, err := c.newRequest(ctx, endpoint)
	if err != nil {
		return Download{}, false, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Download{}, true, err
	}

	defer func() { _ = resp.Body.Close() }()

	dl := Download{Status: resp.StatusCode, CacheStatus: resp.Header.Get("Cf-Cache-Status")}

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return dl, resp.StatusCode >= http.StatusInternalServerError, fmt.Errorf("download returned %d", resp.StatusCode)
	}

	hash := sha256.New()

	dl.Size, err = io.Copy(hash, resp.Body)
	if err != nil {
		return Download{}, true, err
	}

	dl.Hash = hex.EncodeToString(hash.Sum(nil))

	return dl, false, nil
}

// Validate calls the validate endpoint with hash and returns the status code,
// retrying on network and server errors. A server error left after the retries is returned as an error.
func (c *Client) Validate(ctx context.Context, module, version, hash string) (int, error) {
	var status int

	err := c.retry(ctx, func() (bool, error) {
		var err error

		status, err = validateOnce(ctx, c, module, version, hash)
		if err != nil {
			return true, err
		}

		if status >= http.StatusInternalServerError {
			return true, fmt.Errorf("validate returned %d", status)
		}

		return false, nil
	})

	return status, err
}

func validateOnce(ctx context.Context, c *Client, module, version, hash string) (int, error) {
	// A unique query string makes the CDN forward the request to the plugin service.
	endpoint := fmt.Sprintf("%s/public/validate/%s/%s?audit=%s", c.BaseURL, module, version, nonce())

	req, err := c.newRequest(ctx, endpoint)
	if err != nil {
		return 0, err
	}

	req.Header.Set(hashHeader, hash)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, err
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	return resp.StatusCode, nil
}

func nonce() string {
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func (c *Client) download(ctx context.Context, endpoint string) (Download, error) {
	var dl Download

	err := c.retry(ctx, func() (bool, error) {
		var (
			retry bool
			err   error
		)

		dl, retry, err = downloadOnce(ctx, c, endpoint)

		return retry, err
	})

	return dl, err
}

// retry calls fn up to 3 times, while it fails and asks for a retry.
func (c *Client) retry(ctx context.Context, fn func() (bool, error)) error {
	var err error

	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * c.RetryWait):
			}
		}

		var retry bool

		retry, err = fn()
		if err == nil || !retry {
			return err
		}
	}

	return err
}

func (c *Client) newRequest(ctx context.Context, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", c.UserAgent)

	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}

	return req, nil
}

// forEach calls fn for 0..n-1 with at most concurrency calls in flight, and stops early when ctx is done.
func forEach(ctx context.Context, n, concurrency int, fn func(i int)) {
	if concurrency < 1 {
		concurrency = 1
	}

	sem := make(chan struct{}, concurrency)

	var (
		wg   sync.WaitGroup
		done atomic.Int64
	)

	for i := range n {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case sem <- struct{}{}:
		}

		wg.Go(func() {
			defer func() { <-sem }()

			fn(i)

			if d := done.Add(1); d%100 == 0 {
				log.Info().Msgf("%d/%d", d, n)
			}
		})
	}

	wg.Wait()
}
