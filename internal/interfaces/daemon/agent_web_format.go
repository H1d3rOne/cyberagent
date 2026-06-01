package daemon

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/agent/search"
)

func agentFormatWebSearch(language string, result search.Result) string {
	if len(result.Items) == 0 {
		return agentText(language,
			"web.search 没有结果。",
			"web.search returned no results.",
		)
	}
	var b strings.Builder
	header := agentText(language,
		fmt.Sprintf("web.search via %s — %d 条结果：", result.Provider, len(result.Items)),
		fmt.Sprintf("web.search via %s — %d results:", result.Provider, len(result.Items)),
	)
	b.WriteString(header)
	b.WriteString("\n")
	for i, item := range result.Items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = item.URL
		}
		snippet := strings.TrimSpace(item.Snippet)
		if len(snippet) > 320 {
			snippet = snippet[:320] + "…"
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, title, item.URL)
		if snippet != "" {
			fmt.Fprintf(&b, "   %s\n", snippet)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func agentFormatWebFetch(language, target, title, content, contentType string, truncated bool) string {
	var b strings.Builder
	header := agentText(language,
		fmt.Sprintf("web.fetch %s (%s)", target, strings.TrimSpace(contentType)),
		fmt.Sprintf("web.fetch %s (%s)", target, strings.TrimSpace(contentType)),
	)
	b.WriteString(header)
	b.WriteString("\n")
	if title != "" {
		fmt.Fprintf(&b, "title: %s\n", title)
	}
	b.WriteString(content)
	if truncated {
		b.WriteString(agentText(language, "\n... (已截断)", "\n... (truncated)"))
	}
	return strings.TrimRight(b.String(), "\n")
}

var (
	htmlScriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	htmlStyleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	htmlCommentRe  = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTitleRe    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	htmlTagRe      = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlBlockRe    = regexp.MustCompile(`(?i)</?(p|div|section|article|li|tr|h[1-6]|br|hr|blockquote|pre)[^>]*>`)
	htmlEntitySpRe = regexp.MustCompile(`&nbsp;`)
	whitespaceRe   = regexp.MustCompile(`[\t \x00-\x08\x0b\x0c]+`)
	multiNewlineRe = regexp.MustCompile(`\n{3,}`)
)

// htmlToReadable strips scripts/styles/comments and collapses tags into plain
// text, preserving paragraph boundaries. Returns (title, body).
func htmlToReadable(html string) (string, string) {
	cleaned := htmlScriptRe.ReplaceAllString(html, "")
	cleaned = htmlStyleRe.ReplaceAllString(cleaned, "")
	cleaned = htmlCommentRe.ReplaceAllString(cleaned, "")
	title := ""
	if m := htmlTitleRe.FindStringSubmatch(cleaned); len(m) == 2 {
		title = strings.TrimSpace(htmlDecodeBasic(htmlTagRe.ReplaceAllString(m[1], "")))
	}
	cleaned = htmlBlockRe.ReplaceAllString(cleaned, "\n")
	cleaned = htmlTagRe.ReplaceAllString(cleaned, "")
	cleaned = htmlEntitySpRe.ReplaceAllString(cleaned, " ")
	cleaned = htmlDecodeBasic(cleaned)
	cleaned = whitespaceRe.ReplaceAllString(cleaned, " ")
	lines := strings.Split(cleaned, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimSpace(line))
	}
	body := strings.Join(out, "\n")
	body = multiNewlineRe.ReplaceAllString(body, "\n\n")
	return title, strings.TrimSpace(body)
}

func htmlDecodeBasic(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
		"&apos;", "'",
	)
	return r.Replace(s)
}
