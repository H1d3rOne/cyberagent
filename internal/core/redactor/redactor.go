package redactor

import (
	"regexp"
	"strings"
)

type Redactor struct {
	patterns []pattern
}

type pattern struct {
	re          *regexp.Regexp
	replacement string
}

func New() *Redactor {
	return &Redactor{
		patterns: defaultPatterns(),
	}
}

func (r *Redactor) Redact(input string) string {
	result := input
	for _, p := range r.patterns {
		result = p.re.ReplaceAllString(result, p.replacement)
	}
	return result
}

func (r *Redactor) RedactHeaders(headers map[string][]string) map[string][]string {
	redacted := make(map[string][]string, len(headers))
	for key, values := range headers {
		if isSensitiveHeader(key) {
			masked := make([]string, len(values))
			for i, v := range values {
				masked[i] = maskValue(v)
			}
			redacted[key] = masked
		} else {
			redacted[key] = values
		}
	}
	return redacted
}

func (r *Redactor) RedactBytes(data []byte) []byte {
	return []byte(r.Redact(string(data)))
}

func defaultPatterns() []pattern {
	return []pattern{
		{re: regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9\-._~+/]+=*`), replacement: "${1}[REDACTED]"},
		{re: regexp.MustCompile(`(?i)(token[=:]\s*")[^"]+"`), replacement: "${1}[REDACTED]\""},
		{re: regexp.MustCompile(`(?i)(token[=:]\s*)[^\s,;&"']+`), replacement: "${1}[REDACTED]"},
		{re: regexp.MustCompile(`(?i)(api[_-]?key[=:]\s*")[^"]+"`), replacement: "${1}[REDACTED]\""},
		{re: regexp.MustCompile(`(?i)(api[_-]?key[=:]\s*)[^\s,;&"']+`), replacement: "${1}[REDACTED]"},
		{re: regexp.MustCompile(`(?i)(password[=:]\s*")[^"]+"`), replacement: "${1}[REDACTED]\""},
		{re: regexp.MustCompile(`(?i)(password[=:]\s*)[^\s,;&"']+`), replacement: "${1}[REDACTED]"},
		{re: regexp.MustCompile(`(?i)(secret[=:]\s*")[^"]+"`), replacement: "${1}[REDACTED]\""},
		{re: regexp.MustCompile(`(?i)(secret[=:]\s*)[^\s,;&"']+`), replacement: "${1}[REDACTED]"},
		{re: regexp.MustCompile(`(?i)(session[_-]?id[=:]\s*)[^\s,;&"']+`), replacement: "${1}[REDACTED]"},
		{re: regexp.MustCompile(`(?i)(csrf[_-]?token[=:]\s*)[^\s,;&"']+`), replacement: "${1}[REDACTED]"},
	}
}

var sensitiveHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"set-cookie":          true,
	"x-api-key":           true,
	"x-auth-token":        true,
	"x-csrf-token":        true,
	"proxy-authorization": true,
}

func isSensitiveHeader(key string) bool {
	return sensitiveHeaders[strings.ToLower(key)]
}

func maskValue(v string) string {
	if len(v) <= 8 {
		return "[REDACTED]"
	}
	return v[:4] + "...[REDACTED]"
}
