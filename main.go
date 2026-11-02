// Command cdocs generates API documentation for a Spring repository from its
// source code and opens it in the browser.
//
// Nothing is written into the documented repository: the generated HTML and its
// metadata live in the user cache directory, and the optional curated file
// lives in the user config directory.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/GzaJai/cdocs/internal/cache"
	"github.com/GzaJai/cdocs/internal/compose"
	"github.com/GzaJai/cdocs/internal/curated"
	"github.com/GzaJai/cdocs/internal/render"
	"github.com/GzaJai/cdocs/internal/scan"
)

// version is overridden at build time with -ldflags.
var version = "dev"

const usageText = `cdocs — API documentation from a Spring repository.

Usage:
  cdocs [path] [flags]

Generates a self-contained HTML document from the *Controller.java files under
the repository at path (default: the current directory). Output goes to the
user cache directory; the documented repository is never modified.

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
  -h, --help       print this help

Curated data:
  Hand-written examples live in a JSON file outside the repository:
    $XDG_CONFIG_HOME/cdocs/<repo-slug>-<hash>.json

Exit codes:
  0 success, 1 failure, 2 invalid usage
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type options struct {
	out      string
	asJSON   bool
	config   string
	baseURL  string
	noOpen   bool
	list     bool
	clean    bool
	cleanAll bool
	version  bool
	path     string
}

func run(args []string, stdout, stderr io.Writer) int {
	opts, code := parseFlags(args, stderr)
	if code != 0 {
		return code
	}

	if opts.version {
		fmt.Fprintf(stdout, "cdocs %s\n", version)
		return 0
	}

	if opts.list {
		return listCached(stdout, stderr)
	}
	if opts.cleanAll {
		return cleanAll(stdout, stderr)
	}

	repo, err := resolveRepo(opts.path)
	if err != nil {
		fmt.Fprintf(stderr, "cdocs: %v\n", err)
		return 1
	}

	if opts.clean {
		removed, removeErr := cache.Remove(repo)
		if removeErr != nil {
			fmt.Fprintf(stderr, "cdocs: %v\n", removeErr)
			return 1
		}
		if !removed {
			fmt.Fprintf(stdout, "cdocs: no cached docs for %s\n", repo)
			return 0
		}
		fmt.Fprintf(stdout, "✓ removed cached docs for %s\n", repo)
		return 0
	}

	return generate(repo, opts, stdout, stderr)
}

func parseFlags(args []string, stderr io.Writer) (options, int) {
	var opts options

	fs := flag.NewFlagSet("cdocs", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	fs.StringVar(&opts.out, "out", "", "write the HTML to FILE")
	fs.BoolVar(&opts.asJSON, "json", false, "print the model to stdout")
	fs.StringVar(&opts.config, "config", "", "curated data file")
	fs.StringVar(&opts.baseURL, "base-url", "", "override the base URL")
	fs.BoolVar(&opts.noOpen, "no-open", false, "do not open the browser")
	fs.BoolVar(&opts.list, "list", false, "list cached documents")
	fs.BoolVar(&opts.clean, "clean", false, "remove cached docs for this repo")
	fs.BoolVar(&opts.cleanAll, "clean-all", false, "remove every cached document")
	fs.BoolVar(&opts.version, "version", false, "print the version")

	if err := fs.Parse(reorderArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(stderr, usageText)
			return opts, 0
		}
		fmt.Fprintf(stderr, "cdocs: %v\n\n%s", err, usageText)
		return opts, 2
	}

	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "cdocs: expected at most one path, got %d\n\n%s", fs.NArg(), usageText)
		return opts, 2
	}
	if fs.NArg() == 1 {
		opts.path = fs.Arg(0)
	}

	return opts, 0
}

// reorderArgs moves positional arguments after the flags.
//
// The standard flag package stops parsing at the first non-flag argument, so
// "cdocs path --json" would otherwise treat --json as a path. Accepting flags
// on either side is worth the small amount of plumbing.
func reorderArgs(args []string) []string {
	valueFlags := map[string]bool{"out": true, "config": true, "base-url": true}

	var flags, positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			flags = append(flags, arg)
			name := strings.TrimLeft(arg, "-")
			if strings.ContainsRune(name, '=') {
				continue
			}
			if valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
			continue
		}
		positional = append(positional, arg)
	}
	return append(flags, positional...)
}

func resolveRepo(path string) (string, error) {
	if path == "" {
		path = "."
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("cannot access %s: %w", path, err)
	}
	if !info.IsDir() {
		path = filepath.Dir(path)
	}
	return scan.FindRepo(path), nil
}

func generate(repo string, opts options, stdout, stderr io.Writer) int {
	result, err := scan.Extract(repo)
	if err != nil {
		fmt.Fprintf(stderr, "cdocs: %v\n", err)
		fmt.Fprintf(stderr, "       is this a Spring repository?\n")
		return 1
	}

	if opts.config != "" {
		if _, statErr := os.Stat(opts.config); statErr != nil {
			fmt.Fprintf(stderr, "cdocs: config not found: %s\n", opts.config)
			return 1
		}
	}

	file, source, err := curated.Load(repo, opts.config)
	if err != nil {
		fmt.Fprintf(stderr, "cdocs: %v\n", err)
		return 1
	}
	if source.Path != "" && source.InRepo {
		// A config file inside the repository is the one location that really
		// is deprecated: it is the only case that couples the tool to a
		// checkout. The name-based config path is a normal, supported fallback.
		fmt.Fprintf(stderr, "cdocs: curated data from %s (deprecated: move it to %s)\n",
			source.Path, curated.Path(repo))
	}

	data := compose.Compose(repo, result, file)
	if opts.baseURL != "" {
		data.Meta.BaseURL = opts.baseURL
	}

	if opts.asJSON {
		return writeJSON(data, stdout, stderr)
	}

	html, err := render.Render(data)
	if err != nil {
		fmt.Fprintf(stderr, "cdocs: %v\n", err)
		return 1
	}

	var outPath string
	if opts.out != "" {
		if err := os.MkdirAll(filepath.Dir(opts.out), 0o755); err != nil {
			fmt.Fprintf(stderr, "cdocs: %v\n", err)
			return 1
		}
		if err := os.WriteFile(opts.out, html, 0o644); err != nil {
			fmt.Fprintf(stderr, "cdocs: %v\n", err)
			return 1
		}
		outPath = opts.out
	} else {
		outPath, err = cache.Write(repo, html, data.Total())
		if err != nil {
			fmt.Fprintf(stderr, "cdocs: %v\n", err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "→ %s: %d endpoints en %d grupos\n",
		filepath.Base(repo), data.Total(), len(data.Groups))
	fmt.Fprintf(stdout, "✓ %s\n", outPath)

	if opts.noOpen {
		return 0
	}
	if err := openBrowser(outPath); err != nil {
		fmt.Fprintf(stderr, "cdocs: %v; open the path above manually\n", err)
	}
	return 0
}

func writeJSON(data any, stdout, stderr io.Writer) int {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(stderr, "cdocs: %v\n", err)
		return 1
	}
	return 0
}

func listCached(stdout, stderr io.Writer) int {
	entries, err := cache.List()
	if err != nil {
		fmt.Fprintf(stderr, "cdocs: %v\n", err)
		return 1
	}
	if len(entries) == 0 {
		fmt.Fprintln(stdout, "cdocs: cache is empty")
		return 0
	}
	for _, entry := range entries {
		repo := entry.Meta.Repo
		if repo == "" {
			repo = filepath.Base(entry.Dir)
		}
		fmt.Fprintf(stdout, "%-60s  %s\n", repo, entry.IndexPath)
	}
	return 0
}

func cleanAll(stdout, stderr io.Writer) int {
	removed, err := cache.RemoveAll()
	if err != nil {
		fmt.Fprintf(stderr, "cdocs: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "✓ removed %d cached document(s)\n", removed)
	return 0
}

func openBrowser(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}

	var candidates [][]string
	switch runtime.GOOS {
	case "darwin":
		candidates = [][]string{{"open", absolute}}
	case "windows":
		candidates = [][]string{{"rundll32", "url.dll,FileProtocolHandler", absolute}}
	default:
		candidates = [][]string{
			{"xdg-open", absolute},
			{"wslview", absolute},
			{"sensible-browser", absolute},
		}
	}

	var lastErr error
	for _, candidate := range candidates {
		bin, lookupErr := exec.LookPath(candidate[0])
		if lookupErr != nil {
			lastErr = lookupErr
			continue
		}
		cmd := exec.Command(bin, candidate[1:]...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if startErr := cmd.Start(); startErr != nil {
			lastErr = startErr
			continue
		}
		go func() { _ = cmd.Wait() }()
		return nil
	}

	if lastErr == nil {
		lastErr = errors.New("no browser launcher found")
	}
	return lastErr
}
