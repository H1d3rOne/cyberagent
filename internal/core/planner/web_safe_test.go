package planner

import (
	"testing"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

func TestBuildWebPlanSafeProfile(t *testing.T) {
	target, err := contracts.NewWebTarget("target_001", "http://localhost:8080/")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{Hosts: []string{"localhost"}, Ports: []int{8080}})
	job := contracts.NewWebScanJob("job_001", target.ID, scope.ID, ProfileSafe, "runs/job_001")

	result, err := New().BuildWebPlan(job, target, scope, Options{Profile: ProfileSafe})
	if err != nil {
		t.Fatalf("BuildWebPlan() error = %v", err)
	}
	if result.Plan.ID != "job_001_plan_001" {
		t.Fatalf("unexpected plan id: %s", result.Plan.ID)
	}
	if result.Plan.Planner != BuiltinWebSafePlanner {
		t.Fatalf("unexpected planner: %s", result.Plan.Planner)
	}
	if len(result.Steps) == 0 || len(result.Plan.Steps) != len(result.Steps) {
		t.Fatalf("unexpected step counts: plan=%d steps=%d", len(result.Plan.Steps), len(result.Steps))
	}

	byID := map[string]contracts.Step{}
	for _, step := range result.Steps {
		byID[step.ID] = step
		if step.JobID != job.ID || step.PlanID != result.Plan.ID {
			t.Fatalf("step has wrong ownership: %#v", step)
		}
		if step.Input["url"] != "http://localhost:8080" {
			t.Fatalf("step %s url input = %#v", step.ID, step.Input["url"])
		}
	}

	if _, ok := byID["step_http_probe_001"]; !ok {
		t.Fatalf("missing http probe step")
	}
	if _, ok := byID["step_optional_nuclei_safe_001"]; ok {
		t.Fatalf("nuclei-safe should not be enabled by default")
	}
	if _, ok := byID["step_optional_python_js_route_analyzer_001"]; ok {
		t.Fatalf("python-js-route-analyzer should not be enabled by default")
	}
	if _, ok := byID["step_optional_ffuf_low_rate_001"]; ok {
		t.Fatalf("ffuf-low-rate should not be enabled by default")
	}
	if got := byID["step_content_discovery_small_001"].Risk; got != contracts.RiskLow {
		t.Fatalf("content discovery risk = %s, want low", got)
	}
	if got := byID["step_report_generate_001"].DependsOn; len(got) != 1 || got[0] != "step_report_reflection_gate_001" {
		t.Fatalf("unexpected report dependencies: %#v", got)
	}
}

func TestBuildWebPlanWithNucleiSafeExplicitlyEnabled(t *testing.T) {
	target, err := contracts.NewWebTarget("target_001", "http://localhost:8080/")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{Hosts: []string{"localhost"}})
	job := contracts.NewWebScanJob("job_001", target.ID, scope.ID, ProfileSafe, "runs/job_001")

	result, err := New().BuildWebPlan(job, target, scope, Options{Profile: ProfileSafe, EnabledPlugins: []string{"nuclei-safe"}})
	if err != nil {
		t.Fatalf("BuildWebPlan() error = %v", err)
	}

	var found bool
	for _, step := range result.Steps {
		if step.ID == "step_optional_nuclei_safe_001" {
			found = true
			if step.Risk != contracts.RiskLow {
				t.Fatalf("nuclei-safe wrapper risk = %s, want low", step.Risk)
			}
			if step.Tool.Name != "plugin.nuclei-safe" {
				t.Fatalf("unexpected nuclei tool: %#v", step.Tool)
			}
		}
	}
	if !found {
		t.Fatalf("expected nuclei-safe step")
	}
}

