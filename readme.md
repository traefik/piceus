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

They read the catalog from MongoDB (`--mongo-uri`, `--mongo-database`) and probe the plugin service at `--plugin-base-url` (required, so a run against one catalog never probes another one's service).
Commands that write are dry-run unless `--dry-run=false`. See `piceus <command> --help`.

- `audit`: for every recorded hash, download the archive, sha256 it, then call validate with that hash.
  `--check-tags` reports versions whose tag moved after publication, `--compare` diffs against a previous `audit.json`.
- `realign`: record the hash served today for the mismatches of an `audit.json`, guarded on the old value.
  The archive is downloaded again past the CDN cache and must match the audit.
  Versions whose tag moved, or was not checked (audit without `--check-tags`), are only realigned when named with `--only`.
- `prune`: remove the versions of an `audit.json` that are not downloadable and are gone upstream
  (no go.mod on the Go module proxy, or no commit on GitHub). A plugin left without versions is hidden.
  Removed versions are recorded in `removedVersions`, so `delta-sync` does not bring them back.
- `delta-sync`: copy into the catalog what was added to `--source-mongo-uri` after the dump taken at `--since`, then audit the copied hashes.
  A plugin written in the catalog meanwhile is left as is and listed in `changed`: run it again.
  Nothing is deleted: plugins the source no longer has are listed in `missingInSource`, to review by hand.
  The blacklist is not copied.
- `warmup`: download the latest version of the top plugins so the CDN caches them, plugins with dependencies first.
