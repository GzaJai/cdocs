package scan

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/GzaJai/cdocs/internal/model"
)

var (
	// The original Python pattern used a negative lookahead to skip type
	// declarations. RE2 has no lookarounds, so the declaration keywords are
	// filtered after the match instead.
	sigRe     = regexp.MustCompile(`(?m)^[\t ]*public\s+([\w.$<>\[\], ?|]+?)\s+(\w+)\s*\(`)
	mapRe     = regexp.MustCompile(`@(Get|Post|Put|Patch|Delete)Mapping\b\s*(\((?:[^()]|\([^()]*\))*\))?`)
	preRe     = regexp.MustCompile(`@PreAuthorize\s*\(\s*"([^"]*)"\s*\)`)
	closeRe   = regexp.MustCompile(`(?m)^[\t ]{0,4}\}[ \t]*$`)
	recordRe  = regexp.MustCompile(`(?s)record\s+(\w+)\s*\((.*?)\)\s*\{`)
	annRe     = regexp.MustCompile(`@\w+(\s*\((?:[^()]|\([^()]*\))*\))?`)
	baseMapRe = regexp.MustCompile(`@RequestMapping\s*\(([^)]*)\)`)
)

// declarationKeywords are the types that start with "public" but are not
// methods.
var declarationKeywords = map[string]bool{
	"class":     true,
	"interface": true,
	"enum":      true,
	"record":    true,
}

// Result is the raw extraction of a repository, before curated data is merged.
type Result struct {
	// BySegment groups endpoints by their package segment (the controller's
	// parent package, which is usually the domain name).
	BySegment map[string][]model.Endpoint
	// Returns maps "METHOD /path" to the declared Java return type, so
	// curated entries can inherit it.
	Returns map[string]string
}

// Key identifies an endpoint by method and path.
func Key(method, path string) string { return method + " " + path }

// Extract reads every controller under repo and resolves its access rules.
func Extract(repo string) (*Result, error) {
	controllers := FindFiles(repo, func(name string) bool {
		return strings.HasSuffix(name, "Controller.java")
	})
	if len(controllers) == 0 {
		return nil, fmt.Errorf("no *Controller.java found in %s", repo)
	}

	rules := ParseSecurity(repo)
	secured := HasSpringSecurity(repo)

	result := &Result{
		BySegment: map[string][]model.Endpoint{},
		Returns:   map[string]string{},
	}

	for _, path := range controllers {
		segment, raws := extractController(path, repo)
		for _, raw := range raws {
			endpoint := raw.endpoint
			urlAccess := URLAccess(endpoint.Method, endpoint.Path, rules, secured)
			endpoint.Access = EffectiveAccess(urlAccess, raw.pre)
			result.BySegment[segment] = append(result.BySegment[segment], endpoint)
		}
	}

	for _, endpoints := range result.BySegment {
		for _, endpoint := range endpoints {
			result.Returns[Key(endpoint.Method, endpoint.Path)] = endpoint.Returns
		}
	}

	return result, nil
}

