package builtin

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

var (
	tagRe     = regexp.MustCompile(`(?is)<\s*(a|script|form)\b([^>]*)>`)
	attrRe    = regexp.MustCompile(`(?is)([a-zA-Z_:][-a-zA-Z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s"'=<>]+))`)
	commentRe = regexp.MustCompile(`(?is)<!--(.*?)-->`)
)

// HTMLExtractInput contains HTML and ownership metadata for evidence generation.
type HTMLExtractInput struct {
	JobID      string
	StepID     string
	ArtifactID string
	BaseURL    string
	HTML       []byte
}

// ExtractedURL is a normalized URL extracted from HTML.
type ExtractedURL struct {
	Raw      string `json:"raw"`
	Resolved string `json:"resolved"`
}

// ExtractedForm describes a HTML form.
type ExtractedForm struct {
	Action   ExtractedURL      `json:"action"`
	Method   string            `json:"method"`
	RawAttrs map[string]string `json:"raw_attrs,omitempty"`
}

// HTMLExtractResult is the output of HTML extraction.
type HTMLExtractResult struct {
	Links    []ExtractedURL       `json:"links"`
	Scripts  []ExtractedURL       `json:"scripts"`
	Forms    []ExtractedForm      `json:"forms"`
	Comments []string             `json:"comments,omitempty"`
	Evidence []contracts.Evidence `json:"-"`
}

// ExtractHTML extracts links, scripts, forms, and comments from HTML.
func ExtractHTML(input HTMLExtractInput) (HTMLExtractResult, error) {
	base, err := url.Parse(input.BaseURL)
	if err != nil {
		return HTMLExtractResult{}, err
	}
	result := HTMLExtractResult{Links: []ExtractedURL{}, Scripts: []ExtractedURL{}, Forms: []ExtractedForm{}}
	seenEvidence := 1

	for _, match := range tagRe.FindAllSubmatch(input.HTML, -1) {
		tag := strings.ToLower(string(match[1]))
		attrs := parseAttrs(string(match[2]))
		switch tag {
		case "a":
			if href, ok := attrs["href"]; ok {
				if extracted, ok := resolveExtractedURL(base, href); ok {
					result.Links = append(result.Links, extracted)
					result.Evidence = append(result.Evidence, htmlEvidence(input, contracts.EvidenceHTMLLink, "html_link", seenEvidence, "HTML link discovered", extracted.Resolved, map[string]any{"raw": extracted.Raw, "resolved": extracted.Resolved}))
					seenEvidence++
				}
			}
		case "script":
			if src, ok := attrs["src"]; ok {
				if extracted, ok := resolveExtractedURL(base, src); ok {
					result.Scripts = append(result.Scripts, extracted)
					result.Evidence = append(result.Evidence, htmlEvidence(input, contracts.EvidenceHTMLLink, "html_script", seenEvidence, "JavaScript asset discovered", extracted.Resolved, map[string]any{"raw": extracted.Raw, "resolved": extracted.Resolved}))
					seenEvidence++
				}
			}
		case "form":
			action := attrs["action"]
			if action == "" {
				action = input.BaseURL
			}
			extracted, ok := resolveExtractedURL(base, action)
			if !ok {
				continue
			}
			method := strings.ToUpper(attrs["method"])
			if method == "" {
				method = "GET"
			}
			form := ExtractedForm{Action: extracted, Method: method, RawAttrs: attrs}
			result.Forms = append(result.Forms, form)
			result.Evidence = append(result.Evidence, htmlEvidence(input, contracts.EvidenceHTMLForm, "html_form", seenEvidence, "HTML form discovered", extracted.Resolved, map[string]any{"action": extracted.Resolved, "method": method}))
			seenEvidence++
		}
	}

	for _, match := range commentRe.FindAllSubmatch(input.HTML, -1) {
		comment := strings.TrimSpace(string(match[1]))
		if comment != "" {
			result.Comments = append(result.Comments, comment)
		}
	}
	return result, nil
}

func parseAttrs(raw string) map[string]string {
	attrs := map[string]string{}
	for _, match := range attrRe.FindAllStringSubmatch(raw, -1) {
		key := strings.ToLower(match[1])
		value := firstNonEmpty(match[2], match[3], match[4])
		attrs[key] = value
	}
	return attrs
}

func resolveExtractedURL(base *url.URL, raw string) (ExtractedURL, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ExtractedURL{}, false
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{"javascript:", "mailto:", "tel:", "data:"} {
		if strings.HasPrefix(lower, prefix) {
			return ExtractedURL{}, false
		}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ExtractedURL{}, false
	}
	resolved := base.ResolveReference(parsed)
	return ExtractedURL{Raw: raw, Resolved: resolved.String()}, true
}

func htmlEvidence(input HTMLExtractInput, evidenceType contracts.EvidenceType, kind string, n int, title, targetRef string, data map[string]any) contracts.Evidence {
	return contracts.Evidence{
		ID:         fmt.Sprintf("evidence_%s_%03d", kind, n),
		JobID:      input.JobID,
		StepID:     input.StepID,
		ArtifactID: input.ArtifactID,
		Type:       evidenceType,
		Title:      title,
		Summary:    title + ": " + targetRef,
		TargetRef:  targetRef,
		Data:       data,
		Confidence: contracts.ConfidenceHigh,
		CreatedAt:  contracts.Now(),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
