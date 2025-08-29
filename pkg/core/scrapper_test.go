package core

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/google/go-github/v57/github"
	"github.com/ldez/grignotin/goproxy"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/piceus/internal/plugin"
	"github.com/traefik/piceus/pkg/sources"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/oauth2"
)

type mockPluginClient struct {
	create    func(p plugin.Plugin) error
	update    func(p plugin.Plugin) error
	getByName func(string) (*plugin.Plugin, error)
}

func (f *mockPluginClient) Create(_ context.Context, p plugin.Plugin) error {
	log.Info().Str("module_name", p.Name).Interface("plugin", p).Msg("Create plugin")

	if f.create != nil {
		return f.create(p)
	}

	return nil
}

func (f *mockPluginClient) Update(_ context.Context, p plugin.Plugin) error {
	log.Info().Str("module_name", p.Name).Interface("plugin", p).Msg("Update plugin")

	if f.update != nil {
		return f.update(p)
	}

	return nil
}

func (f *mockPluginClient) GetByName(_ context.Context, name string) (*plugin.Plugin, error) {
	if f.getByName != nil {
		return f.getByName(name)
	}

	return nil, nil
}

func Test_loadManifestContent(t *testing.T) {
	testCases := []struct {
		desc     string
		filename string
		expected Manifest
	}{
		{
			desc:     "Middleware",
			filename: ".traefik-middleware.yml",
			expected: Manifest{
				DisplayName:   "Plugin Example",
				Type:          "middleware",
				Import:        "github.com/traefik/plugintest/example",
				BasePkg:       "example",
				Compatibility: "TODO",
				Summary:       "Simple example plugin.",
				IconPath:      "icon.png",
				BannerPath:    "https://example.org/a/banner.png",
				TestData: map[string]interface{}{
					"Headers": map[string]interface{}{
						"Foo": "Bar",
					},
					"trustIP": []interface{}{
						"10.0.0.0/8",
						"172.0.0.0/8",
						"192.0.0.0/8",
					},
					"allowedGroups": []interface{}{
						"ou=mathematicians,dc=example,dc=com",
						"ou=foo,ou=scientists,dc=example,dc=com",
					},
					"valuesFloat": []interface{}{
						float64(1),
						2.01,
						3.01,
					},
					"valuesInt": []interface{}{
						int64(1),
						int64(2),
						int64(3),
					},
				},
			},
		},
		{
			desc:     "Provider",
			filename: ".traefik-provider.yml",
			expected: Manifest{
				DisplayName:   "Plugin Example",
				Type:          "provider",
				Import:        "github.com/traefik/plugintest/example",
				BasePkg:       "example",
				Compatibility: "TODO",
				Summary:       "Simple example plugin.",
				IconPath:      "icon.png",
				BannerPath:    "http://example.org/a/banner.png",
				TestData: map[string]interface{}{
					"Foo": "Bar",
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			file, err := os.Open("./fixtures/" + test.filename)
			require.NoError(t, err)

			t.Cleanup(func() { _ = file.Close() })

			b, err := io.ReadAll(file)
			require.NoError(t, err)

			s := Scrapper{}
			m, err := s.loadManifestContent(string(b))
			require.NoError(t, err)

			assert.Equal(t, test.expected, m)
		})
	}
}

func TestScrapper_store(t *testing.T) {
	testCases := []struct {
		desc     string
		pgClient pluginClient
	}{
		{
			desc: "create",
			pgClient: &mockPluginClient{
				getByName: func(_ string) (*plugin.Plugin, error) {
					return nil, &plugin.APIError{StatusCode: http.StatusNotFound, Message: "not found"}
				},
			},
		},
		{
			desc: "update",
			pgClient: &mockPluginClient{
				getByName: func(_ string) (*plugin.Plugin, error) {
					return &plugin.Plugin{ID: "aaaa", Name: "test"}, nil
				},
			},
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			scrapper := NewScrapper(nil, nil, test.pgClient, true, nil, nil, nil)

			data := &plugin.Plugin{Name: "test"}
			err := scrapper.store(context.Background(), data)

			require.NoError(t, err)
		})
	}
}