// Segments returns the segment names in sorted order.
func (r *Result) Segments() []string {
	names := make([]string, 0, len(r.BySegment))
	for name := range r.BySegment {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// rawEndpoint keeps the unresolved @PreAuthorize expression next to the
// endpoint until the URL rule is known.
type rawEndpoint struct {
	endpoint model.Endpoint
	pre      *string
}

func extractController(path, repo string) (string, []rawEndpoint) {
	src, ok := readFile(path)
	if !ok {
		return "API", nil
	}

	segment := moduleOf(path, repo)
	base := classBase(src)
	classPre := classPre(src)
	classStart := strings.Index(src, "public class")
	if classStart < 0 {
		classStart = 0
	}

	stem := strings.TrimSuffix(filepath.Base(path), ".java")
	className := strings.ReplaceAll(stem, "Controller", "")

	var endpoints []rawEndpoint
	for _, m := range sigRe.FindAllStringSubmatchIndex(src, -1) {
		returnType := src[m[2]:m[3]]
		if declarationKeywords[strings.TrimSpace(returnType)] {
			continue
		}
		methodName := src[m[4]:m[5]]
		sigStart := m[0]

		paren := m[1] - 1
		parenEnd := MatchClose(src, paren, '(', ')')
		if parenEnd < 0 {
			continue
		}

		block := blockBefore(src, sigStart, classStart)
		mappings := mapRe.FindAllStringSubmatch(block, -1)
		if len(mappings) == 0 {
			continue
		}
		last := mappings[len(mappings)-1]
		httpMethod := last[1]
		subPath := FirstQuoted(last[2])

		raw := base + "/" + strings.TrimLeft(subPath, "/")
		raw = strings.TrimRight(raw, "/")
		full := NormalizePath(raw)
		if full == "" {
			full = "/"
		}

		pre := effectivePre(block, classPre)

		body := ""
		if brace := strings.Index(src[parenEnd:], "{"); brace >= 0 {
			brace += parenEnd
			if end := MatchClose(src, brace, '{', '}'); end > 0 {
				body = src[brace : end+1]
			}
		}

		var params []model.Param
		for _, rawParam := range SplitTop(src[paren+1 : parenEnd]) {
			params = append(params, parseParam(rawParam, repo)...)
		}
		if params == nil {
			params = []model.Param{}
		}

		endpoints = append(endpoints, rawEndpoint{
			pre: pre,
			endpoint: model.Endpoint{
				Method:    strings.ToUpper(httpMethod),
				Path:      full,
				Title:     Humanize(methodName),
				Desc:      className,
				Params:    params,
				Returns:   returnType,
				Responses: []model.Response{{Status: StatusOf(body)}},
				Notes:     []string{},
				Errors:    []string{},
			},
		})
	}

	return segment, endpoints
}

// effectivePre returns the method-level @PreAuthorize, falling back to the
// class-level one.
func effectivePre(block string, classPre *string) *string {
	found := preRe.FindAllStringSubmatch(block, -1)
	if len(found) == 0 {
		return classPre
	}
	value := found[len(found)-1][1]
	return &value
}

func moduleOf(path, repo string) string {
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		rel = filepath.Base(path)
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")

	var pkg []string
	fromJava := false
	for i, part := range parts {
		if part == "java" {
			if i+1 <= len(parts)-1 {
				pkg = append(pkg, parts[i+1:]...)
			}
			fromJava = true
			break
		}
	}
	if !fromJava {
		if len(parts) > 1 {
			pkg = append(pkg, parts[:len(parts)-1]...)
		}
	} else if len(pkg) > 0 {
		// Drop the file name.
		pkg = pkg[:len(pkg)-1]
	}

	last := -1
	for i := len(pkg) - 1; i >= 0; i-- {
		if pkg[i] == "controller" {
			last = i
			break
		}
	}
	if last >= 0 {
		if last > 0 {
			return pkg[last-1]
		}
		return "API"
	}
	if len(pkg) > 0 {
		return pkg[len(pkg)-1]
	}
	return "API"
}

func classBase(src string) string {
	classStart := strings.Index(src, "public class")
	head := src
	if classStart >= 0 {
		head = src[:classStart]
	}

	lastArgs := ""
	for _, m := range baseMapRe.FindAllStringSubmatch(head, -1) {
		lastArgs = m[1]
	}
	if lastArgs == "" {
		return ""
	}
	path := FirstQuoted(lastArgs)
	return strings.TrimRight(path, "/")
}

func classPre(src string) *string {
	classStart := strings.Index(src, "public class")
	before := ""
	if classStart >= 0 {
		start := classStart - 900
		if start < 0 {
			start = 0
		}
		before = src[start:classStart]
	}
	found := preRe.FindAllStringSubmatch(before, -1)
	if len(found) == 0 {
		return nil
	}
	value := found[len(found)-1][1]
	return &value
}

func blockBefore(src string, sigStart, classStart int) string {
	best := classStart
	if sigStart > len(src) {
		sigStart = len(src)
	}
	for _, loc := range closeRe.FindAllStringIndex(src[:sigStart], -1) {
		best = loc[1]
	}
	if best < 0 || best > sigStart {
		return ""
	}
	return src[best:sigStart]
}

type RoleSet struct {
	kind  string // list, authenticated, public, denied
	roles []string
}

// ParseRoles interprets a @PreAuthorize expression. It returns nil when the
// expression does not constrain access.
func ParseRoles(expr string) *RoleSet {
	for _, fn := range []string{"hasAnyRole", "hasAnyAuthority", "hasRole", "hasAuthority"} {
		re := regexp.MustCompile(fn + `\s*\(([^)]*)\)`)
		m := re.FindStringSubmatch(expr)
		if m == nil {
			continue
		}
		var roles []string
		for _, raw := range strings.Split(m[1], ",") {
			value := strings.Trim(strings.TrimSpace(raw), `'"`)
			if value == "" {
				continue
			}
			roles = append(roles, strings.TrimPrefix(value, "ROLE_"))
		}
		return &RoleSet{kind: "list", roles: roles}
	}

	switch {
	case strings.Contains(expr, "isAuthenticated"), strings.Contains(expr, "isFullyAuthenticated"):
		return &RoleSet{kind: "authenticated"}
	case strings.Contains(expr, "permitAll"):
		return &RoleSet{kind: "public"}
	case strings.Contains(expr, "denyAll"):
		return &RoleSet{kind: "denied"}
	}
	return nil
}

func parseParam(raw, repo string) []model.Param {
	p := strings.TrimSpace(raw)
	if p == "" || strings.Contains(p, "Authentication") || strings.Contains(p, "Principal") {
		return nil
	}

	isPath := strings.Contains(p, "@PathVariable")
	isModel := strings.Contains(p, "@ModelAttribute")
	isBody := strings.Contains(p, "@RequestBody")
	isQuery := strings.Contains(p, "@RequestParam") || isModel
	isHeader := strings.Contains(p, "@RequestHeader")
	if !isPath && !isBody && !isQuery && !isHeader {
		return nil
	}

	clean := annRe.ReplaceAllString(p, " ")
	tokens := strings.Fields(clean)
	if len(tokens) < 2 {
		return nil
	}
	name := tokens[len(tokens)-1]
	typeName := strings.Join(tokens[:len(tokens)-1], " ")

	switch {
	case isPath:
		return []model.Param{{In: "path", Name: name, Type: typeName, Required: true}}
	case isBody:
		return []model.Param{{
			In:       "body",
			Name:     "body",
			Type:     typeName,
			Required: !strings.Contains(p, "required = false"),
		}}
	case isModel:
		if components := recordComponents(typeName, repo); len(components) > 0 {
			return components
		}
		return []model.Param{{In: "query", Name: typeName, Type: typeName}}
	case isQuery:
		optional := strings.Contains(p, "required = false") || strings.Contains(p, "defaultValue")
		return []model.Param{{In: "query", Name: name, Type: typeName, Required: !optional}}
	default:
		optional := strings.Contains(p, "required = false")
		return []model.Param{{In: "header", Name: name, Type: typeName, Required: !optional}}
	}
}

// recordComponents flattens a record DTO into query parameters.
func recordComponents(typeName, repo string) []model.Param {
	simple := typeName
	if idx := strings.Index(simple, "<"); idx >= 0 {
		simple = simple[:idx]
	}
	simple = strings.TrimSpace(simple)
	if idx := strings.LastIndex(simple, "."); idx >= 0 {
		simple = simple[idx+1:]
	}
	simple = strings.TrimSpace(simple)
	if simple == "" {
		return nil
	}

	target := simple + ".java"
	var components []model.Param
	found := false

	_ = Walk(repo, func(path string) bool {
		if found || filepath.Base(path) != target {
			return true
		}
		text, ok := readFile(path)
		if !ok {
			return true
		}
		m := recordRe.FindStringSubmatch(text)
		if m == nil {
			return true
		}
		for _, part := range SplitTop(m[2]) {
			clean := annRe.ReplaceAllString(part, " ")
			tokens := strings.Fields(clean)
			if len(tokens) >= 2 {
				components = append(components, model.Param{
					In:   "query",
					Name: tokens[len(tokens)-1],
					Type: strings.Join(tokens[:len(tokens)-1], " "),
				})
			}
		}
		found = true
		return false
	})

	if !found {
		return nil
	}
	return components
}
