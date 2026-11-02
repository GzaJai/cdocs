package scan

import (
	"regexp"
	"strings"
	"sync"

	"github.com/GzaJai/cdocs/internal/model"
)

var (
	secRe = regexp.MustCompile(
		`requestMatchers\s*\(([^)]*)\)\s*\.\s*` +
			`(permitAll|hasRole|hasAnyRole|hasAuthority|hasAnyAuthority|authenticated|denyAll)\s*\(([^)]*)\)`)
	anyRe = regexp.MustCompile(
		`anyRequest\s*\(\s*\)\s*\.\s*` +
			`(permitAll|hasRole|hasAnyRole|hasAuthority|hasAnyAuthority|authenticated|denyAll)\s*\(([^)]*)\)`)
	httpMethodRe = regexp.MustCompile(`HttpMethod\.(GET|POST|PUT|PATCH|DELETE|HEAD|OPTIONS)`)
)

// Rule is one URL-based security rule from SecurityConfig.
type Rule struct {
	HTTP   string // empty means any method
	Path   string
	Access model.Access
}

var (
	patternCache   = map[string]*regexp.Regexp{}
	patternCacheMu sync.Mutex
	neverMatches   = regexp.MustCompile(`[^\s\S]`)
)

// ParseSecurity reads requestMatchers/anyRequest rules from the security
// configuration files of repo.
func ParseSecurity(repo string) []Rule {
	candidates := FindFiles(repo, func(name string) bool {
		return strings.Contains(name, "Security") && strings.HasSuffix(name, ".java")
	})
	if len(candidates) == 0 {
		candidates = FindFiles(repo, func(name string) bool {
			return strings.Contains(name, "Config") && strings.HasSuffix(name, ".java")
		})
	}

	var rules []Rule
	for _, path := range candidates {
		text, ok := readFile(path)
		if !ok {
			continue
		}
		if !strings.Contains(text, "requestMatchers") && !strings.Contains(text, "anyRequest") {
			continue
		}

		for _, m := range secRe.FindAllStringSubmatch(text, -1) {
			args, kind, rolesRaw := m[1], m[2], m[3]
			http := ""
			if hm := httpMethodRe.FindStringSubmatch(args); hm != nil {
				http = strings.ToUpper(hm[1])
			}
			access := accessOf(kind, rolesRaw)
			for _, quoted := range firstQuotedAll(args) {
				rules = append(rules, Rule{HTTP: http, Path: quoted, Access: access})
			}
		}

		for _, m := range anyRe.FindAllStringSubmatch(text, -1) {
			rules = append(rules, Rule{HTTP: "", Path: "/**", Access: accessOf(m[1], m[2])})
		}
	}

	return rules
}

// HasSpringSecurity reports whether the project appears to use Spring Security.
func HasSpringSecurity(repo string) bool {
	for _, name := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		for _, path := range FindFiles(repo, func(base string) bool { return base == name }) {
			text, ok := readFile(path)
			if ok && strings.Contains(text, "spring-boot-starter-security") {
				return true
			}
		}
	}
	return len(FindFiles(repo, func(name string) bool {
		return strings.Contains(name, "Security") && strings.HasSuffix(name, ".java")
	})) > 0
}

func accessOf(kind, rolesRaw string) model.Access {
	switch kind {
	case "permitAll":
		return model.Public()
	case "authenticated":
		return model.Authenticated()
	case "denyAll":
		return model.Denied()
	}
	roles := firstQuotedAll(rolesRaw)
	for i := range roles {
		roles[i] = strings.TrimPrefix(roles[i], "ROLE_")
	}
	if len(roles) == 0 {
		return model.Authenticated()
	}
	return model.Roles(roles)
}

// URLAccess returns the first declared rule matching method and path.
func URLAccess(method, path string, rules []Rule, secured bool) model.Access {
	for _, rule := range rules {
		if rule.HTTP != "" && rule.HTTP != method {
			continue
		}
		if springRegexp(rule.Path).MatchString(path) {
			return rule.Access
		}
	}
	if secured {
		return model.Authenticated()
	}
	return model.Public()
}

// EffectiveAccess combines the URL rule with a @PreAuthorize expression.
//
// A method-level permitAll wins over the URL rule, and a role list is
// intersected with a role-restricted URL rule.
func EffectiveAccess(urlAccess model.Access, pre *string) model.Access {
	if pre == nil {
		return urlAccess
	}
	parsed := ParseRoles(*pre)
	if parsed == nil {
		return urlAccess
	}

	switch parsed.kind {
	case "authenticated":
		if urlAccess.IsRoles() {
			return urlAccess
		}
		return model.Authenticated()
	case "list":
		if !urlAccess.IsRoles() {
			return model.Roles(parsed.roles)
		}
		var intersection []string
		for _, role := range parsed.roles {
			for _, allowed := range urlAccess.Roles {
				if role == allowed {
					intersection = append(intersection, role)
					break
				}
			}
		}
		if len(intersection) > 0 {
			return model.Roles(intersection)
		}
		return urlAccess
	default:
		return model.Access{Kind: parsed.kind}
	}
}

func firstQuotedAll(text string) []string {
	matches := firstQuotedRe.FindAllStringSubmatch(text, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// springRegexp translates a Spring ant pattern into a regular expression.
func springRegexp(pattern string) *regexp.Regexp {
	patternCacheMu.Lock()
	if re, ok := patternCache[pattern]; ok {
		patternCacheMu.Unlock()
		return re
	}
	patternCacheMu.Unlock()

	var out strings.Builder
	for i := 0; i < len(pattern); {
		switch {
		case strings.HasPrefix(pattern[i:], "/**"):
			// Spring's /x/** also matches /x, so the tail is optional.
			out.WriteString("(?:/.*)?")
			i += 3
		case strings.HasPrefix(pattern[i:], "**"):
			out.WriteString(".*")
			i += 2
		case pattern[i] == '*':
			out.WriteString("[^/]*")
			i++
		case pattern[i] == '{':
			j := strings.IndexByte(pattern[i:], '}')
			if j > 0 {
				out.WriteString("[^/]+")
				i += j + 1
			} else {
				out.WriteString(regexp.QuoteMeta("{"))
				i++
			}
		default:
			out.WriteString(regexp.QuoteMeta(string(pattern[i])))
			i++
		}
	}

	re, err := regexp.Compile("^" + out.String() + "$")
	if err != nil {
		re = neverMatches
	}

	patternCacheMu.Lock()
	patternCache[pattern] = re
	patternCacheMu.Unlock()

	return re
}
