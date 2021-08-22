# Camponotus Piceus

- [builds](https://pilot.semaphoreci.com/projects/piceus)

## CLI

```
NAME:
   piceus run - Run Piceus

USAGE:
   piceus run [command options] [arguments...]

DESCRIPTION:
   Launch application piceus

OPTIONS:
   --github-token value           GitHub Token. [$GITHUB_TOKEN]
   --services-access-token value  Pilot Services Access Token [$PILOT_SERVICES_ACCESS_TOKEN]
   --plugin-url value             Plugin Service URL [$PILOT_PLUGIN_URL]
   --tracing-endpoint value       Endpoint to send traces (default: "https://collector.infra.traefiklabs.tech") [$TRACING_ENDPOINT]
   --tracing-username value       Username to connect to Jaeger (default: "jaeger") [$TRACING_USERNAME]
   --tracing-password value       Password to connect to Jaeger (default: "jaeger") [$TRACING_PASSWORD]
   --tracing-probability value    Probability to send traces. (default: 0) [$TRACING_PROBABILITY]
   --help, -h                     show help (default: false)
```

extra:

- `PICEUS_PRIVATE_MODE`: uses GitHub instead of GoProxy.