func Test_createMiddlewareSnippets(t *testing.T) {
	repository := &github.Repository{
		Name: github.String("plugintest"),
	}

	testData := map[string]interface{}{
		"Headers": map[string]interface{}{
			"Foo": "Bar",
		},
	}

	snippets, err := createMiddlewareSnippets(repository, testData)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"toml": `
[http]

  [http.middlewares]

    [http.middlewares.my-plugintest]

      [http.middlewares.my-plugintest.plugin]

        [http.middlewares.my-plugintest.plugin.plugintest]

          [http.middlewares.my-plugintest.plugin.plugintest.Headers]
            Foo = "Bar"
`,
		"yaml": `http:
    middlewares:
        my-plugintest:
            plugin:
                plugintest:
                    Headers:
                        Foo: Bar
`,
		"k8s": `apiVersion: traefik.containo.us/v1alpha1
kind: Middleware
metadata:
    name: my-plugintest
    namespace: my-namespace
spec:
    plugin:
        plugintest:
            Headers:
                Foo: Bar
`,
	}

	assert.Equal(t, expected, snippets)
}

func Test_createProviderSnippets(t *testing.T) {
	repository := &github.Repository{
		Name: github.String("plugintest"),
	}

	testData := map[string]interface{}{
		"foo": "Bar",
	}

	snippets, err := createProviderSnippets(repository, testData)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"toml": `
[providers]

  [providers.plugin]

    [providers.plugin.plugintest]
      foo = "Bar"
`,
		"yaml": `providers:
    plugin:
        plugintest:
            foo: Bar
`,
	}

	assert.Equal(t, expected, snippets)
}

func Test_parseImageURL(t *testing.T) {
	repo := &github.Repository{
		Owner: &github.User{
			Login: github.String("traefik"),
		},
		Name:    github.String("traefik"),
		HTMLURL: github.String("https://github.com/traefik/traefik/"),
	}

	testCases := []struct {
		desc     string
		imgPath  string
		expected string
	}{
		{
			desc:     "empty image path",
			imgPath:  "",
			expected: "",
		},
		{
			desc:     "full URL with /raw",
			imgPath:  "https://github.com/traefik/traefik/raw/v2.0.0/docs/content/assets/img/traefik.logo.png",
			expected: "https://github.com/traefik/traefik/raw/v2.0.0/docs/content/assets/img/traefik.logo.png",
		},
		{
			desc:     "full URL with raw.githubusercontent.com",
			imgPath:  "https://raw.githubusercontent.com/traefik/traefik/master/docs/content/assets/img/traefik.logo.png",
			expected: "https://raw.githubusercontent.com/traefik/traefik/master/docs/content/assets/img/traefik.logo.png",
		},
		{
			desc:     "invalid host",
			imgPath:  "https://example.com/traefik/traefik/master/docs/content/assets/img/traefik.logo.png",
			expected: "",
		},
		{
			desc:     "relative path with .",
			imgPath:  "./docs/content/assets/img/traefik.logo.png",
			expected: "https://raw.githubusercontent.com/traefik/traefik/v2.0.0/docs/content/assets/img/traefik.logo.png",
		},
		{
			desc:     "relative path",
			imgPath:  "docs/content/assets/img/traefik.logo.png",
			expected: "https://raw.githubusercontent.com/traefik/traefik/v2.0.0/docs/content/assets/img/traefik.logo.png",
		},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			imgURL := parseImageURL(repo, "v2.0.0", test.imgPath)

			assert.Equal(t, test.expected, imgURL)
		})
	}
}

func TestScrapper_process(t *testing.T) {
	t.Skip("for debug purpose only")

	token := ""
	owner := ""
	repo := ""

	ctx := context.Background()

	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	client.Transport = otelhttp.NewTransport(client.Transport)

	ghClient := github.NewClient(client)
	pgClient := plugin.New("") // ignored for this test
	gpClient := goproxy.NewClient("")
	srcs := &sources.GitHub{Client: ghClient}

	scrapper := NewScrapper(ghClient, gpClient, pgClient, true, srcs, []string{"topic:traefik-plugin language:Go archived:false is:public"}, []string{"is:open is:issue is:public author:traefiker"})

	repository, _, err := ghClient.Repositories.Get(ctx, owner, repo)
	require.NoError(t, err)

	p, err := scrapper.process(ctx, repository)
	require.NoError(t, err)

	assert.NotNil(t, p)
}

