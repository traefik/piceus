module github.com/traefik/piceus

go 1.16

require (
	github.com/google/go-cmp v0.5.6
	github.com/google/go-github/v35 v35.3.0
	github.com/ldez/grignotin v0.4.1
	github.com/mitchellh/mapstructure v1.4.1
	github.com/pelletier/go-toml v1.9.3
	github.com/rs/zerolog v1.23.0
	github.com/stretchr/testify v1.7.0
	github.com/traefik/paerser v0.1.4
	github.com/traefik/yaegi v0.9.23
	github.com/urfave/cli/v2 v2.3.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.18.0
	go.opentelemetry.io/otel v0.18.0
	go.opentelemetry.io/otel/exporters/trace/jaeger v0.18.0
	go.opentelemetry.io/otel/sdk v0.18.0
	go.opentelemetry.io/otel/trace v0.18.0
	golang.org/x/mod v0.5.0
	golang.org/x/oauth2 v0.0.0-20210220000619-9bb904979d93
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
)

retract v1.10.2 // error during tag creation
