# Camponotus Piceus

- [builds](https://traefiklabs.semaphoreci.com/projects/piceus)

## CLI

```
NAME:
   Piceus CLI run - Run Piceus

USAGE:
   Piceus CLI run [command options] [arguments...]

DESCRIPTION:
   Launch application piceus

OPTIONS:
   --log-level value            Log level (default: "info") [$LOG_LEVEL]
   --github-token value         GitHub Token. Deprecated: use --github-token-file. [$GITHUB_TOKEN]
   --github-token-file value    Path of a file that contains the GitHub Token. [$GITHUB_TOKEN_FILE]
   --plugin-url value           Plugin Service URL [$PLUGIN_URL]
   --evaluation-timeout value   Maximum duration of one plugin evaluation. Below 60s, slow WASM checks are stopped by this timeout. (default: 1m30s) [$EVALUATION_TIMEOUT]
   --tracing-address value      Address to send traces (default: "jaeger.jaeger.svc.cluster.local:4318") [$TRACING_ADDRESS]
   --tracing-insecure           use HTTP instead of HTTPS (default: true) [$TRACING_INSECURE]
   --tracing-username value     Username to connect to Jaeger (default: "jaeger") [$TRACING_USERNAME]
   --tracing-password value     Password to connect to Jaeger (default: "jaeger") [$TRACING_PASSWORD]
   --tracing-probability value  Probability to send traces (default: 0) [$TRACING_PROBABILITY]
   --help, -h                   show help
```

extra:

- `PICEUS_PRIVATE_MODE`: uses GitHub instead of GoProxy.

## Plugin evaluation sandbox

Piceus runs the code of each plugin in a child process (`piceus evaluate`):

- The child process has an empty environment.
- Landlock limits the file system access to the plugin directory and to the system files that TLS, DNS and time zones need.
  On Linux 6.7 or later, Landlock also limits TCP connections to the ports 443 and 53.
- seccomp blocks `execve`, `execveat`, `ptrace` and `process_vm_readv`/`process_vm_writev`.
- Piceus kills the child process group at the end of the evaluation timeout (`--evaluation-timeout`, 90 seconds by default).

Piceus requires Linux with Landlock and seccomp. `piceus run` does not start if the kernel or the container runtime blocks them.
Network isolation (cloud metadata endpoint, cluster services) must be done by the infrastructure.
