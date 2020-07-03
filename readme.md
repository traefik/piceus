# Camponotus Piceus

## CLI

```
piceus 
  -access-token string
        Services Access Token (PLAEN_SERVICES_ACCESS_TOKEN)
  -h    show this help
  -plugin-url string
        Plugin service base URL (PLAEN_PLUGIN_URL)
  -token string
        GitHub Token (GITHUB_TOKEN)
```

extra:

- `PICEUS_SKIP_PERSISTENCE`: skips the call to the plugin service.
- `PICEUS_PRIVATE_MODE`: uses GitHub instead of GoProxy.

## TODO

- [ ] define the manifest filename (`.traefik.yml`?)
- [ ] improve issue in the plugin repo when error
- [x] when public: replace GH by GoProxy for archive and versions.
- [x] storage: plugins info
- [x] storage: blacklist
- [x] simple CLI
- [x] get module name
- [x] use goproxy (versions)
- [x] use goproxy (archive)
