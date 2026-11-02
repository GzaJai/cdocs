package compose

import (
	"testing"

	"github.com/GzaJai/cdocs/internal/curated"
	"github.com/GzaJai/cdocs/internal/model"
	"github.com/GzaJai/cdocs/internal/scan"
)

func TestComposeKeepsCuratedAndAppendsTheRest(t *testing.T) {
	result := &scan.Result{
		BySegment: map[string][]model.Endpoint{
			"cupones": {
				{Method: "GET", Path: "/api/v1/cupones/{id}", Title: "Obtener", Returns: "CuponResponse", Access: model.Public()},
				{Method: "POST", Path: "/api/v1/cupones", Title: "Crear", Returns: "CuponResponse", Access: model.Public()},
			},
			"auth": {
				{Method: "POST", Path: "/api/v1/auth/login", Title: "Login", Returns: "Token", Access: model.Public()},
			},
		},
		Returns: map[string]string{
			"GET /api/v1/cupones/{id}": "CuponResponse",
			"POST /api/v1/cupones":     "CuponResponse",
			"POST /api/v1/auth/login":  "Token",
		},
	}

	file := &curated.File{
		Groups: []curated.Group{
			{
				Name: "Cupones",
				Endpoints: []map[string]any{
					{
						"method": "GET",
						"path":   "/api/v1/cupones/{id}",
						"title":  "Obtener un cupón",
						"access": "public",
						"body":   map[string]any{"ejemplo": "curado"},
					},
					{
						// No matching endpoint: it must be dropped, not invented.
						"method": "GET",
						"path":   "/api/v1/inventado",
						"title":  "Inventado",
					},
				},
			},
		},
	}

	data := Compose("/tmp/acme-service", result, file)

	// Curated groups first, then the extracted segments in sorted order.
	if len(data.Groups) != 3 {
		t.Fatalf("got %d groups, want 3: %+v", len(data.Groups), data.Groups)
	}

	first := data.Groups[0]
	if first.Name != "Cupones" {
		t.Errorf("first group = %q, want the curated one first", first.Name)
	}
	if len(first.Endpoints) != 1 {
		t.Fatalf("curated group kept %d endpoints, want 1", len(first.Endpoints))
	}
	rich, ok := first.Endpoints[0].(map[string]any)
	if !ok {
		t.Fatalf("curated endpoint lost its map shape: %T", first.Endpoints[0])
	}
	if rich["title"] != "Obtener un cupón" {
		t.Errorf("curated title was overwritten: %v", rich["title"])
	}
	if rich["returns"] != "CuponResponse" {
		t.Errorf("curated endpoint did not inherit the return type: %v", rich["returns"])
	}
	if _, ok := rich["body"]; !ok {
		t.Error("curated body was dropped")
	}

	second := data.Groups[1]
	if second.Name != "Auth" {
		t.Errorf("second group = %q, want Auth (sorted segments)", second.Name)
	}

	third := data.Groups[2]
	if third.Name != "Cupones" {
		t.Errorf("third group = %q", third.Name)
	}
	// POST (curated covered GET), then auth/login.
	if len(third.Endpoints) != 1 {
		t.Fatalf("remaining group has %d endpoints, want 1: %+v", len(third.Endpoints), third.Endpoints)
	}
	remaining, ok := third.Endpoints[0].(model.Endpoint)
	if !ok {
		t.Fatalf("extracted endpoint lost its type: %T", third.Endpoints[0])
	}
	if remaining.Method != "POST" || remaining.Path != "/api/v1/cupones" {
		t.Errorf("remaining endpoint = %s %s", remaining.Method, remaining.Path)
	}

	if data.Total() != 3 {
		t.Errorf("Total = %d, want 3", data.Total())
	}
}

func TestComposeUsesGenericFallbacks(t *testing.T) {
	result := &scan.Result{
		BySegment: map[string][]model.Endpoint{
			"salud": {{Method: "GET", Path: "/health", Returns: "String", Access: model.Public()}},
		},
		Returns: map[string]string{"GET /health": "String"},
	}

	data := Compose("/tmp/acme-service", result, nil)

	if data.AuthNote != GenericAuthNote {
		t.Error("missing generic auth note")
	}
	if len(data.Groups) != 1 || data.Groups[0].Name != "Salud" {
		t.Errorf("groups = %+v", data.Groups)
	}
}

func TestComposeGroupNameOverride(t *testing.T) {
	result := &scan.Result{
		BySegment: map[string][]model.Endpoint{
			"cupones": {{Method: "GET", Path: "/c", Returns: "X", Access: model.Public()}},
		},
		Returns: map[string]string{"GET /c": "X"},
	}
	file := &curated.File{GroupNames: map[string]string{"cupones": "Cupones y descuentos"}}

	data := Compose("/tmp/acme-service", result, file)
	if data.Groups[0].Name != "Cupones y descuentos" {
		t.Errorf("group name = %q", data.Groups[0].Name)
	}
}
