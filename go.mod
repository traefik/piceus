module github.com/traefik/piceus

go 1.23.0

require (
	github.com/ettle/strcase v0.2.0
	github.com/google/go-cmp v0.7.0
	github.com/google/go-github/v57 v57.0.0
	github.com/http-wasm/http-wasm-host-go v0.6.0
	github.com/juliens/wasm-goexport v0.0.6
	github.com/ldez/grignotin v0.9.0
	github.com/mitchellh/mapstructure v1.5.0
	github.com/pelletier/go-toml v1.9.5
	github.com/rs/zerolog v1.31.0
	github.com/stealthrocket/wasi-go v0.8.0
	github.com/stealthrocket/wazergo v0.19.1
	github.com/stretchr/testify v1.10.0
	github.com/tetratelabs/wazero v1.7.2
	github.com/traefik/paerser v0.2.0
	github.com/traefik/yaegi v0.16.1
	github.com/urfave/cli/v2 v2.27.2
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.62.0
	go.opentelemetry.io/otel v1.37.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.37.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.37.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.37.0
	go.opentelemetry.io/otel/exporters/stdout/stdoutmetric v1.37.0
	go.opentelemetry.io/otel/metric v1.37.0
	go.opentelemetry.io/otel/sdk v1.37.0
	go.opentelemetry.io/otel/sdk/metric v1.37.0
	go.opentelemetry.io/otel/trace v1.37.0
	golang.org/x/mod v0.25.0
	golang.org/x/oauth2 v0.30.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/BurntSushi/toml v1.4.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.4 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.27.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/xrash/smetrics v0.0.0-20240312152122-5f08fbb34913 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/proto/otlp v1.7.0 // indirect
	golang.org/x/exp v0.0.0-20240404231335-c0f41cb1a7a0 // indirect
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250603155806-513f23925822 // indirect
	google.golang.org/grpc v1.73.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
)

retract (
	v1.13.0 // error during tag creation
	v1.12.2 // error during tag creation
	v1.10.2 // error during tag creation
)

replace github.com/http-wasm/http-wasm-host-go => github.com/traefik/http-wasm-host-go v0.0.0-20240618100324-3c53dcaa1a70
