module github.com/traefik/piceus

go 1.22

require (
	github.com/ettle/strcase v0.2.0
	github.com/google/go-cmp v0.6.0
	github.com/google/go-github/v57 v57.0.0
	github.com/http-wasm/http-wasm-host-go v0.5.1
	github.com/ldez/grignotin v0.5.1
	github.com/mitchellh/mapstructure v1.5.0
	github.com/pelletier/go-toml v1.9.5
	github.com/rs/zerolog v1.31.0
	github.com/stretchr/testify v1.8.4
	github.com/traefik/paerser v0.2.0
	github.com/traefik/yaegi v0.15.1
	github.com/urfave/cli/v2 v2.26.0
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.46.1
	go.opentelemetry.io/otel v1.21.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.21.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.21.0
	go.opentelemetry.io/otel/sdk v1.21.0
	go.opentelemetry.io/otel/trace v1.21.0
	golang.org/x/mod v0.14.0
	golang.org/x/oauth2 v0.15.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.16.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/tetratelabs/wazero v1.2.0 // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	go.opentelemetry.io/otel/metric v1.21.0 // indirect
	go.opentelemetry.io/proto/otlp v1.0.0 // indirect
	golang.org/x/net v0.19.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230822172742-b8732ec3820d // indirect
	google.golang.org/grpc v1.59.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
)

retract (
	v1.13.0 // error during tag creation
	v1.12.2 // error during tag creation
	v1.10.2 // error during tag creation
)
