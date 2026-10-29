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
  For a public Yaegi plugin, the Go proxy still gives the commit it pinned at publication, the one plugin-service serves: the plugin is read at that commit.

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

## Plugin evaluation sandbox

Piceus runs the code of each plugin in a child process (`piceus evaluate`):

- The child process has an empty environment.
- Landlock limits the file system access to the plugin directory and to the system files that TLS, DNS and time zones need.
  Landlock also limits TCP connections to the ports 443 and 53, and blocks signals to processes outside of the sandbox.
- seccomp blocks `execve`, `execveat`, `ptrace` and `process_vm_readv`/`process_vm_writev`.
- Piceus kills the child process group at the end of the evaluation timeout (`--evaluation-timeout`, 90 seconds by default).

Piceus requires Linux 6.12 or later (Landlock ABI v6) and seccomp. `piceus run` does not start if the kernel or the container runtime blocks them.
Network isolation (cloud metadata endpoint, cluster services) must be done by the infrastructure.

### Test a plugin with `piceus evaluate`

`piceus evaluate` is the command that Piceus runs for each plugin. You can run it yourself to check a plugin in the same sandbox, without GitHub.
It works only on Linux 6.12 or later, with Landlock and seccomp.

1. Build Piceus:

   ```bash
   make build
   ```

2. Create a GOPATH, and copy the plugin to `<GOPATH>/src/<module name>`. The module name is the first line of the `go.mod` of the plugin.
   For example, for [traefik/plugindemo](https://github.com/traefik/plugindemo):

   ```bash
   mkdir -p /tmp/plugin-gopath/src/github.com/traefik/plugindemo
   ```

   ```bash
   cp -r /path/to/plugindemo/. /tmp/plugin-gopath/src/github.com/traefik/plugindemo/
   ```

3. Write the request in `request.json`.
   `manifest` contains the values of the `.traefik.yml` of the plugin, in JSON.
   `workDir` is the only directory that the plugin can write to: use the GOPATH.

   ```json
   {
     "manifest": {
       "type": "middleware",
       "import": "github.com/traefik/plugindemo",
       "testData": {
         "Headers": {
           "X-Demo": "test",
           "X-URL": "{{URL}}"
         }
       }
     },
     "workDir": "/tmp/plugin-gopath",
     "goPath": "/tmp/plugin-gopath"
   }
   ```

   For a WASM plugin, set `"runtime": "wasm"` in `manifest`, and replace `goPath` with `"wasmFile": "<workDir>/plugin.wasm"`.

4. Run the evaluation. `env -i` gives an empty environment, as Piceus does. The result is written on the file descriptor 3:

   ```bash
   env -i ./dist/linux/amd64/piceus evaluate < request.json 3> result.json
   ```

5. Read the result:

   ```bash
   cat result.json
   ```

   - `{}`: the plugin passes the checks.
   - `{"error":"..."}`: the plugin fails. Piceus puts this text in the GitHub issue, cut to 1024 characters.
   - Empty file, exit code 3: the evaluation could not run (sandbox not available, invalid request). The cause is on stderr.
   - Empty file, another exit code: the plugin stopped the process (for example a panic in a goroutine). Piceus creates no issue.

Notes:

- Piceus does not evaluate plugins with `useUnsafe: true`. `piceus evaluate` refuses their `unsafe` imports.
- The plugin runs with the Piceus restrictions: it cannot start programs, and it can only read its own directory and the system files that TLS, DNS and time zones need.
  A plugin that reads other files, or that connects to a TCP port other than 443 and 53 in `New`, fails here and in Piceus.
