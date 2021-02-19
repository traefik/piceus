package core

import (
	"context"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/google/go-github/v32/github"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/piceus/internal/plugin"
)

type mockPluginClient struct {
	create    func(p plugin.Plugin) error
	update    func(p plugin.Plugin) error
	getByName func(string) (*plugin.Plugin, error)
}

func (f *mockPluginClient) Create(ctx context.Context, p plugin.Plugin) error {
	log.Info().Str("moduleName", p.Name).Msgf("Create: %+v", p)

	if f.create != nil {
		return f.create(p)
	}
	return nil
}

func (f *mockPluginClient) Update(ctx context.Context, p plugin.Plugin) error {
	log.Info().Str("moduleName", p.Name).Msgf("Update: %+v", p)

	if f.update != nil {
		return f.update(p)
	}
	return nil
}

func (f *mockPluginClient) GetByName(ctx context.Context, name string) (*plugin.Plugin, error) {
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
				BannerPath:    "http://example.org/a/banner.png",
				TestData: map[string]interface{}{
					"Headers": map[string]interface{}{
						"Foo": "Bar",
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
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			file, err := os.Open("./fixtures/" + test.filename)
			require.NoError(t, err)

			t.Cleanup(func() { _ = file.Close() })

			b, err := ioutil.ReadAll(file)
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
				getByName: func(name string) (*plugin.Plugin, error) {
					return nil, &plugin.APIError{StatusCode: http.StatusNotFound, Message: "not found"}
				},
			},
		},
		{
			desc: "update",
			pgClient: &mockPluginClient{
				getByName: func(name string) (*plugin.Plugin, error) {
					return &plugin.Plugin{ID: "aaaa", Name: "test"}, nil
				},
			},
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			scrapper := NewScrapper(nil, nil, test.pgClient, nil)

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
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			imgURL := parseImageURL(repo, "v2.0.0", test.imgPath)

			assert.Equal(t, test.expected, imgURL)
		})
	}
}
