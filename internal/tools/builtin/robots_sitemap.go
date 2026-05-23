package builtin

import (
	"bytes"
	"encoding/xml"
	"regexp"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// RobotsResult contains paths and sitemap URLs discovered from robots.txt.
type RobotsResult struct {
	Paths    []string
	Sitemaps []string
	Evidence []contracts.Evidence
}

// ParseRobots extracts Allow/Disallow/Sitemap entries from robots.txt content.
func ParseRobots(jobID, stepID, artifactID, targetRef string, body []byte) RobotsResult {
	result := RobotsResult{Paths: []string{}, Sitemaps: []string{}}
	lines := strings.Split(string(body), "\n")
	idx := 1
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])
		if value == "" {
			continue
		}
		switch key {
		case "allow", "disallow":
			if strings.HasPrefix(value, "/") {
				result.Paths = append(result.Paths, value)
				result.Evidence = append(result.Evidence, contracts.Evidence{ID: fmtEvidenceID("robots_path", idx), JobID: jobID, StepID: stepID, ArtifactID: artifactID, Type: contracts.EvidenceAPIHint, Title: "robots.txt path discovered", Summary: key + ": " + value, TargetRef: targetRef, Data: map[string]any{"directive": key, "path": value}, Confidence: contracts.ConfidenceMedium, CreatedAt: contracts.Now()})
				idx++
			}
		case "sitemap":
			result.Sitemaps = append(result.Sitemaps, value)
			result.Evidence = append(result.Evidence, contracts.Evidence{ID: fmtEvidenceID("robots_sitemap", idx), JobID: jobID, StepID: stepID, ArtifactID: artifactID, Type: contracts.EvidenceAPIHint, Title: "robots.txt sitemap discovered", Summary: "Sitemap discovered in robots.txt: " + value, TargetRef: value, Data: map[string]any{"sitemap": value}, Confidence: contracts.ConfidenceMedium, CreatedAt: contracts.Now()})
			idx++
		}
	}
	return result
}

// SitemapResult contains URLs extracted from a sitemap.
type SitemapResult struct {
	URLs     []string
	Evidence []contracts.Evidence
}

type urlSet struct {
	URLs []struct {
		Loc string `xml:"loc"`
	} `xml:"url"`
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

var locRe = regexp.MustCompile(`(?is)<loc>\s*([^<]+?)\s*</loc>`)

// ParseSitemap extracts loc entries from sitemap XML. It tolerates malformed/simple XML via regex fallback.
func ParseSitemap(jobID, stepID, artifactID, targetRef string, body []byte) SitemapResult {
	urls := []string{}
	var parsed urlSet
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(&parsed); err == nil {
		for _, item := range parsed.URLs {
			if strings.TrimSpace(item.Loc) != "" {
				urls = append(urls, strings.TrimSpace(item.Loc))
			}
		}
		for _, item := range parsed.Sitemaps {
			if strings.TrimSpace(item.Loc) != "" {
				urls = append(urls, strings.TrimSpace(item.Loc))
			}
		}
	}
	if len(urls) == 0 {
		for _, match := range locRe.FindAllSubmatch(body, -1) {
			if len(match) > 1 && strings.TrimSpace(string(match[1])) != "" {
				urls = append(urls, strings.TrimSpace(string(match[1])))
			}
		}
	}

	result := SitemapResult{URLs: urls}
	for i, loc := range urls {
		result.Evidence = append(result.Evidence, contracts.Evidence{ID: fmtEvidenceID("sitemap_url", i+1), JobID: jobID, StepID: stepID, ArtifactID: artifactID, Type: contracts.EvidenceAPIHint, Title: "sitemap URL discovered", Summary: "Sitemap URL discovered: " + loc, TargetRef: loc, Data: map[string]any{"url": loc, "sitemap": targetRef}, Confidence: contracts.ConfidenceMedium, CreatedAt: contracts.Now()})
	}
	return result
}

func fmtEvidenceID(kind string, n int) string {
	return "evidence_" + kind + "_" + threeDigits(n)
}

func threeDigits(n int) string {
	if n < 10 {
		return "00" + itoaDigit(n)
	}
	if n < 100 {
		return "0" + itoaDigit(n/10) + itoaDigit(n%10)
	}
	return itoaDigit(n/100) + itoaDigit((n/10)%10) + itoaDigit(n%10)
}

func itoaDigit(n int) string { return string(rune('0' + n)) }