func TestScrapper_process_all(t *testing.T) {
	t.Skip("for debug purpose only")

	token := ""
	ctx := context.Background()

	client := oauth2.NewClient(ctx, oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}))
	client.Transport = otelhttp.NewTransport(client.Transport)

	ghClient := github.NewClient(client)
	pgClient := plugin.New("") // ignored for this test
	gpClient := goproxy.NewClient("")
	srcs := &sources.GitHub{Client: ghClient}

	scrapper := NewScrapper(ghClient, gpClient, pgClient, true, srcs, []string{"topic:traefik-plugin language:Go archived:false is:public"}, []string{"is:open is:issue is:public author:traefiker"})

	reposWithExistingIssue, err := scrapper.searchReposWithExistingIssue(ctx)
	require.NoError(t, err)

	repositories, err := scrapper.search(ctx)
	require.NoError(t, err)

	for _, repository := range repositories {
		logger := log.With().Str("repo_name", repository.GetFullName()).Logger()

		if scrapper.isSkipped(logger.WithContext(ctx), reposWithExistingIssue, repository) {
			continue
		}

		t.Log(repository.GetFullName())

		_, err := scrapper.process(ctx, repository)
		if err != nil {
			t.Logf("%s: %v", repository.GetFullName(), err)
		}
	}
}

