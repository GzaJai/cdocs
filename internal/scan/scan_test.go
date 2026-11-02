package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/GzaJai/cdocs/internal/model"
)

func TestHumanize(t *testing.T) {
	cases := map[string]string{
		"listar":       "Listar",
		"getAll":       "Get All",
		"crearCupon":   "Crear Cupon",
		"ObtenerTodos": "Obtener Todos",
		"":             "",
		"a":            "A",
		"HTTPGet":      "H T T P Get",
	}
	for input, want := range cases {
		if got := Humanize(input); got != want {
			t.Errorf("Humanize(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSplitTop(t *testing.T) {
	got := SplitTop(`@PathVariable Long id, @RequestBody Map<String, Object> body, @RequestParam(required = false) String q`)
	want := []string{
		"@PathVariable Long id",
		"@RequestBody Map<String, Object> body",
		"@RequestParam(required = false) String q",
	}
	if len(got) != len(want) {
		t.Fatalf("SplitTop returned %d parts, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("part %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSplitTopDropsEmptyParts(t *testing.T) {
	if got := SplitTop(" , "); len(got) != 0 {
		t.Fatalf("expected no parts, got %v", got)
	}
}

func TestMatchClose(t *testing.T) {
	text := `a(b(c)d)e`
	if got := MatchClose(text, 1, '(', ')'); got != 7 {
		t.Errorf("MatchClose = %d, want 7", got)
	}
	if got := MatchClose("a(b", 1, '(', ')'); got != -1 {
		t.Errorf("unbalanced MatchClose = %d, want -1", got)
	}
}

func TestSpringRegexp(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"/api/v1/cupones", "/api/v1/cupones", true},
		{"/api/v1/cupones", "/api/v1/cupones/1", false},
		{"/api/v1/cupones/**", "/api/v1/cupones", true},
		{"/api/v1/cupones/**", "/api/v1/cupones/1/pagar", true},
		{"/api/v1/*/estado", "/api/v1/cupones/estado", true},
		{"/api/v1/*/estado", "/api/v1/a/b/estado", false},
		{"/api/v1/{id}", "/api/v1/42", true},
		{"/**", "/anything/at/all", true},
	}
	for _, c := range cases {
		if got := springRegexp(c.pattern).MatchString(c.path); got != c.want {
			t.Errorf("springRegexp(%q).Match(%q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestParseRoles(t *testing.T) {
	cases := []struct {
		expr string
		kind string
		role []string
	}{
		{`hasRole('ADMIN')`, "list", []string{"ADMIN"}},
		{`hasAnyRole("ADMIN", "ROLE_PROPIETARIO")`, "list", []string{"ADMIN", "PROPIETARIO"}},
		{`isAuthenticated()`, "authenticated", nil},
		{`permitAll()`, "public", nil},
		{`denyAll()`, "denied", nil},
		{`somethingElse()`, "", nil},
	}
	for _, c := range cases {
		got := ParseRoles(c.expr)
		if c.kind == "" {
			if got != nil {
				t.Errorf("ParseRoles(%q) = %+v, want nil", c.expr, got)
			}
			continue
		}
		if got == nil {
			t.Fatalf("ParseRoles(%q) = nil, want kind %q", c.expr, c.kind)
		}
		if got.kind != c.kind {
			t.Errorf("ParseRoles(%q).kind = %q, want %q", c.expr, got.kind, c.kind)
		}
		if len(got.roles) != len(c.role) {
			t.Fatalf("ParseRoles(%q).roles = %v, want %v", c.expr, got.roles, c.role)
		}
		for i := range c.role {
			if got.roles[i] != c.role[i] {
				t.Errorf("ParseRoles(%q).roles[%d] = %q, want %q", c.expr, i, got.roles[i], c.role[i])
			}
		}
	}
}

func TestEffectiveAccess(t *testing.T) {
	permitAll := "permitAll()"
	adminOnly := "hasRole('ADMIN')"
	authenticated := "isAuthenticated()"

	t.Run("no preauthorize keeps the url rule", func(t *testing.T) {
		got := EffectiveAccess(model.Roles([]string{"ADMIN"}), nil)
		if !got.Equal(model.Roles([]string{"ADMIN"})) {
			t.Errorf("got %v", got)
		}
	})

	t.Run("method permitAll wins over a role rule", func(t *testing.T) {
		// The previous implementation intersected the characters of "public"
		// with the role list and silently kept the role rule.
		got := EffectiveAccess(model.Roles([]string{"ADMIN"}), &permitAll)
		if !got.Equal(model.Public()) {
			t.Errorf("got %v, want public", got)
		}
	})

	t.Run("role list overrides a public url rule", func(t *testing.T) {
		got := EffectiveAccess(model.Public(), &adminOnly)
		if !got.Equal(model.Roles([]string{"ADMIN"})) {
			t.Errorf("got %v, want [ADMIN]", got)
		}
	})

	t.Run("roles are intersected", func(t *testing.T) {
		got := EffectiveAccess(model.Roles([]string{"ADMIN", "USER"}), &adminOnly)
		if !got.Equal(model.Roles([]string{"ADMIN"})) {
			t.Errorf("got %v, want [ADMIN]", got)
		}
	})

	t.Run("disjoint roles keep the url rule", func(t *testing.T) {
		got := EffectiveAccess(model.Roles([]string{"USER"}), &adminOnly)
		if !got.Equal(model.Roles([]string{"USER"})) {
			t.Errorf("got %v, want [USER]", got)
		}
	})

	t.Run("authenticated does not demote a role rule", func(t *testing.T) {
		got := EffectiveAccess(model.Roles([]string{"ADMIN"}), &authenticated)
		if !got.Equal(model.Roles([]string{"ADMIN"})) {
			t.Errorf("got %v, want [ADMIN]", got)
		}
	})

	t.Run("authenticated wins over a public url rule", func(t *testing.T) {
		got := EffectiveAccess(model.Public(), &authenticated)
		if !got.Equal(model.Authenticated()) {
			t.Errorf("got %v, want authenticated", got)
		}
	})
}

func TestURLAccess(t *testing.T) {
	rules := []Rule{
		{HTTP: "POST", Path: "/api/v1/auth/**", Access: model.Public()},
		{HTTP: "", Path: "/api/v1/admin/**", Access: model.Roles([]string{"ADMIN"})},
	}

	if got := URLAccess("POST", "/api/v1/auth/login", rules, true); !got.Equal(model.Public()) {
		t.Errorf("POST /auth/login = %v, want public", got)
	}
	if got := URLAccess("GET", "/api/v1/auth/login", rules, true); !got.Equal(model.Authenticated()) {
		t.Errorf("GET /auth/login = %v, want authenticated", got)
	}
	if got := URLAccess("GET", "/api/v1/admin/users", rules, true); !got.Equal(model.Roles([]string{"ADMIN"})) {
		t.Errorf("GET /admin/users = %v, want [ADMIN]", got)
	}
	if got := URLAccess("GET", "/api/v1/public", rules, false); !got.Equal(model.Public()) {
		t.Errorf("unsecured project = %v, want public", got)
	}
}

func TestModuleOf(t *testing.T) {
	repo := "/repo"
	cases := map[string]string{
		"/repo/src/main/java/com/acme/backend/cupones/controller/CuponesController.java": "cupones",
		"/repo/src/main/java/com/acme/backend/controller/CuponesController.java":         "backend",
		"/repo/src/main/java/com/acme/backend/controller/nested/CuponesController.java":  "backend",
		"/repo/src/main/java/controller/FooController.java":                              "API",
		"/repo/CuponesController.java":                                                   "API",
	}
	for path, want := range cases {
		if got := moduleOf(filepath.FromSlash(path), filepath.FromSlash(repo)); got != want {
			t.Errorf("moduleOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestExtractFixture(t *testing.T) {
	repo := t.TempDir()

	javaDir := filepath.Join(repo, "src", "main", "java", "com", "acme", "cupones", "controller")
	if err := os.MkdirAll(javaDir, 0o755); err != nil {
		t.Fatal(err)
	}

	controller := `package com.acme.cupones.controller;

@RestController
@RequestMapping("/api/v1/cupones")
public class CuponesController {

    @GetMapping("/{id}")
    @PreAuthorize("hasRole('ADMIN')")
    public CuponResponse obtener(@PathVariable Long id) {
        return null;
    }

    @PostMapping
    public ResponseEntity<Void> crear(@RequestBody CrearCuponRequest request,
                                      @RequestParam(required = false) String origen) {
        return ResponseEntity.status(HttpStatus.CREATED).build();
    }

    @DeleteMapping("/{id}")
    public void borrar(@PathVariable Long id, Authentication auth) {
        return;
    }
}
`
	writeFile(t, filepath.Join(javaDir, "CuponesController.java"), controller)

	securityDir := filepath.Join(repo, "src", "main", "java", "com", "acme", "config")
	if err := os.MkdirAll(securityDir, 0o755); err != nil {
		t.Fatal(err)
	}
	security := `@Configuration
public class SecurityConfig {
    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(auth -> auth
            .requestMatchers("/api/v1/cupones/**").authenticated()
            .anyRequest().permitAll());
        return http.build();
    }
}
`
	writeFile(t, filepath.Join(securityDir, "SecurityConfig.java"), security)

	writeFile(t, filepath.Join(repo, "pom.xml"),
		`<dependency><artifactId>spring-boot-starter-security</artifactId></dependency>`)

	result, err := Extract(repo)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	endpoints := result.BySegment["cupones"]
	if len(endpoints) != 3 {
		t.Fatalf("got %d endpoints, want 3: %+v", len(endpoints), endpoints)
	}

	byKey := map[string]model.Endpoint{}
	for _, endpoint := range endpoints {
		byKey[Key(endpoint.Method, endpoint.Path)] = endpoint
	}

	index, ok := byKey["GET /api/v1/cupones/{id}"]
	if !ok {
		t.Fatalf("missing GET endpoint, got keys %v", keys(byKey))
	}
	if !index.Access.Equal(model.Roles([]string{"ADMIN"})) {
		t.Errorf("GET access = %v, want [ADMIN]", index.Access)
	}
	if index.Title != "Obtener" {
		t.Errorf("GET title = %q", index.Title)
	}
	if index.Returns != "CuponResponse" {
		t.Errorf("GET returns = %q", index.Returns)
	}
	if len(index.Params) != 1 || index.Params[0].In != "path" || index.Params[0].Name != "id" {
		t.Errorf("GET params = %+v", index.Params)
	}

	create, ok := byKey["POST /api/v1/cupones"]
	if !ok {
		t.Fatalf("missing POST endpoint, got keys %v", keys(byKey))
	}
	if create.Access.Equal(model.Roles([]string{"ADMIN"})) {
		t.Error("POST must not inherit the sibling's @PreAuthorize")
	}
	if !create.Access.Equal(model.Authenticated()) {
		t.Errorf("POST access = %v, want authenticated", create.Access)
	}
	if create.Responses[0].Status != "201" {
		t.Errorf("POST status = %q, want 201", create.Responses[0].Status)
	}
	if len(create.Params) != 2 {
		t.Fatalf("POST params = %+v", create.Params)
	}
	if create.Params[0].In != "body" || create.Params[0].Name != "body" {
		t.Errorf("POST body param = %+v", create.Params[0])
	}
	if create.Params[1].In != "query" || create.Params[1].Required {
		t.Errorf("POST query param = %+v", create.Params[1])
	}

	del, ok := byKey["DELETE /api/v1/cupones/{id}"]
	if !ok {
		t.Fatalf("missing DELETE endpoint, got keys %v", keys(byKey))
	}
	if len(del.Params) != 1 {
		t.Errorf("DELETE must ignore Authentication parameters, got %+v", del.Params)
	}
}

func TestExtractWithoutControllersFails(t *testing.T) {
	if _, err := Extract(t.TempDir()); err == nil {
		t.Fatal("expected an error for a repository without controllers")
	}
}

func TestDetectBaseURL(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, "src", "main", "resources", "application.yml"),
		"server:\n  port: 9090\n  servlet:\n    context-path: /api\n")

	if got := DetectBaseURL(repo); got != "http://localhost:9090/api" {
		t.Errorf("DetectBaseURL = %q", got)
	}
}

func TestDetectBaseURLDefaults(t *testing.T) {
	if got := DetectBaseURL(t.TempDir()); got != "http://localhost:8080" {
		t.Errorf("DetectBaseURL = %q, want http://localhost:8080", got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func keys(m map[string]model.Endpoint) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}
