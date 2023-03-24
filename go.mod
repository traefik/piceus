module github.com/traefik/piceus

go 1.19

require (
	github.com/ettle/strcase v0.1.1
	github.com/google/go-cmp v0.5.9
	github.com/google/go-github/v45 v45.2.0
	github.com/ldez/grignotin v0.4.2
	github.com/mitchellh/mapstructure v1.5.0
	github.com/pelletier/go-toml v1.9.5
	github.com/rs/zerolog v1.29.0
	github.com/stretchr/testify v1.8.2
	github.com/traefik/paerser v0.2.0
	github.com/traefik/yaegi v0.15.1
	github.com/urfave/cli/v2 v2.25.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.18.0
	go.opentelemetry.io/otel v0.18.0
	go.opentelemetry.io/otel/exporters/trace/jaeger v0.18.0
	go.opentelemetry.io/otel/sdk v0.18.0
	go.opentelemetry.io/otel/trace v0.18.0
	golang.org/x/mod v0.9.0
	golang.org/x/oauth2 v0.6.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.opentelemetry.io/contrib v0.18.0 // indirect
	go.opentelemetry.io/otel/metric v0.18.0 // indirect
	golang.org/x/crypto v0.7.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	google.golang.org/api v0.40.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.28.0 // indirect
)

retract (
	v1.12.2 // error during tag creation
	v1.10.2 // error during tag creation
)
