package builtin

import "testing"

func TestExtractJSRoutesLightAndAPIHints(t *testing.T) {
	result, err := ExtractJSRoutesLight(JSRouteInput{JobID: "job_001", StepID: "step_js_route_001", ArtifactID: "artifact_js_001", BaseURL: "https://example.test/app/", Sources: map[string][]byte{"app.js": []byte(`fetch('/api/v1/users'); axios.get("/graphql"); const x = "/v2/status";`)}})
	if err != nil {
		t.Fatalf("ExtractJSRoutesLight() error = %v", err)
	}
	if len(result.Routes) != 3 {
		t.Fatalf("routes = %#v, want 3", result.Routes)
	}
	if len(result.Evidence) != 3 {
		t.Fatalf("evidence count = %d, want 3", len(result.Evidence))
	}
	hints := DetectAPIHints("job_001", "step_api_hint_001", result.Routes)
	if len(hints) != 3 {
		t.Fatalf("hints count = %d, want 3", len(hints))
	}
}
