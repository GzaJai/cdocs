// Package curated loads the optional hand-written data file that enriches the
// generated documentation: real request/response examples, shared models, the
// error catalogue, and prose conventions.
//
// The file lives outside the documented repository so that generating docs
// never writes anything into a project checkout.
package curated

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GzaJai/cdocs/internal/scan"
)

// Meta is the document header override.
type Meta struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	BaseURL  string `json:"baseUrl"`
}

// Group is a curated group. Endpoints are kept as decoded JSON so that
// hand-written examples and any extra keys survive untouched.
type Group struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Endpoints   []map[string]any `json:"endpoints"`
}

// File is the curated document.
type File struct {
	Meta        *Meta             `json:"meta"`
	AuthNote    string            `json:"authNote"`
	Conventions any               `json:"conventions"`
	Changes     any               `json:"changes"`
	Enums       any               `json:"enums"`
	Errors      any               `json:"errors"`
	Models      any               `json:"models"`
	GroupNames  map[string]string `json:"groupNames"`
	Groups      []Group           `json:"groups"`
}

// Source describes where a curated file was found.
type Source struct {
	Path   string
	Legacy bool
	InRepo bool
}

// ConfigDir returns the directory that holds curated files.
func ConfigDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "."
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "cdocs")
}

// Key returns the stable identifier for a repository: a slug plus a short hash
// of its absolute path. Two checkouts with the same directory name get
// different keys.
func Key(repo string) string {
	abs, err := filepath.Abs(repo)
	if err != nil {
		abs = repo
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	sum := sha1.Sum([]byte(abs))
	return fmt.Sprintf("%s-%s", scan.Slug(filepath.Base(abs)), hex.EncodeToString(sum[:])[:8])
}

// Path returns the canonical curated file path for a repository.
func Path(repo string) string {
	return filepath.Join(ConfigDir(), Key(repo)+".json")
}

// Load returns the curated data for repo, or nil when none exists.
//
// Lookup order: the explicit path, the hashed key, the legacy name-only file,
// then the legacy in-repo file. The last one is read for backwards
// compatibility and reported as deprecated; nothing is ever written there.
func Load(repo, explicit string) (*File, Source, error) {
	type candidate struct {
		path   string
		legacy bool
		inRepo bool
	}

	var candidates []candidate
	if explicit != "" {
		candidates = append(candidates, candidate{path: explicit})
	}
	candidates = append(candidates,
		candidate{path: Path(repo)},
		candidate{path: filepath.Join(ConfigDir(), filepath.Base(repo)+".json"), legacy: true},
		candidate{path: filepath.Join(repo, "docs", "api-doc", "cupones-descuentos.json"), legacy: true, inRepo: true},
	)

	for _, c := range candidates {
		data, err := os.ReadFile(c.path)
		if err != nil {
			continue
		}
		file, parseErr := decode(data)
		if parseErr != nil {
			return nil, Source{Path: c.path}, fmt.Errorf("cannot parse %s: %w", c.path, parseErr)
		}
		return file, Source{Path: c.path, Legacy: c.legacy, InRepo: c.inRepo}, nil
	}

	return nil, Source{}, nil
}

func decode(data []byte) (*File, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var file File
	if err := decoder.Decode(&file); err != nil {
		return nil, err
	}
	return &file, nil
}

// GroupLabel turns a package segment into a display label, preferring an
// explicit override from the curated file.
func GroupLabel(segment string, overrides map[string]string) string {
	if label, ok := overrides[strings.ToLower(segment)]; ok {
		return label
	}
	if label, ok := DefaultGroupNames[strings.ToLower(segment)]; ok {
		return label
	}
	return scan.Humanize(strings.NewReplacer("_", " ", "-", " ").Replace(segment))
}

// DefaultGroupNames maps common domain segments to display labels. Projects can
// override or extend this with the groupNames key of their curated file.
var DefaultGroupNames = map[string]string{
	"admin":         "Administración",
	"afiliaciones":  "Afiliaciones",
	"auth":          "Auth",
	"finanzas":      "Finanzas",
	"horario":       "Horarios",
	"horarios":      "Horarios",
	"informe":       "Informes",
	"informes":      "Informes",
	"mascota":       "Mascotas",
	"mascotas":      "Mascotas",
	"medico":        "Médicos",
	"medicos":       "Médicos",
	"paciente":      "Pacientes",
	"pacientes":     "Pacientes",
	"prestador":     "Prestadores",
	"prestadores":   "Prestadores",
	"propietario":   "Propietario",
	"recordatorio":  "Recordatorios",
	"recordatorios": "Recordatorios",
	"servicio":      "Servicios",
	"servicios":     "Servicios",
	"turno":         "Turnos",
	"turnos":        "Turnos",
	"usuario":       "Usuarios",
	"usuarios":      "Usuarios",
	"veterinaria":   "Veterinarias",
	"veterinarias":  "Veterinarias",
}