func TestBuildWebPlanWithFFUFLowRateExplicitlyEnabled(t *testing.T) {
	target, err := contracts.NewWebTarget("target_001", "http://localhost:8080/")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{Hosts: []string{"localhost"}})
	job := contracts.NewWebScanJob("job_001", target.ID, scope.ID, ProfileSafe, "runs/job_001")

	result, err := New().BuildWebPlan(job, target, scope, Options{Profile: ProfileSafe, EnabledPlugins: []string{"ffuf-low-rate"}})
	if err != nil {
		t.Fatalf("BuildWebPlan() error = %v", err)
	}

	byID := map[string]contracts.Step{}
	for _, step := range result.Steps {
		byID[step.ID] = step
	}
	step, ok := byID["step_optional_ffuf_low_rate_001"]
	if !ok {
		t.Fatalf("expected ffuf-low-rate step")
	}
	if step.Risk != contracts.RiskMedium {
		t.Fatalf("ffuf-low-rate risk = %s, want medium", step.Risk)
	}
	if !step.RequiresApproval {
		t.Fatalf("ffuf-low-rate should require approval")
	}
	if step.Tool.Name != "plugin.ffuf-low-rate" {
		t.Fatalf("unexpected ffuf tool: %#v", step.Tool)
	}
	if len(step.DependsOn) != 1 || step.DependsOn[0] != "step_http_probe_001" {
		t.Fatalf("unexpected ffuf deps: %#v", step.DependsOn)
	}

	evidenceStep := byID["step_evidence_index_001"]
	var evidenceDependsOnPlugin bool
	for _, dep := range evidenceStep.DependsOn {
		if dep == step.ID {
			evidenceDependsOnPlugin = true
			break
		}
	}
	if !evidenceDependsOnPlugin {
		t.Fatalf("evidence index deps missing ffuf-low-rate: %#v", evidenceStep.DependsOn)
	}
}

func TestBuildWebPlanWithPythonJSRouteAnalyzerExplicitlyEnabled(t *testing.T) {
	target, err := contracts.NewWebTarget("target_001", "http://localhost:8080/")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{Hosts: []string{"localhost"}})
	job := contracts.NewWebScanJob("job_001", target.ID, scope.ID, ProfileSafe, "runs/job_001")

	result, err := New().BuildWebPlan(job, target, scope, Options{Profile: ProfileSafe, EnabledPlugins: []string{"python-js-route-analyzer"}})
	if err != nil {
		t.Fatalf("BuildWebPlan() error = %v", err)
	}

	byID := map[string]contracts.Step{}
	for _, step := range result.Steps {
		byID[step.ID] = step
	}
	step, ok := byID["step_optional_python_js_route_analyzer_001"]
	if !ok {
		t.Fatalf("expected python-js-route-analyzer step")
	}
	if step.Risk != contracts.RiskLow {
		t.Fatalf("python-js-route-analyzer risk = %s, want low", step.Risk)
	}
	if step.Tool.Name != "plugin.python-js-route-analyzer" {
		t.Fatalf("unexpected python analyzer tool: %#v", step.Tool)
	}
	if len(step.DependsOn) != 1 || step.DependsOn[0] != "step_js_asset_collect_001" {
		t.Fatalf("unexpected python analyzer deps: %#v", step.DependsOn)
	}

	evidenceStep := byID["step_evidence_index_001"]
	var evidenceDependsOnPlugin bool
	for _, dep := range evidenceStep.DependsOn {
		if dep == step.ID {
			evidenceDependsOnPlugin = true
			break
		}
	}
	if !evidenceDependsOnPlugin {
		t.Fatalf("evidence index deps missing python analyzer: %#v", evidenceStep.DependsOn)
	}
}

func TestBuildWebPlanRejectsMissingScope(t *testing.T) {
	target, err := contracts.NewWebTarget("target_001", "http://localhost:8080/")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	job := contracts.NewWebScanJob("job_001", target.ID, "", ProfileSafe, "runs/job_001")
	_, err = New().BuildWebPlan(job, target, contracts.Scope{}, Options{Profile: ProfileSafe})
	if err == nil {
		t.Fatalf("expected missing scope error")
	}
}

func TestBuildWebPlanRejectsUnsupportedProfile(t *testing.T) {
	target, err := contracts.NewWebTarget("target_001", "http://localhost:8080/")
	if err != nil {
		t.Fatalf("NewWebTarget() error = %v", err)
	}
	scope := contracts.NewScope("scope_001", contracts.ScopeRules{Hosts: []string{"localhost"}})
	job := contracts.NewWebScanJob("job_001", target.ID, scope.ID, "aggressive", "runs/job_001")
	_, err = New().BuildWebPlan(job, target, scope, Options{Profile: "aggressive"})
	if err == nil {
		t.Fatalf("expected unsupported profile error")
	}
}
