// Package render turns the document model into a self-contained HTML file.
package render

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html"
	"strings"

	"github.com/GzaJai/cdocs/internal/model"
)

// templateHTML is embedded in the binary so a cdocs install is a single file
// with no external assets to find at runtime.
//
//go:embed template.html
var templateHTML string

// Render embeds the model into the template.
//
// The JSON is embedded rather than fetched because browsers block fetch over
// file://, and the document has to work by double-clicking it.
func Render(data model.Data) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		return nil, err
	}

	payload := strings.TrimSuffix(buffer.String(), "\n")
	// A literal </script> inside a JSON string would close the script tag.
	payload = strings.ReplaceAll(payload, "</", `<\/`)

	output := strings.NewReplacer(
		"__TITLE__", html.EscapeString(data.Meta.Title),
		"__BASE_URL__", html.EscapeString(data.Meta.BaseURL),
		"__API_DATA__", payload,
	).Replace(templateHTML)

	return []byte(output), nil
}
