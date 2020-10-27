package core

import (
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"github.com/google/go-github/v32/github"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/piceus/internal/plugin"
	"gopkg.in/yaml.v3"
)

type mockPluginClient struct {
	create    func(p plugin.Plugin) error
	update    func(p plugin.Plugin) error
	getByName func(string) (*plugin.Plugin, error)
}

func (f *mockPluginClient) Create(p plugin.Plugin) error {
	log.Info().Str("moduleName", p.Name).Msgf("Create: %+v", p)

	if f.create != nil {
		return f.create(p)
	}
	return nil
}

func (f *mockPluginClient) Update(p plugin.Plugin) error {
	log.Info().Str("moduleName", p.Name).Msgf("Update: %+v", p)

	if f.update != nil {
		return f.update(p)
	}
	return nil
}

func (f *mockPluginClient) GetByName(name string) (*plugin.Plugin, error) {
	if f.getByName != nil {
		return f.getByName(name)
	}
	return nil, nil
}

func TestReadManifest(t *testing.T) {
	file, err := os.Open("./fixtures/" + manifestFile)
	require.NoError(t, err)

	defer func() { _ = file.Close() }()

	m := Manifest{}
	err = yaml.NewDecoder(file).Decode(&m)
	require.NoError(t, err)

	expected := Manifest{
		DisplayName:   "Plugin Example",
		Type:          "middleware",
		Import:        "github.com/containous/plugintest/example",
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
	}

	assert.Equal(t, expected, m)
}

func TestReadManifestContent(t *testing.T) {
	file, err := os.Open("./fixtures/" + manifestFile)
	require.NoError(t, err)

	defer func() { _ = file.Close() }()

	b, err := ioutil.ReadAll(file)
	require.NoError(t, err)

	s := Scrapper{}
	m, err := s.loadManifestContent(string(b))
	require.NoError(t, err)

	expected := Manifest{
		DisplayName:   "Plugin Example",
		Type:          "middleware",
		Import:        "github.com/containous/plugintest/example",
		BasePkg:       "example",
		Compatibility: "TODO",
		Summary:       "Simple example plugin.",
		IconPath:      "icon.png",
		BannerPath:    "/a/banner.png",
		TestData: map[string]interface{}{
			"Headers": map[string]interface{}{
				"Foo": "Bar",
			},
		},
	}

	assert.Equal(t, expected, m)
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
			err := scrapper.store(data)

			require.NoError(t, err)
		})
	}
}

func Test_createSnippets(t *testing.T) {
	repository := &github.Repository{
		Name: github.String("plugintest"),
	}

	testData := map[string]interface{}{
		"Headers": map[string]interface{}{
			"Foo": "Bar",
		},
	}

	snippets, err := createSnippets(repository, testData)
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string]interface{}{
		"toml": `
[middlewares]

  [middlewares.my-plugintest]

    [middlewares.my-plugintest.plugin]

      [middlewares.my-plugintest.plugin.plugintest]

        [middlewares.my-plugintest.plugin.plugintest.Headers]
          Foo = "Bar"
`,
		"yaml": `middlewares:
    my-plugintest:
        plugin:
            plugintest:
                Headers:
                    Foo: Bar
`,
	}

	assert.Equal(t, expected, snippets)
}
