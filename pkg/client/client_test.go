package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

func TestNewWithOptions(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		desc                     string
		options                  []Option
		responses                []http.Response
		wantRequest              assert.ValueAssertionFunc
		wantResponseTimeInterval []time.Duration
		wantMetricRequestsTotal  int64
	}{
		{
			desc:      "without option",
			responses: []http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}},
			wantRequest: func(_ assert.TestingT, _ interface{}, _ ...interface{}) bool {
				return true
			},
		},
		{
			desc:      "with token",
			options:   []Option{WithToken("token")},
			responses: []http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}},
			wantRequest: func(t assert.TestingT, i interface{}, _ ...interface{}) bool {
				req := i.(*http.Request)
				assert.Equal(t, "Bearer token", req.Header.Get("Authorization"))
				return true
			},
		},
		{
			desc:                     "with rate limit (limit reached)",
			options:                  []Option{WithRateLimiter(1, 1, time.Now().Add(time.Second))},
			responses:                []http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}},
			wantResponseTimeInterval: []time.Duration{900 * time.Millisecond, time.Second + 500*time.Millisecond},
		},
		{
			desc:                     "with rate limit (limit not reached)",
			options:                  []Option{WithRateLimiter(10, 1, time.Now().Add(time.Second))},
			responses:                []http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}},
			wantResponseTimeInterval: []time.Duration{0 * time.Second, 50 * time.Millisecond},
		},
		{
			desc:    "with rate limit (limit from response)",
			options: []Option{WithRateLimiter(10, 1, time.Now().Add(10*time.Second))},
			responses: []http.Response{{
				StatusCode: http.StatusOK,
				Header: map[string][]string{
					headerRateRemaining: {"0"},
					headerRateReset:     {strconv.Itoa(int(time.Now().Add(2 * time.Second).Unix()))},
				},
				Body: io.NopCloser(strings.NewReader("{}")),
			}},
			wantResponseTimeInterval: []time.Duration{time.Second, 2*time.Second + 500*time.Millisecond},
		},
		{
			desc:                    "with metrics",
			options:                 []Option{WithMetrics(true)},
			responses:               []http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}},
			wantMetricRequestsTotal: 1,
		},
		{
			desc:                    "with metrics (disabled)",
			options:                 []Option{WithMetrics(false)},
			responses:               []http.Response{{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))}},
			wantMetricRequestsTotal: 0,
		},
		{
			desc:    "with retry",
			options: []Option{WithRetry(1, time.Second)},
			responses: []http.Response{
				{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("{}"))},
				{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))},
			},
			wantResponseTimeInterval: []time.Duration{900 * time.Millisecond, time.Second + 500*time.Millisecond},
		},
		{
			desc: "with all options",
			options: []Option{
				WithRateLimiter(1, 1, time.Now().Add(time.Second)),
				WithMetrics(true),
				WithToken("token"),
				WithRetry(1, time.Second),
			},
			responses: []http.Response{
				{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("{}"))},
				{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{}"))},
			},
			wantRequest: func(t assert.TestingT, i interface{}, _ ...interface{}) bool {
				req := i.(*http.Request)
				assert.Equal(t, "Bearer token", req.Header.Get("Authorization"))
				return true
			},
			wantResponseTimeInterval: []time.Duration{time.Second, 2*time.Second + 500*time.Millisecond},
			wantMetricRequestsTotal:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			t.Parallel()

			reader := sdkmetric.NewManualReader()
			metricProvider := sdkmetric.NewMeterProvider(
				sdkmetric.WithResource(resource.NewWithAttributes(
					semconv.SchemaURL,
					semconv.ServiceNameKey.String("piceus"),
					attribute.String("exporter", "prometheus"),
					attribute.String("namespace", "test"),
				)),
				sdkmetric.WithReader(
					reader,
				))
			otel.SetMeterProvider(metricProvider)

			c, err := New(ctx, tt.options...)
			require.NoError(t, err)
			assert.NotNil(t, c)

			try := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.wantRequest != nil {
					tt.wantRequest(t, r)
				}

				if len(tt.responses) <= try {
					t.Errorf("try #%d doesn't have associated response", try)
				}
				for h, v := range tt.responses[try].Header {
					w.Header().Set(h, v[0])
				}
				w.WriteHeader(tt.responses[try].StatusCode)

				bodyBytes, rErr := io.ReadAll(tt.responses[try].Body)
				assert.NoError(t, rErr)
				_, err = w.Write(bodyBytes)
				assert.NoError(t, err)
				try++
			}))
			t.Cleanup(server.Close)

			start := time.Now()
			resp, err := c.client.Get(server.URL)
			require.NoError(t, err)
			err = resp.Body.Close()
			require.NoError(t, err)

			if len(tt.wantResponseTimeInterval) == 2 {
				assert.WithinRange(t, time.Now(), start.Add(tt.wantResponseTimeInterval[0]), start.Add(tt.wantResponseTimeInterval[1]))
			}

			err = metricProvider.ForceFlush(ctx)
			require.NoError(t, err)

			data := metricdata.ResourceMetrics{}
			err = reader.Collect(ctx, &data)
			require.NoError(t, err)

			foundMetrics := false
			for _, sm := range data.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name == "http.requests.total" {
						foundMetrics = true
						assert.Equal(t, tt.wantMetricRequestsTotal, m.Data.(metricdata.Sum[int64]).DataPoints[0].Value)
						break
					}
				}
			}

			if tt.wantMetricRequestsTotal > 0 {
				assert.True(t, foundMetrics)
			} else {
				assert.False(t, foundMetrics)
			}
		})
	}
}
