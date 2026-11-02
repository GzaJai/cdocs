# cdocs

API documentation for Spring repositories, generated from the source and opened
in your browser. One static binary, no runtime, and **nothing is ever written
into the documented repository**.

```console
$ cd ~/code/my-service
$ cdocs
→ my-service: 133 endpoints en 23 grupos
✓ ~/.cache/cdocs/my-service-c32e9322/index.html
```

## Why

Reading a Spring codebase to answer "what endpoints exist and who can call
them?" means grepping `*Controller.java`, tracing `@PreAuthorize`, and then
jumping to `SecurityConfig` to find out what the URL rules actually allow.
`cdocs` does that pass once and produces a single HTML file you can read like
Swagger, without running Swagger, without a server, and without adding anything
to the project.

## Install

### From a release (recommended)

Download the archive for your platform from the releases page, then:

```bash
tar -xzf cdocs_*_linux_amd64.tar.gz
install -m 755 cdocs ~/.local/bin/cdocs
```

### With Go

```bash
go install github.com/GzaJai/cdocs@latest
```

### From source

```bash
git clone https://github.com/GzaJai/cdocs
cd cdocs
make install
```

`make install` puts the binary in `$(go env GOPATH)/bin`; make sure that
directory is on your `PATH`.

## Usage

```
cdocs [path] [flags]

Flags:
  --out FILE       write the HTML to FILE instead of the cache
  --json           print the extracted document model to stdout and exit
  --config FILE    use FILE as the curated data file
  --base-url URL   override the detected base URL
  --no-open        do not open the browser
  --list           list cached documents
  --clean          remove the cached document for this repository
  --clean-all      remove every cached document
  --version        print the version
  -h, --help       print help
```

`path` defaults to the current directory and may be anywhere inside the
repository. Flags may appear before or after the path.

## Where files go

`cdocs` writes **outside** the documented repository, always:

| What | Where |
| --- | --- |
| Generated HTML | `$XDG_CACHE_HOME/cdocs/<slug>-<hash>/index.html` |
| Cache metadata | `$XDG_CACHE_HOME/cdocs/<slug>-<hash>/cdocs.json` |
| Curated data (optional) | `$XDG_CONFIG_HOME/cdocs/<slug>-<hash>.json` |

On Linux those resolve to `~/.cache` and `~/.config`. On macOS and Windows the
platform directories are used instead (`os.UserCacheDir` / `os.UserConfigDir`).

The key is a slug of the directory name plus a short hash of the absolute path,
so two checkouts that happen to share a directory name never collide.

Existing `~/.config/cdocs/<repo-name>.json` files are still read, so nothing
breaks when upgrading. `cdocs` prints the source it used, and the first time it
reads a legacy name-based or in-repo file it also prints the path to move it to.

## What it reads

- `*Controller.java` — `@RequestMapping`, `@GetMapping`/`@PostMapping` and
  friends, `@PreAuthorize`, and the declared parameters and return type.
- `@ModelAttribute` record DTOs are flattened into query parameters.
- `*Security*.java` — `requestMatchers(...).permitAll()`, `hasRole(...)` and
  `anyRequest()`, translated into the effective access of each endpoint.
- `application*.{properties,yml,yaml}` — `server.port` and
  `server.servlet.context-path` for the base URL.

Access is resolved in this order: method `@PreAuthorize`, class
`@PreAuthorize`, matching `SecurityConfig` URL rule, then the project default
(authenticated when Spring Security is present, public otherwise).

## Curated data

The extractor can only see what the code declares. For real request/response
examples, shared models, an error catalogue and prose conventions, write a
curated file in your config directory.

Generate a starting point and move it into place:

```bash
mkdir -p ~/.config/cdocs
cdocs --json > ~/.config/cdocs/my-service-<hash>.json
```

The exact path for your repository is printed when `cdocs` finds an existing
curated file; it is the same key used for the cache directory.

Curated entries are matched to extracted endpoints by `method` + `path`: an
entry that matches keeps all its hand-written content and inherits the declared
Java return type; an entry that matches nothing is dropped. Every extracted
endpoint not covered by the curated file is still documented automatically.

Minimal example:

```json
{
  "meta": { "baseUrl": "/api/v1" },
  "groups": [
    {
      "name": "Health",
      "endpoints": [
        {
          "method": "GET",
          "path": "/health",
          "title": "Health check",
          "access": "public",
          "responses": [{ "status": "200", "json": { "status": "UP" } }]
        }
      ]
    }
  ]
}
```

Optional top-level keys: `meta`, `authNote`, `conventions`, `changes`, `enums`,
`errors`, `models`, `groupNames`, `groups`.

`groupNames` overrides the display label of a package segment:

```json
{ "groupNames": { "cupones": "Cupones y descuentos" } }
```

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success |
| 1 | Failure (not a repository, no controllers, I/O error) |
| 2 | Invalid usage |

## Development

```bash
make build      # build ./cdocs
make test       # go test ./...
make vet        # go vet ./...
make snapshot   # local goreleaser build, no publish
make release    # goreleaser release (needs GITHUB_TOKEN)
```

## License

MIT. See [LICENSE](LICENSE).
