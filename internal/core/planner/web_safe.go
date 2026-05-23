package planner

import "github.com/H1d3rOne/cyberagent/internal/core/contracts"

type stepSpec struct {
	idSuffix         string
	name             string
	stepType         contracts.StepType
	risk             contracts.RiskLevel
	requiresApproval bool
	tool             contracts.ToolRef
	dependsOn        []string
	timeoutSeconds   int
	expectedOutputs  []string
}

func buildWebSafePlan(job contracts.Job, target contracts.Target, opts Options) Result {
	baseURL := target.Normalized.BaseURL
	if target.Normalized.Path != "" && target.Normalized.Path != "/" {
		baseURL += target.Normalized.Path
	}

	specs := []stepSpec{
		{idSuffix: "scope_check", name: "Scope Check", stepType: contracts.StepTypePolicyCheck, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.scope_guard", Version: "v1"}, timeoutSeconds: 5},
		{idSuffix: "target_normalize", name: "Target Normalize", stepType: contracts.StepTypeAnalyzer, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.target_normalizer", Version: "v1"}, dependsOn: []string{"scope_check"}, timeoutSeconds: 5},
		{idSuffix: "doctor_preflight", name: "Doctor Preflight", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.doctor", Version: "v1"}, dependsOn: []string{"target_normalize"}, timeoutSeconds: 10},
		{idSuffix: "http_probe", name: "HTTP Probe", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "builtin.http_probe", Version: "v1"}, dependsOn: []string{"doctor_preflight"}, timeoutSeconds: 30, expectedOutputs: []string{"artifact_http_probe", "evidence_http_status"}},
		{idSuffix: "http_fetch_homepage", name: "Fetch Homepage", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "builtin.http_fetcher", Version: "v1"}, dependsOn: []string{"http_probe"}, timeoutSeconds: 30, expectedOutputs: []string{"artifact_homepage_html", "artifact_homepage_headers"}},
		{idSuffix: "header_check", name: "Header Check", stepType: contracts.StepTypeAnalyzer, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.header_checker", Version: "v1"}, dependsOn: []string{"http_fetch_homepage"}, timeoutSeconds: 10, expectedOutputs: []string{"evidence_security_headers"}},
		{idSuffix: "cookie_check", name: "Cookie Check", stepType: contracts.StepTypeAnalyzer, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.cookie_checker", Version: "v1"}, dependsOn: []string{"http_fetch_homepage"}, timeoutSeconds: 10, expectedOutputs: []string{"evidence_cookie_attributes"}},
		{idSuffix: "robots_fetch", name: "Fetch robots.txt", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "builtin.robots_fetcher", Version: "v1"}, dependsOn: []string{"http_probe"}, timeoutSeconds: 20, expectedOutputs: []string{"artifact_robots_txt"}},
		{idSuffix: "sitemap_fetch", name: "Fetch sitemap.xml", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "builtin.sitemap_fetcher", Version: "v1"}, dependsOn: []string{"http_probe"}, timeoutSeconds: 20, expectedOutputs: []string{"artifact_sitemap_xml"}},
		{idSuffix: "html_extract", name: "HTML Extract", stepType: contracts.StepTypeParser, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.html_extractor", Version: "v1"}, dependsOn: []string{"http_fetch_homepage"}, timeoutSeconds: 10, expectedOutputs: []string{"evidence_html_links", "evidence_html_forms", "evidence_html_scripts"}},
		{idSuffix: "js_asset_collect", name: "Collect JavaScript Assets", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "builtin.js_asset_collector", Version: "v1"}, dependsOn: []string{"html_extract"}, timeoutSeconds: 60, expectedOutputs: []string{"artifact_js_assets"}},
		{idSuffix: "js_route_extract_light", name: "Light JS Route Extract", stepType: contracts.StepTypeParser, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.js_route_extractor_light", Version: "v1"}, dependsOn: []string{"js_asset_collect"}, timeoutSeconds: 30, expectedOutputs: []string{"evidence_js_routes"}},
		{idSuffix: "api_hint_detect", name: "API Hint Detect", stepType: contracts.StepTypeAnalyzer, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.api_hint_detector", Version: "v1"}, dependsOn: []string{"html_extract", "js_route_extract_light"}, timeoutSeconds: 15, expectedOutputs: []string{"evidence_api_hints"}},
		{idSuffix: "content_discovery_small", name: "Small Content Discovery", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "builtin.content_discovery_small", Version: "v1"}, dependsOn: []string{"http_probe"}, timeoutSeconds: 120, expectedOutputs: []string{"evidence_content_discovery"}},
		{idSuffix: "tls_inspect", name: "TLS Inspect", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.tls_inspector", Version: "v1"}, dependsOn: []string{"http_probe"}, timeoutSeconds: 15, expectedOutputs: []string{"evidence_tls_info"}},
		{idSuffix: "tcp_port_check", name: "TCP Port Check", stepType: contracts.StepTypeBuiltinTool, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "builtin.tcp_port_check", Version: "v1"}, dependsOn: []string{"doctor_preflight"}, timeoutSeconds: 30, expectedOutputs: []string{"evidence_open_ports"}},
	}

	if pluginEnabled(opts, "nuclei-safe") {
		specs = append(specs, stepSpec{idSuffix: "optional_nuclei_safe", name: "Optional nuclei-safe", stepType: contracts.StepTypeExternalTool, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "plugin.nuclei-safe", Version: "v1"}, dependsOn: []string{"http_probe"}, timeoutSeconds: 300, expectedOutputs: []string{"artifact_nuclei_jsonl", "evidence_nuclei_findings"}})
	}
	if pluginEnabled(opts, "ffuf-low-rate") {
		specs = append(specs, stepSpec{idSuffix: "optional_ffuf_low_rate", name: "Optional ffuf Low Rate", stepType: contracts.StepTypeExternalTool, risk: contracts.RiskMedium, requiresApproval: true, tool: contracts.ToolRef{Name: "plugin.ffuf-low-rate", Version: "v1"}, dependsOn: []string{"http_probe"}, timeoutSeconds: 300, expectedOutputs: []string{"artifact_ffuf_low_rate_json", "evidence_ffuf_low_rate_results"}})
	}
	if pluginEnabled(opts, "python-js-route-analyzer") {
		specs = append(specs, stepSpec{idSuffix: "optional_python_js_route_analyzer", name: "Optional Python JS Route Analyzer", stepType: contracts.StepTypeAnalyzer, risk: contracts.RiskLow, tool: contracts.ToolRef{Name: "plugin.python-js-route-analyzer", Version: "v1"}, dependsOn: []string{"js_asset_collect"}, timeoutSeconds: 60, expectedOutputs: []string{"artifact_python_js_route_analyzer_jsonl", "evidence_python_js_routes"}})
	}

	specs = append(specs,
		stepSpec{idSuffix: "evidence_index", name: "Evidence Index", stepType: contracts.StepTypeAnalyzer, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.evidence_indexer", Version: "v1"}, dependsOn: terminalEvidenceDeps(opts), timeoutSeconds: 30},
		stepSpec{idSuffix: "finding_normalize", name: "Finding Normalize", stepType: contracts.StepTypeAnalyzer, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.finding_normalizer", Version: "v1"}, dependsOn: []string{"evidence_index"}, timeoutSeconds: 30},
		stepSpec{idSuffix: "report_reflection_gate", name: "Report Reflection Gate", stepType: contracts.StepTypeReflection, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.report_reflection_gate", Version: "v1"}, dependsOn: []string{"finding_normalize"}, timeoutSeconds: 15},
		stepSpec{idSuffix: "report_generate", name: "Generate Report", stepType: contracts.StepTypeReporter, risk: contracts.RiskInfo, tool: contracts.ToolRef{Name: "builtin.report_generator", Version: "v1"}, dependsOn: []string{"report_reflection_gate"}, timeoutSeconds: 30, expectedOutputs: []string{"report_markdown", "report_json"}},
	)

	steps := make([]contracts.Step, 0, len(specs))
	stepIDs := make([]string, 0, len(specs))
	for _, spec := range specs {
		id := stepID(spec.idSuffix)
		stepIDs = append(stepIDs, id)
		step := contracts.NewStep(id, job.ID, planID(job.ID), spec.name, spec.stepType, spec.risk, spec.tool)
		step.RequiresApproval = spec.requiresApproval
		step.DependsOn = resolveDeps(spec.dependsOn)
		step.TimeoutSeconds = spec.timeoutSeconds
		step.ExpectedOutputs = spec.expectedOutputs
		step.Input = map[string]any{"url": baseURL}
		steps = append(steps, step)
	}

	plan := contracts.NewPlan(planID(job.ID), job.ID, BuiltinWebSafePlanner, stepIDs)
	plan.Rationale = "safe web profile baseline plan"
	return Result{Plan: plan, Steps: steps}
}

func terminalEvidenceDeps(opts Options) []string {
	deps := []string{"header_check", "cookie_check", "robots_fetch", "sitemap_fetch", "html_extract", "api_hint_detect", "content_discovery_small", "tls_inspect", "tcp_port_check"}
	if pluginEnabled(opts, "nuclei-safe") {
		deps = append(deps, "optional_nuclei_safe")
	}
	if pluginEnabled(opts, "ffuf-low-rate") {
		deps = append(deps, "optional_ffuf_low_rate")
	}
	if pluginEnabled(opts, "python-js-route-analyzer") {
		deps = append(deps, "optional_python_js_route_analyzer")
	}
	return deps
}

func resolveDeps(suffixes []string) []string {
	if len(suffixes) == 0 {
		return nil
	}
	out := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		out = append(out, stepID(suffix))
	}
	return out
}

func planID(jobID string) string {
	return jobID + "_plan_001"
}

func stepID(suffix string) string {
	return "step_" + suffix + "_001"
}