func Test_safeIssueBody(t *testing.T) {
	t.Setenv("FEATURE_SERVICE_PORT", "tcp://172.20.236.87:80")
	t.Setenv("FEATURE_SERVICE_PORT_80_TCP", "tcp://172.20.236.87:80")
	t.Setenv("FEATURE_SERVICE_PORT_80_TCP_ADDR", "172.20.236.87")
	t.Setenv("FEATURE_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("FEATURE_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("FEATURE_SERVICE_SERVICE_HOST", "172.20.236.87")
	t.Setenv("FEATURE_SERVICE_SERVICE_PORT", "80")
	t.Setenv("FEATURE_SERVICE_SERVICE_PORT_FEATURE_SERVICE", "80")
	t.Setenv("GITHUB_TOKEN", "d29e33c33d871a2c7300b14069b14643b54b5aeeadfc1347de15404fcb3a3cd2")
	t.Setenv("HOSTNAME", "piceus-job-1634142000-hltrs")
	t.Setenv("INSTANCE_INFO_SERVICE_PORT", "tcp://172.20.17.141:80")
	t.Setenv("INSTANCE_INFO_SERVICE_PORT_80_TCP", "tcp://172.20.17.141:80")
	t.Setenv("INSTANCE_INFO_SERVICE_PORT_80_TCP_ADDR", "172.20.17.141")
	t.Setenv("INSTANCE_INFO_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("INSTANCE_INFO_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("INSTANCE_INFO_SERVICE_SERVICE_HOST", "172.20.17.141")
	t.Setenv("INSTANCE_INFO_SERVICE_SERVICE_PORT", "80")
	t.Setenv("INSTANCE_INFO_SERVICE_SERVICE_PORT_INSTANCE_INFO_SERVICE", "80")
	t.Setenv("KUBERNETES_PORT", "tcp://172.20.0.1:443")
	t.Setenv("KUBERNETES_PORT_443_TCP", "tcp://172.20.0.1:443")
	t.Setenv("KUBERNETES_PORT_443_TCP_ADDR", "172.20.0.1")
	t.Setenv("KUBERNETES_PORT_443_TCP_PORT", "443")
	t.Setenv("KUBERNETES_PORT_443_TCP_PROTO", "tcp")
	t.Setenv("KUBERNETES_SERVICE_HOST", "172.20.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	t.Setenv("KUBERNETES_SERVICE_PORT_HTTPS", "443")
	t.Setenv("METRIC_SERVICE_PORT", "tcp://172.20.75.223:80")
	t.Setenv("METRIC_SERVICE_PORT_80_TCP", "tcp://172.20.75.223:80")
	t.Setenv("METRIC_SERVICE_PORT_80_TCP_ADDR", "172.20.75.223")
	t.Setenv("METRIC_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("METRIC_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("METRIC_SERVICE_SERVICE_HOST", "172.20.75.223")
	t.Setenv("METRIC_SERVICE_SERVICE_PORT", "80")
	t.Setenv("METRIC_SERVICE_SERVICE_PORT_METRIC_SERVICE", "80")
	t.Setenv("MONITORING_SERVICE_PORT", "tcp://172.20.94.195:80")
	t.Setenv("MONITORING_SERVICE_PORT_80_TCP", "tcp://172.20.94.195:80")
	t.Setenv("MONITORING_SERVICE_PORT_80_TCP_ADDR", "172.20.94.195")
	t.Setenv("MONITORING_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("MONITORING_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("MONITORING_SERVICE_SERVICE_HOST", "172.20.94.195")
	t.Setenv("MONITORING_SERVICE_SERVICE_PORT", "80")
	t.Setenv("MONITORING_SERVICE_SERVICE_PORT_MONITORING_SERVICE", "80")
	t.Setenv("MULTI_CLUSTER_SERVICE_PORT", "tcp://172.20.4.179:80")
	t.Setenv("MULTI_CLUSTER_SERVICE_PORT_80_TCP", "tcp://172.20.4.179:80")
	t.Setenv("MULTI_CLUSTER_SERVICE_PORT_80_TCP_ADDR", "172.20.4.179")
	t.Setenv("MULTI_CLUSTER_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("MULTI_CLUSTER_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("MULTI_CLUSTER_SERVICE_SERVICE_HOST", "172.20.4.179")
	t.Setenv("MULTI_CLUSTER_SERVICE_SERVICE_PORT", "80")
	t.Setenv("MULTI_CLUSTER_SERVICE_SERVICE_PORT_MULTI_CLUSTER_SERVICE", "80")
	t.Setenv("NOTIFICATION_SERVICE_PORT", "tcp://172.20.246.182:80")
	t.Setenv("NOTIFICATION_SERVICE_PORT_80_TCP", "tcp://172.20.246.182:80")
	t.Setenv("NOTIFICATION_SERVICE_PORT_80_TCP_ADDR", "172.20.246.182")
	t.Setenv("NOTIFICATION_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("NOTIFICATION_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("NOTIFICATION_SERVICE_SERVICE_HOST", "172.20.246.182")
	t.Setenv("NOTIFICATION_SERVICE_SERVICE_PORT", "80")
	t.Setenv("NOTIFICATION_SERVICE_SERVICE_PORT_NOTIFICATION_SERVICE", "80")
	t.Setenv("ORGANIZATION_SERVICE_PORT", "tcp://172.20.250.189:80")
	t.Setenv("ORGANIZATION_SERVICE_PORT_80_TCP", "tcp://172.20.250.189:80")
	t.Setenv("ORGANIZATION_SERVICE_PORT_80_TCP_ADDR", "172.20.250.189")
	t.Setenv("ORGANIZATION_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("ORGANIZATION_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("ORGANIZATION_SERVICE_SERVICE_HOST", "172.20.250.189")
	t.Setenv("ORGANIZATION_SERVICE_SERVICE_PORT", "80")
	t.Setenv("ORGANIZATION_SERVICE_SERVICE_PORT_ORGANIZATION_SERVICE", "80")
	t.Setenv("PICEUS_PRIVATE_MODE", "true")
	t.Setenv("PLUGIN_URL", "http://plugin-service/internal/")
	t.Setenv("SERVICES_ACCESS_TOKEN", "7138b22877a8e9f9caf95528dbe82ff25fb2e836ce459a2b99b53f2aa336aca9")
	t.Setenv("PLATFORM_WEBAPP_PORT", "tcp://172.20.6.189:80")
	t.Setenv("PLATFORM_WEBAPP_PORT_80_TCP", "tcp://172.20.6.189:80")
	t.Setenv("PLATFORM_WEBAPP_PORT_80_TCP_ADDR", "172.20.6.189")
	t.Setenv("PLATFORM_WEBAPP_PORT_80_TCP_PORT", "80")
	t.Setenv("PLATFORM_WEBAPP_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("PLATFORM_WEBAPP_SERVICE_HOST", "172.20.6.189")
	t.Setenv("PLATFORM_WEBAPP_SERVICE_PORT", "80")
	t.Setenv("PLATFORM_WEBAPP_SERVICE_PORT_PLATFORM_WEBAPP", "80")
	t.Setenv("PLUGIN_SERVICE_PORT", "tcp://172.20.37.203:80")
	t.Setenv("PLUGIN_SERVICE_PORT_80_TCP", "tcp://172.20.37.203:80")
	t.Setenv("PLUGIN_SERVICE_PORT_80_TCP_ADDR", "172.20.37.203")
	t.Setenv("PLUGIN_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("PLUGIN_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("PLUGIN_SERVICE_SERVICE_HOST", "172.20.37.203")
	t.Setenv("PLUGIN_SERVICE_SERVICE_PORT", "80")
	t.Setenv("PLUGIN_SERVICE_SERVICE_PORT_PLUGIN_SERVICE", "80")
	t.Setenv("SECURITY_ISSUES_SERVICE_PORT", "tcp://172.20.112.31:80")
	t.Setenv("SECURITY_ISSUES_SERVICE_PORT_80_TCP", "tcp://172.20.112.31:80")
	t.Setenv("SECURITY_ISSUES_SERVICE_PORT_80_TCP_ADDR", "172.20.112.31")
	t.Setenv("SECURITY_ISSUES_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("SECURITY_ISSUES_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("SECURITY_ISSUES_SERVICE_SERVICE_HOST", "172.20.112.31")
	t.Setenv("SECURITY_ISSUES_SERVICE_SERVICE_PORT", "80")
	t.Setenv("SECURITY_ISSUES_SERVICE_SERVICE_PORT_SECURITY_ISSUES_SERVICE", "80")
	t.Setenv("SUBSCRIPTION_SERVICE_PORT", "tcp://172.20.32.181:80")
	t.Setenv("SUBSCRIPTION_SERVICE_PORT_80_TCP", "tcp://172.20.32.181:80")
	t.Setenv("SUBSCRIPTION_SERVICE_PORT_80_TCP_ADDR", "172.20.32.181")
	t.Setenv("SUBSCRIPTION_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("SUBSCRIPTION_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("SUBSCRIPTION_SERVICE_SERVICE_HOST", "172.20.32.181")
	t.Setenv("SUBSCRIPTION_SERVICE_SERVICE_PORT", "80")
	t.Setenv("SUBSCRIPTION_SERVICE_SERVICE_PORT_SUBSCRIPTION_SERVICE", "80")
	t.Setenv("THANOS_COMPACTOR_PORT", "tcp://172.20.133.173:10902")
	t.Setenv("THANOS_COMPACTOR_PORT_10902_TCP", "tcp://172.20.133.173:10902")
	t.Setenv("THANOS_COMPACTOR_PORT_10902_TCP_ADDR", "172.20.133.173")
	t.Setenv("THANOS_COMPACTOR_PORT_10902_TCP_PORT", "10902")
	t.Setenv("THANOS_COMPACTOR_PORT_10902_TCP_PROTO", "tcp")
	t.Setenv("THANOS_COMPACTOR_SERVICE_HOST", "172.20.133.173")
	t.Setenv("THANOS_COMPACTOR_SERVICE_PORT", "10902")
	t.Setenv("TOKEN_SERVICE_PORT", "tcp://172.20.180.224:80")
	t.Setenv("TOKEN_SERVICE_PORT_80_TCP", "tcp://172.20.180.224:80")
	t.Setenv("TOKEN_SERVICE_PORT_80_TCP_ADDR", "172.20.180.224")
	t.Setenv("TOKEN_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("TOKEN_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("TOKEN_SERVICE_SERVICE_HOST", "172.20.180.224")
	t.Setenv("TOKEN_SERVICE_SERVICE_PORT", "80")
	t.Setenv("TOKEN_SERVICE_SERVICE_PORT_TOKEN_SERVICE", "80")
	t.Setenv("TRACING_PASSWORD", "c487989b32abb6fa024c70443e6db7205ef4d5e62c8d55e9a89a0eea9c72f0de")
	t.Setenv("TRACING_PROBABILITY", "0")
	t.Setenv("TRACING_USERNAME", "jaeger")
	t.Setenv("USER_SERVICE_PORT", "tcp://172.20.238.163:80")
	t.Setenv("USER_SERVICE_PORT_80_TCP", "tcp://172.20.238.163:80")
	t.Setenv("USER_SERVICE_PORT_80_TCP_ADDR", "172.20.238.163")
	t.Setenv("USER_SERVICE_PORT_80_TCP_PORT", "80")
	t.Setenv("USER_SERVICE_PORT_80_TCP_PROTO", "tcp")
	t.Setenv("USER_SERVICE_SERVICE_HOST", "172.20.238.163")
	t.Setenv("USER_SERVICE_SERVICE_PORT", "80")
	t.Setenv("USER_SERVICE_SERVICE_PORT_USER_SERVICE", "80")

	err := errors.New(`failed to run the plugin with Yaegi: failed to create a new plugin instance: failed to open database: open /root/go/src/github.com/nscuro/traefik-plugin-geoblock/IP2LOCATION-LITE-DB1.IPV6.BIN: no such file or directory (cwd: /, gopath: /root/go, env: []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOSTNAME=piceus-job-1634142000-hltrs", "PICEUS_PRIVATE_MODE=true", "TRACING_USERNAME=jaeger", "TRACING_PASSWORD=c487989b32abb6fa024c70443e6db7205ef4d5e62c8d55e9a89a0eea9c72f0de", "TRACING_PROBABILITY=0", "GITHUB_TOKEN=d29e33c33d871a2c7300b14069b14643b54b5aeeadfc1347de15404fcb3a3cd2", "SERVICES_ACCESS_TOKEN=7138b22877a8e9f9caf95528dbe82ff25fb2e836ce459a2b99b53f2aa336aca9", "PLUGIN_URL=http://plugin-service/internal/", "SUBSCRIPTION_SERVICE_SERVICE_PORT_SUBSCRIPTION_SERVICE=80", "ORGANIZATION_SERVICE_PORT_80_TCP_PROTO=tcp", "SECURITY_ISSUES_SERVICE_PORT_80_TCP_ADDR=172.20.112.31", "KUBERNETES_PORT_443_TCP_ADDR=172.20.0.1", "KUBERNETES_SERVICE_HOST=172.20.0.1", "KUBERNETES_SERVICE_PORT_HTTPS=443", "THANOS_COMPACTOR_PORT_10902_TCP_PORT=10902", "THANOS_COMPACTOR_PORT_10902_TCP_ADDR=172.20.133.173", "USER_SERVICE_PORT=tcp://172.20.238.163:80", "FEATURE_SERVICE_SERVICE_PORT=80", "SUBSCRIPTION_SERVICE_PORT_80_TCP_PORT=80", "ORGANIZATION_SERVICE_PORT=tcp://172.20.250.189:80", "SECURITY_ISSUES_SERVICE_PORT=tcp://172.20.112.31:80", "MONITORING_SERVICE_PORT_80_TCP_ADDR=172.20.94.195", "PLATFORM_WEBAPP_PORT_80_TCP=tcp://172.20.6.189:80", "MULTI_CLUSTER_SERVICE_SERVICE_PORT_MULTI_CLUSTER_SERVICE=80", "SUBSCRIPTION_SERVICE_PORT_80_TCP_PROTO=tcp", "PLATFORM_WEBAPP_PORT_80_TCP_ADDR=172.20.6.189", "MONITORING_SERVICE_PORT_80_TCP_PORT=80", "SUBSCRIPTION_SERVICE_PORT=tcp://172.20.32.181:80", "PLUGIN_SERVICE_PORT=tcp://172.20.37.203:80", "TOKEN_SERVICE_PORT_80_TCP_PROTO=tcp", "PLUGIN_SERVICE_PORT_80_TCP_PROTO=tcp", "ORGANIZATION_SERVICE_PORT_80_TCP_ADDR=172.20.250.189", "THANOS_COMPACTOR_SERVICE_HOST=172.20.133.173", "SUBSCRIPTION_SERVICE_SERVICE_PORT=80", "FEATURE_SERVICE_PORT_80_TCP_PORT=80", "SECURITY_ISSUES_SERVICE_SERVICE_PORT=80", "SECURITY_ISSUES_SERVICE_PORT_80_TCP_PROTO=tcp", "TOKEN_SERVICE_SERVICE_PORT=80", "INSTANCE_INFO_SERVICE_SERVICE_PORT=80", "PLUGIN_SERVICE_PORT_80_TCP_PORT=80", "KUBERNETES_PORT=tcp://172.20.0.1:443", "METRIC_SERVICE_SERVICE_HOST=172.20.75.223", "KUBERNETES_PORT_443_TCP=tcp://172.20.0.1:443", "METRIC_SERVICE_PORT=tcp://172.20.75.223:80", "PLUGIN_SERVICE_PORT_80_TCP=tcp://172.20.37.203:80", "ORGANIZATION_SERVICE_SERVICE_PORT=80", "MULTI_CLUSTER_SERVICE_PORT_80_TCP_PROTO=tcp", "KUBERNETES_SERVICE_PORT=443", "USER_SERVICE_SERVICE_PORT=80", "INSTANCE_INFO_SERVICE_SERVICE_HOST=172.20.17.141", "PLUGIN_SERVICE_PORT_80_TCP_ADDR=172.20.37.203", "SECURITY_ISSUES_SERVICE_SERVICE_HOST=172.20.112.31", "MULTI_CLUSTER_SERVICE_PORT=tcp://172.20.4.179:80", "MONITORING_SERVICE_PORT_80_TCP_PROTO=tcp", "USER_SERVICE_PORT_80_TCP_ADDR=172.20.238.163", "INSTANCE_INFO_SERVICE_SERVICE_PORT_INSTANCE_INFO_SERVICE=80", "SUBSCRIPTION_SERVICE_SERVICE_HOST=172.20.32.181", "MULTI_CLUSTER_SERVICE_PORT_80_TCP_ADDR=172.20.4.179", "MONITORING_SERVICE_PORT_80_TCP=tcp://172.20.94.195:80", "METRIC_SERVICE_SERVICE_PORT=80", "INSTANCE_INFO_SERVICE_PORT_80_TCP_PORT=80", "FEATURE_SERVICE_PORT_80_TCP_PROTO=tcp", "PLATFORM_WEBAPP_SERVICE_PORT=80", "TOKEN_SERVICE_PORT_80_TCP=tcp://172.20.180.224:80", "MULTI_CLUSTER_SERVICE_PORT_80_TCP=tcp://172.20.4.179:80", "KUBERNETES_PORT_443_TCP_PORT=443", "THANOS_COMPACTOR_PORT_10902_TCP_PROTO=tcp", "METRIC_SERVICE_PORT_80_TCP=tcp://172.20.75.223:80", "ORGANIZATION_SERVICE_PORT_80_TCP=tcp://172.20.250.189:80", "TOKEN_SERVICE_SERVICE_HOST=172.20.180.224", "MONITORING_SERVICE_SERVICE_PORT_MONITORING_SERVICE=80", "FEATURE_SERVICE_PORT_80_TCP=tcp://172.20.236.87:80", "INSTANCE_INFO_SERVICE_PORT_80_TCP_PROTO=tcp", "FEATURE_SERVICE_PORT_80_TCP_ADDR=172.20.236.87", "THANOS_COMPACTOR_PORT=tcp://172.20.133.173:10902", "USER_SERVICE_SERVICE_HOST=172.20.238.163", "ORGANIZATION_SERVICE_SERVICE_HOST=172.20.250.189", "TOKEN_SERVICE_SERVICE_PORT_TOKEN_SERVICE=80", "MULTI_CLUSTER_SERVICE_SERVICE_PORT=80", "MONITORING_SERVICE_PORT=tcp://172.20.94.195:80", "NOTIFICATION_SERVICE_PORT_80_TCP_PROTO=tcp", "MULTI_CLUSTER_SERVICE_SERVICE_HOST=172.20.4.179", "USER_SERVICE_PORT_80_TCP_PROTO=tcp", "PLATFORM_WEBAPP_PORT=tcp://172.20.6.189:80", "ORGANIZATION_SERVICE_PORT_80_TCP_PORT=80", "SECURITY_ISSUES_SERVICE_PORT_80_TCP=tcp://172.20.112.31:80", "NOTIFICATION_SERVICE_PORT=tcp://172.20.246.182:80", "KUBERNETES_PORT_443_TCP_PROTO=tcp", "FEATURE_SERVICE_SERVICE_PORT_FEATURE_SERVICE=80", "USER_SERVICE_PORT_80_TCP=tcp://172.20.238.163:80", "SUBSCRIPTION_SERVICE_PORT_80_TCP=tcp://172.20.32.181:80", "PLATFORM_WEBAPP_PORT_80_TCP_PORT=80", "TOKEN_SERVICE_PORT_80_TCP_PORT=80", "NOTIFICATION_SERVICE_PORT_80_TCP=tcp://172.20.246.182:80", "SUBSCRIPTION_SERVICE_PORT_80_TCP_ADDR=172.20.32.181", "NOTIFICATION_SERVICE_SERVICE_PORT=80", "MONITORING_SERVICE_SERVICE_HOST=172.20.94.195", "USER_SERVICE_SERVICE_PORT_USER_SERVICE=80", "THANOS_COMPACTOR_PORT_10902_TCP=tcp://172.20.133.173:10902", "INSTANCE_INFO_SERVICE_PORT_80_TCP_ADDR=172.20.17.141", "FEATURE_SERVICE_PORT=tcp://172.20.236.87:80", "PLUGIN_SERVICE_SERVICE_HOST=172.20.37.203", "NOTIFICATION_SERVICE_PORT_80_TCP_PORT=80", "MONITORING_SERVICE_SERVICE_PORT=80", "METRIC_SERVICE_PORT_80_TCP_PROTO=tcp", "THANOS_COMPACTOR_SERVICE_PORT=10902", "INSTANCE_INFO_SERVICE_PORT_80_TCP=tcp://172.20.17.141:80", "PLATFORM_WEBAPP_PORT_80_TCP_PROTO=tcp", "ORGANIZATION_SERVICE_SERVICE_PORT_ORGANIZATION_SERVICE=80", "TOKEN_SERVICE_PORT=tcp://172.20.180.224:80", "MULTI_CLUSTER_SERVICE_PORT_80_TCP_PORT=80", "PLATFORM_WEBAPP_SERVICE_PORT_PLATFORM_WEBAPP=80", "INSTANCE_INFO_SERVICE_PORT=tcp://172.20.17.141:80", "PLATFORM_WEBAPP_SERVICE_HOST=172.20.6.189", "USER_SERVICE_PORT_80_TCP_PORT=80", "FEATURE_SERVICE_SERVICE_HOST=172.20.236.87", "NOTIFICATION_SERVICE_SERVICE_PORT_NOTIFICATION_SERVICE=80", "NOTIFICATION_SERVICE_PORT_80_TCP_ADDR=172.20.246.182", "METRIC_SERVICE_PORT_80_TCP_ADDR=172.20.75.223", "PLUGIN_SERVICE_SERVICE_PORT=80", "PLUGIN_SERVICE_SERVICE_PORT_PLUGIN_SERVICE=80", "TOKEN_SERVICE_PORT_80_TCP_ADDR=172.20.180.224", "METRIC_SERVICE_SERVICE_PORT_METRIC_SERVICE=80", "SECURITY_ISSUES_SERVICE_SERVICE_PORT_SECURITY_ISSUES_SERVICE=80", "SECURITY_ISSUES_SERVICE_PORT_80_TCP_PORT=80", "NOTIFICATION_SERVICE_SERVICE_HOST=172.20.246.182", "METRIC_SERVICE_PORT_80_TCP_PORT=80", "HOME=/root"})`)

	body := safeIssueBody(err)

	expected := "The plugin was not imported into Traefik Plugin Catalog.\n\nCause:\n```\nfailed to run the plugin with Yaegi: failed to create a new plugin instance: failed to open database: open /root/go/src/github.com/nscuro/traefik-plugin-geoblock/IP2LOCATION-LITE-DB1.IPV6.BIN: no such file or directory (cwd: /, gopath: /root/go, env: []string{\"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin\", \"HOSTNAME=piceus-job-1634142000-hltrs\", \"PICEUS_PRIVATE_MODE=true\", \"xxx=xxx\", \"xxx=xxx\", \"TRACING_PROBABILITY=0\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=443\", \"xxx=10902\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=443\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=443\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=10902\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"xxx=xxx\", \"HOME=/root\"})\n```\nTraefik Plugin Analyzer will restart when you will close this issue.\n\nIf you believe there is a problem with the Analyzer or this issue is the result of a false positive, please fill an issue on [piceus](https://github.com/traefik/piceus) repository.\n"

	assert.Equal(t, expected, body)
}
