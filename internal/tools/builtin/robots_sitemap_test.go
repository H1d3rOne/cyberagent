package builtin

import "testing"

func TestParseRobots(t *testing.T) {
	body := []byte("User-agent: *\nDisallow: /admin\nAllow: /public\nSitemap: https://example.test/sitemap.xml\n")
	result := ParseRobots("job_001", "step_robots_fetch_001", "artifact_robots_txt_001", "https://example.test/robots.txt", body)
	if len(result.Paths) != 2 {
		t.Fatalf("paths = %#v, want 2", result.Paths)
	}
	if len(result.Sitemaps) != 1 || result.Sitemaps[0] != "https://example.test/sitemap.xml" {
		t.Fatalf("unexpected sitemaps: %#v", result.Sitemaps)
	}
	if len(result.Evidence) != 3 {
		t.Fatalf("evidence count = %d, want 3", len(result.Evidence))
	}
}

func TestParseSitemap(t *testing.T) {
	body := []byte(`<?xml version="1.0"?><urlset><url><loc>https://example.test/a</loc></url><url><loc>https://example.test/b</loc></url></urlset>`)
	result := ParseSitemap("job_001", "step_sitemap_fetch_001", "artifact_sitemap_xml_001", "https://example.test/sitemap.xml", body)
	if len(result.URLs) != 2 {
		t.Fatalf("urls = %#v, want 2", result.URLs)
	}
	if len(result.Evidence) != 2 {
		t.Fatalf("evidence count = %d, want 2", len(result.Evidence))
	}
}
