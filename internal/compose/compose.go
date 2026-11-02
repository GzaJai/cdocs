// Package compose merges extracted endpoints with curated data into the final
// document model.
package compose

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/GzaJai/cdocs/internal/curated"
	"github.com/GzaJai/cdocs/internal/model"
	"github.com/GzaJai/cdocs/internal/scan"
)

// GenericAuthNote is shown when the curated file provides no auth note.
const GenericAuthNote = "«Público» = sin sesión; «Autenticado» = cualquier usuario logueado; " +
	"el resto exige un rol. La visibilidad se infiere de @PreAuthorize y de " +
	"las reglas de SecurityConfig (permitAll / hasRole)."

// GenericErrors is the fallback error catalogue.
var GenericErrors = []model.ErrorEntry{
	{Code: "400", Desc: "Request inválido (validación)"},
	{Code: "401", Desc: "No autenticado"},
	{Code: "403", Desc: "Sin permiso para el rol"},
	{Code: "404", Desc: "Recurso inexistente"},
	{Code: "409", Desc: "Conflicto de negocio"},
	{Code: "500", Desc: "Error inesperado"},
}

// Compose builds the document model for repo.
func Compose(repo string, result *scan.Result, cur *curated.File) model.Data {
	var (
		groups []model.Group
		used   = map[string]bool{}
	)

	if cur != nil {
		for _, group := range cur.Groups {
			var endpoints []any
			for _, endpoint := range group.Endpoints {
				key := scan.Key(stringField(endpoint, "method"), stringField(endpoint, "path"))
				returnType, ok := result.Returns[key]
				if !ok {
					continue
				}
				rich := clone(endpoint)
				rich["returns"] = returnType
				endpoints = append(endpoints, rich)
				used[key] = true
			}
			if len(endpoints) > 0 {
				groups = append(groups, model.Group{
					Name:        group.Name,
					Description: group.Description,
					Endpoints:   endpoints,
				})
			}
		}
	}

	overrides := map[string]string{}
	if cur != nil && cur.GroupNames != nil {
		overrides = cur.GroupNames
	}

	for _, segment := range result.Segments() {
		var endpoints []model.Endpoint
		for _, endpoint := range result.BySegment[segment] {
			if used[scan.Key(endpoint.Method, endpoint.Path)] {
				continue
			}
			endpoints = append(endpoints, endpoint)
		}
		sort.SliceStable(endpoints, func(i, j int) bool {
			if endpoints[i].Path != endpoints[j].Path {
				return endpoints[i].Path < endpoints[j].Path
			}
			return endpoints[i].Method < endpoints[j].Method
		})
		if len(endpoints) == 0 {
			continue
		}

		items := make([]any, len(endpoints))
		for i := range endpoints {
			items[i] = endpoints[i]
		}
		groups = append(groups, model.Group{
			Name:        curated.GroupLabel(segment, overrides),
			Description: "",
			Endpoints:   items,
		})
	}

	total := 0
	for _, group := range groups {
		total += len(group.Endpoints)
	}

	branch := scan.Git(repo, "rev-parse", "--abbrev-ref", "HEAD")
	when := time.Now().Format("2006-01-02 15:04")

	title := fmt.Sprintf("%s · API docs", filepath.Base(repo))
	subtitle := fmt.Sprintf("%d endpoints", total)
	if branch != "" {
		subtitle += " · " + branch
	}
	subtitle += " · " + when

	var (
		baseURL     string
		authNote    string
		conventions = any([]any{})
		changes     = any([]any{})
		enums       = any([]any{})
		models      = any([]any{})
		errors      = any(GenericErrors)
	)
	if cur != nil {
		if cur.Meta != nil {
			if cur.Meta.BaseURL != "" {
				baseURL = cur.Meta.BaseURL
			}
			// The curated file is the author's voice: when it names the
			// document, that name wins over the derived one.
			if cur.Meta.Title != "" {
				title = cur.Meta.Title
			}
			if cur.Meta.Subtitle != "" {
				subtitle = cur.Meta.Subtitle
			}
		}
		if cur.AuthNote != "" {
			authNote = cur.AuthNote
		}
		if !isEmpty(cur.Conventions) {
			conventions = cur.Conventions
		}
		if !isEmpty(cur.Changes) {
			changes = cur.Changes
		}
		if !isEmpty(cur.Enums) {
			enums = cur.Enums
		}
		if !isEmpty(cur.Models) {
			models = cur.Models
		}
		if !isEmpty(cur.Errors) {
			errors = cur.Errors
		}
	}
	if baseURL == "" {
		baseURL = scan.DetectBaseURL(repo)
	}
	if authNote == "" {
		authNote = GenericAuthNote
	}

	return model.Data{
		Meta: model.Meta{
			Title:    title,
			Subtitle: subtitle,
			BaseURL:  baseURL,
		},
		AuthNote:    authNote,
		Conventions: conventions,
		Changes:     changes,
		Enums:       enums,
		Errors:      errors,
		Models:      models,
		Groups:      groups,
	}
}

func stringField(object map[string]any, key string) string {
	value, ok := object[key].(string)
	if !ok {
		return ""
	}
	return value
}

func clone(source map[string]any) map[string]any {
	out := make(map[string]any, len(source)+1)
	for key, value := range source {
		out[key] = value
	}
	return out
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	if items, ok := value.([]any); ok {
		return len(items) == 0
	}
	return false
}
