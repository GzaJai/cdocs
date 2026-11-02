// Package cache owns the generated output. Everything cdocs produces lives
// here, never in the documented repository.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/GzaJai/cdocs/internal/curated"
)

const (
	indexFile = "index.html"
	metaFile  = "cdocs.json"
)

// Meta records what a cache entry documents, so --list can show something
// useful instead of a hash.
type Meta struct {
	Repo        string    `json:"repo"`
	GeneratedAt time.Time `json:"generatedAt"`
	Endpoints   int       `json:"endpoints"`
}

// Entry is one cached document.
type Entry struct {
	Dir       string
	IndexPath string
	Meta      Meta
	ModTime   time.Time
}

// Dir returns the cache root, honouring XDG_CACHE_HOME on Linux and the
// platform default elsewhere.
func Dir() string {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return filepath.Join(".", "cdocs-cache")
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "cdocs")
}

// EntryDir returns the directory holding the document for repo.
func EntryDir(repo string) string {
	return filepath.Join(Dir(), curated.Key(repo))
}

// IndexPath returns the generated HTML path for repo.
func IndexPath(repo string) string {
	return filepath.Join(EntryDir(repo), indexFile)
}

// Write stores the rendered document and its metadata.
func Write(repo string, html []byte, endpoints int) (string, error) {
	dir := EntryDir(repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	indexPath := filepath.Join(dir, indexFile)
	if err := os.WriteFile(indexPath, html, 0o644); err != nil {
		return "", err
	}

	meta := Meta{Repo: repo, GeneratedAt: time.Now(), Endpoints: endpoints}
	encoded, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, metaFile), encoded, 0o644); err != nil {
		return "", err
	}

	return indexPath, nil
}

// List returns every cached document, newest first.
func List() ([]Entry, error) {
	entries, err := os.ReadDir(Dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var out []Entry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(Dir(), entry.Name())
		indexPath := filepath.Join(dir, indexFile)
		info, statErr := os.Stat(indexPath)
		if statErr != nil {
			continue
		}

		var meta Meta
		if data, readErr := os.ReadFile(filepath.Join(dir, metaFile)); readErr == nil {
			_ = json.Unmarshal(data, &meta)
		}
		out = append(out, Entry{Dir: dir, IndexPath: indexPath, Meta: meta, ModTime: info.ModTime()})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ModTime.After(out[j].ModTime) })
	return out, nil
}

// Remove deletes the cached document for repo and reports whether it existed.
func Remove(repo string) (bool, error) {
	dir := EntryDir(repo)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, os.RemoveAll(dir)
}

// RemoveAll deletes the whole cache and returns how many entries were removed.
func RemoveAll() (int, error) {
	entries, err := os.ReadDir(Dir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := os.RemoveAll(filepath.Join(Dir(), entry.Name())); err != nil {
			return removed, fmt.Errorf("removing %s: %w", entry.Name(), err)
		}
		removed++
	}
	return removed, nil
}
