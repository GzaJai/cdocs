// Package scan reads a Java/Spring repository and extracts its HTTP surface:
// controllers, their endpoints, the security rules that apply to them, and the
// base URL declared in application config.
package scan

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// SkipDirs are never traversed: build output and IDE metadata.
var SkipDirs = map[string]bool{
	"target":       true,
	"build":        true,
	"out":          true,
	"node_modules": true,
	".git":         true,
	".gradle":      true,
	".idea":        true,
	".mvn":         true,
}

var (
	slugRe        = regexp.MustCompile(`[^a-z0-9]+`)
	multiSlashRe  = regexp.MustCompile(`/+`)
	firstQuotedRe = regexp.MustCompile(`"([^"]*)"`)
)

// Git runs git inside repo and returns trimmed stdout, or "" on any failure.
func Git(repo string, args ...string) string {
	full := append([]string{"-C", repo}, args...)
	out, err := exec.Command("git", full...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// FindRepo returns the git top level for start, falling back to the resolved
// path when start is not inside a repository.
func FindRepo(start string) string {
	abs, err := filepath.Abs(start)
	if err != nil {
		abs = start
	}
	if top := Git(abs, "rev-parse", "--show-toplevel"); top != "" {
		return top
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
}

// Walk visits every regular file under root, pruning SkipDirs. The visitor
// returns false to stop the walk early.
func Walk(root string, visit func(path string) bool) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != root && SkipDirs[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !visit(path) {
			return fs.SkipAll
		}
		return nil
	})
}

// FindFiles returns every file under root whose base name matches keep, in
// sorted order so the result does not depend on filesystem iteration order.
func FindFiles(root string, keep func(name string) bool) []string {
	var found []string
	_ = Walk(root, func(path string) bool {
		if keep(filepath.Base(path)) {
			found = append(found, path)
		}
		return true
	})
	sortStrings(found)
	return found
}

// MatchClose returns the index of the bracket that closes the one at start, or
// -1 when the text is unbalanced.
func MatchClose(text string, start int, open, close byte) int {
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// SplitTop splits text on commas that are not nested inside brackets.
func SplitTop(text string) []string {
	var (
		parts []string
		cur   strings.Builder
		depth int
	)
	for i := 0; i < len(text); i++ {
		ch := text[i]
		switch ch {
		case '<', '(', '[', '{':
			depth++
		case '>', ')', ']', '}':
			depth--
		}
		if ch == ',' && depth == 0 {
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
			continue
		}
		cur.WriteByte(ch)
	}
	parts = append(parts, strings.TrimSpace(cur.String()))

	out := parts[:0]
	for _, part := range parts {
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// Humanize turns a camelCase identifier into a spaced, capitalized label.
func Humanize(name string) string {
	var b strings.Builder
	for i, r := range name {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return out
	}
	runes := []rune(out)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// StatusOf reads the declared status from a method body.
func StatusOf(body string) string {
	if strings.Contains(body, "HttpStatus.CREATED") {
		return "201"
	}
	if strings.Contains(body, "HttpStatus.NO_CONTENT") {
		return "204"
	}
	return "200"
}

// NormalizePath collapses repeated slashes.
func NormalizePath(path string) string {
	return multiSlashRe.ReplaceAllString(path, "/")
}

// Slug converts a repository name into a filesystem-friendly key.
func Slug(name string) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if slug == "" {
		return "repo"
	}
	return slug
}

// FirstQuoted returns the first double-quoted literal in s, or "".
func FirstQuoted(s string) string {
	if m := firstQuotedRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func readFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}
