module github.com/traefik/piceus

go 1.17

require (
	github.com/ettle/strcase v0.1.1
	github.com/google/go-cmp v0.5.6
	github.com/google/go-github/v38 v38.1.0
	github.com/ldez/grignotin v0.4.1
	github.com/mitchellh/mapstructure v1.4.1
	github.com/pelletier/go-toml v1.9.3
	github.com/rs/zerolog v1.23.0
	github.com/stretchr/testify v1.7.0
	github.com/traefik/paerser v0.1.4
	github.com/traefik/yaegi v0.10.0
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

require (
	github.com/BurntSushi/toml v0.3.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.0-20190314233015-f79a8a8ca69d // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/felixge/httpsnoop v1.0.1 // indirect
	github.com/golang/protobuf v1.4.3 // indirect
	github.com/google/go-querystring v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.0.1 // indirect
	github.com/shurcooL/sanitized_anchor_name v1.0.0 // indirect
	go.opentelemetry.io/contrib v0.18.0 // indirect
	go.opentelemetry.io/otel/metric v0.18.0 // indirect
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9 // indirect
	golang.org/x/net v0.0.0-20201209123823-ac852fbbde11 // indirect
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
	google.golang.org/api v0.40.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/protobuf v1.25.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
)

retract v1.10.2 // error during tag creation
