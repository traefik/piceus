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
   --github-token value         GitHub Token. [$GITHUB_TOKEN]
   --plugin-url value           Plugin Service URL [$PLUGIN_URL]
   --tracing-address value      Address to send traces (default: "jaeger.jaeger.svc.cluster.local:4318") [$TRACING_ADDRESS]
   --tracing-insecure           use HTTP instead of HTTPS (default: true) [$TRACING_INSECURE]
   --tracing-username value     Username to connect to Jaeger (default: "jaeger") [$TRACING_USERNAME]
   --tracing-password value     Password to connect to Jaeger (default: "jaeger") [$TRACING_PASSWORD]
   --tracing-probability value  Probability to send traces (default: 0) [$TRACING_PROBABILITY]
   --help, -h                   show help
```

extra:

- `PICEUS_PRIVATE_MODE`: uses GitHub instead of GoProxy.

## Catalog commands

They read the catalog from MongoDB (`--mongo-uri`, `--mongo-database`) and probe the plugin service at `--plugin-base-url`.
Commands that write are dry-run unless `--dry-run=false`. See `piceus <command> --help`.

- `audit`: for every recorded hash, download the archive, sha256 it, then call validate with that hash.
  `--check-tags` reports versions whose tag moved after publication, `--compare` diffs against a previous `audit.json`.
- `realign`: record the hash served today for the mismatches of an `audit.json`, guarded on the old value.
  Moved tags are only realigned when named with `--only`.
- `delta-sync`: copy into the catalog what was added to `--source-mongo-uri` after the dump taken at `--since`, then audit the copied hashes.
- `warmup`: download the latest version of the top plugins so the CDN caches them, plugins with dependencies first.
