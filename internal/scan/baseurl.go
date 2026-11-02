package scan

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	portRe        = regexp.MustCompile(`server\.port\s*[:=]\s*(\d+)`)
	contextPathRe = regexp.MustCompile(`server\.servlet\.context-path\s*[:=]\s*(\S+)`)
	serverBlockRe = regexp.MustCompile(`(?m)^server:\s*\n((?:[ \t]+.*\n?)+)`)
	yamlPortRe    = regexp.MustCompile(`(?m)^[ \t]+port:\s*(\d+)`)
	yamlContextRe = regexp.MustCompile(`(?m)^[ \t]+context-path:\s*(\S+)`)
)

// DetectBaseURL derives the local base URL from application config, defaulting
// to port 8080 without a context path.
func DetectBaseURL(repo string) string {
	port := 8080
	contextPath := ""

	files := FindFiles(repo, func(name string) bool {
		if !strings.HasPrefix(name, "application") {
			return false
		}
		return strings.HasSuffix(name, ".properties") ||
			strings.HasSuffix(name, ".yml") ||
			strings.HasSuffix(name, ".yaml")
	})

	for _, path := range files {
		text, ok := readFile(path)
		if !ok {
			continue
		}

		if m := portRe.FindStringSubmatch(text); m != nil {
			fmt.Sscanf(m[1], "%d", &port)
		}
		if m := contextPathRe.FindStringSubmatch(text); m != nil {
			contextPath = strings.Trim(m[1], `"'`)
		}

		if block := serverBlockRe.FindStringSubmatch(text); block != nil {
			if m := yamlPortRe.FindStringSubmatch(block[1]); m != nil {
				fmt.Sscanf(m[1], "%d", &port)
			}
			if m := yamlContextRe.FindStringSubmatch(block[1]); m != nil {
				contextPath = strings.Trim(m[1], `"'`)
			}
		}
	}

	contextPath = strings.Trim(contextPath, "/")
	if contextPath != "" {
		contextPath = "/" + contextPath
	}
	return fmt.Sprintf("http://localhost:%d%s", port, contextPath)
}
