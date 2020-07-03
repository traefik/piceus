package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"

	"github.com/containous/piceus/internal/plugin"
	"github.com/containous/piceus/pkg/core"
	"github.com/containous/piceus/pkg/sources"
	"github.com/google/go-github/v32/github"
	"github.com/ldez/grignotin/goproxy"
	"golang.org/x/oauth2"
)

type config struct {
	Token       string
	AccessToken string
	PluginURL   string
}

func main() {
	cfg := config{}
	flag.StringVar(&cfg.Token, "token", os.Getenv("GITHUB_TOKEN"), "GitHub Token (GITHUB_TOKEN)")
	flag.StringVar(&cfg.AccessToken, "access-token", os.Getenv("PLAEN_SERVICES_ACCESS_TOKEN"), "Services Access Token (PLAEN_SERVICES_ACCESS_TOKEN)")
	flag.StringVar(&cfg.PluginURL, "plugin-url", os.Getenv("PLAEN_PLUGIN_URL"), "Plugin service base URL (PLAEN_PLUGIN_URL)")

	help := flag.Bool("h", false, "show this help")

	flag.Usage = usage
	flag.Parse()
	if *help {
		usage()
		return
	}

	nArgs := flag.NArg()
	if nArgs > 0 {
		usage()
		return
	}

	err := checkFlags(cfg)
	if err != nil {
		usage()
		log.Fatal(err)
	}

	ctx := context.Background()

	ghClient := newGitHubClient(ctx, cfg.Token)
	gpClient := goproxy.NewClient("")

	var pgClient pluginClient
	if _, ok := os.LookupEnv("PICEUS_SKIP_PERSISTENCE"); ok {
		pgClient = &fakePluginClient{
			getByName: func(name string) (*plugin.Plugin, error) {
				return nil, errors.New("not found")
			},
		}
	} else {
		pgClient = plugin.New(cfg.PluginURL, cfg.AccessToken)
	}

	var srcs core.Sources
	if _, ok := os.LookupEnv(core.PrivateModeEnv); ok {
		srcs = &sources.GitHub{Client: ghClient}
	} else {
		srcs = &sources.GoProxy{Client: gpClient}
	}

	scrapper := core.NewScrapper(ghClient, gpClient, pgClient, srcs)

	err = scrapper.Run(ctx)
	if err != nil {
		log.Fatal(err)
	}
}

func checkFlags(cfg config) error {
	if cfg.Token == "" {
		return errors.New("missing GitHub Token")
	}

	if cfg.PluginURL == "" {
		return errors.New("missing plugin service UR")
	}

	if cfg.AccessToken == "" {
		return errors.New("missing plugin service access token")
	}

	return nil
}

func usage() {
	_, _ = os.Stderr.WriteString("piceus \n")
	flag.PrintDefaults()
}

func newGitHubClient(ctx context.Context, token string) *github.Client {
	if len(token) == 0 {
		return github.NewClient(nil)
	}

	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	return github.NewClient(oauth2.NewClient(ctx, ts))
}

// ---

type pluginClient interface {
	Create(p plugin.Plugin) error
	Update(p plugin.Plugin) error
	GetByName(name string) (*plugin.Plugin, error)
}

type fakePluginClient struct {
	getByName func(string) (*plugin.Plugin, error)
}

func (f *fakePluginClient) Create(p plugin.Plugin) error {
	log.Println("Create:", p.Name)
	log.Printf("info: %+v\n", p)
	return nil
}

func (f *fakePluginClient) Update(p plugin.Plugin) error {
	log.Println("Update:", p.Name)
	log.Printf("info: %+v\n", p)
	return nil
}

func (f *fakePluginClient) GetByName(name string) (*plugin.Plugin, error) {
	return f.getByName(name)
}