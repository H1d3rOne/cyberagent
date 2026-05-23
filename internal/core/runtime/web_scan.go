package runtime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
	"github.com/H1d3rOne/cyberagent/internal/core/evidence"
	"github.com/H1d3rOne/cyberagent/internal/core/findings"
	"github.com/H1d3rOne/cyberagent/internal/core/planner"
	"github.com/H1d3rOne/cyberagent/internal/core/policy"
	"github.com/H1d3rOne/cyberagent/internal/core/ratelimit"
	"github.com/H1d3rOne/cyberagent/internal/core/reporting"
	"github.com/H1d3rOne/cyberagent/internal/core/scheduler"
	pluginruntime "github.com/H1d3rOne/cyberagent/internal/plugins"
	"github.com/H1d3rOne/cyberagent/internal/runtime/artifacts"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
	"github.com/H1d3rOne/cyberagent/internal/runtime/state"
	"github.com/H1d3rOne/cyberagent/internal/tools/builtin"
)

// WebScanRequest contains the CLI/API request for an MVP web safe scan.
type WebScanRequest struct {
	Target          string
	Scope           string
	Profile         string
	Out             string
	EnabledPlugins  []string
	NoExternalTools bool
	Interactive     bool
	OnProgress      func(stepName, status string)
}

// WebScanResult describes one completed scan.
type WebScanResult struct {
	Job       contracts.Job
	Target    contracts.Target
	Scope     contracts.Scope
	Plan      contracts.Plan
	Workspace string
	ReportMD  string
	Findings  []contracts.Finding
	Evidence  []contracts.Evidence
	Artifacts []contracts.Artifact
}

// RunWebScan runs the MVP built-in safe web analysis flow.
func RunWebScan(ctx context.Context, req WebScanRequest) (WebScanResult, error) {
	if strings.TrimSpace(req.Target) == "" {
		return WebScanResult{}, fmt.Errorf("target is required")
	}
	if strings.TrimSpace(req.Scope) == "" {
		return WebScanResult{}, fmt.Errorf("scope is required for active scan")
	}
	profile := req.Profile
	if profile == "" {
		profile = planner.ProfileSafe
	}

	target, err := contracts.NewWebTarget("target_001", req.Target)
	if err != nil {
		return WebScanResult{}, err
	}
	scope, err := BuildScopeFromString("scope_001", req.Scope)
	if err != nil {
		return WebScanResult{}, err
	}

	root, jobID := outputRootAndJobID(req.Out)
	manager := runs.NewManager(root)
	layout, err := manager.Create(jobID)
	if err != nil {
		return WebScanResult{}, err
	}
	jobID = layout.JobID
	job := contracts.NewWebScanJob(jobID, target.ID, scope.ID, profile, layout.Path)
	job.Options.Interactive = req.Interactive
	job.Options.AllowExternalTools = !req.NoExternalTools

	artifactStore := artifacts.NewStore(layout.Path)
	if err := artifactStore.WriteJSON("target.json", target); err != nil {
		return WebScanResult{}, err
	}
	if err := artifactStore.WriteJSON("scope.json", scope); err != nil {
		return WebScanResult{}, err
	}

	stateStore, err := state.Open(layout.BoltPath)
	if err != nil {
		return WebScanResult{}, err
	}
	defer stateStore.Close()
	eventStore := events.NewStore(layout.EventsPath)
	bus := events.NewBus(eventStore)
	commandRecorder := command.NewRecorder(layout.CommandsPath)
	emit := func(stepID string, typ contracts.EventType, level contracts.EventLevel, message string) {
		_ = bus.Publish(contracts.NewEvent(nextEventID(), job.ID, stepID, typ, level, message))
	}

	emit("", contracts.EventJobCreated, contracts.EventLevelInfo, "job created")
	job.Status = contracts.StatusPlanning
	if err := stateStore.SaveJob(job); err != nil {
		return WebScanResult{}, err
	}

	pln, err := planner.New().BuildWebPlan(job, target, scope, planner.Options{Profile: profile, EnabledPlugins: enabledPlugins(req)})
	if err != nil {
		return WebScanResult{}, err
	}
	job.PlanID = pln.Plan.ID
	job.Summary.StepsTotal = len(pln.Steps)
	if err := artifactStore.WriteJSON("plan.json", pln.Plan); err != nil {
		return WebScanResult{}, err
	}
	if err := stateStore.SavePlan(pln.Plan); err != nil {
		return WebScanResult{}, err
	}
	for _, step := range pln.Steps {
		if err := stateStore.SaveStep(step); err != nil {
			return WebScanResult{}, err
		}
	}
	emit("", contracts.EventPlanCreated, contracts.EventLevelInfo, "plan created")

	guard := policy.NewEngine()
	stepMachine := scheduler.NewStepStateMachine()
	limiter := ratelimit.New(ratelimit.Config{
		GlobalRPS:         20,
		GlobalBurst:       5,
		PerHostRPS:        10,
		PerHostBurst:      3,
		MaxConcurrencyAll: 10,
	})
	artifactsOut := []contracts.Artifact{}
	evidenceOut := []contracts.Evidence{}
	findingsOut := []contracts.Finding{}
	skipped := []string{}

	start := contracts.Now()
	job.StartedAt = &start
	job.Status = contracts.StatusRunning
	emit("", contracts.EventJobStarted, contracts.EventLevelInfo, "job started")

	var fetch builtin.HTTPFetchResult
	var htmlResult builtin.HTMLExtractResult
	jsSources := map[string][]byte{}
	jsRoutes := []builtin.ExtractedURL{}
	progress := req.OnProgress
	if progress == nil {
		progress = func(string, string) {}
	}
	for _, step := range pln.Steps {
		step, err = stepMachine.Start(step)
		if err != nil {
			return WebScanResult{}, err
		}
		_ = stateStore.SaveStep(step)
		emit(step.ID, contracts.EventStepStarted, contracts.EventLevelInfo, step.Name+" started")
		progress(step.Name, "running")
		toolCall := command.NewStepCall(nextToolCallID(), job.ID, step)
		toolCallRecorded := false
		emit(step.ID, contracts.EventToolStarted, contracts.EventLevelInfo, step.Tool.Name+" started")

		riskDecision := guard.CheckStep("policy_"+step.ID, step, profile)
		emit(step.ID, contracts.EventPolicyChecked, contracts.EventLevelInfo, string(riskDecision.Decision))
		if riskDecision.Decision == contracts.PolicyDeny || riskDecision.Decision == contracts.PolicyRequireApproval {
			reason := strings.Join(riskDecision.Reasons, ",")
			step, err = stepMachine.Skip(step, reason)
			if err != nil {
				return WebScanResult{}, err
			}
			skipped = append(skipped, step.ID)
			_ = stateStore.SaveStep(step)
			if riskDecision.Decision == contracts.PolicyRequireApproval {
				toolCall.Metadata["policy_decision"] = string(riskDecision.Decision)
				toolCall.Metadata["policy_reasons"] = append([]string(nil), riskDecision.Reasons...)
			}
			if err := commandRecorder.Append(command.Finish(toolCall, contracts.StatusSkipped, fmt.Errorf("%s", step.Error))); err != nil {
				return WebScanResult{}, err
			}
			toolCallRecorded = true
			emit(step.ID, contracts.EventToolFinished, contracts.EventLevelWarn, step.Tool.Name+" skipped")
			emit(step.ID, contracts.EventStepSkipped, contracts.EventLevelWarn, step.Error)
			continue
		}

		var execErr error
		networkStep := isNetworkStep(step.Tool.Name)
		networkAcquired := false
		if networkStep {
			if waitErr := limiter.Wait(ctx, target.Normalized.Host); waitErr != nil {
				execErr = fmt.Errorf("rate limit wait: %w", waitErr)
			} else {
				networkAcquired = true
			}
		}
		switch step.Tool.Name {
		case "builtin.scope_guard":
			decision := guard.CheckTarget(target, scope)
			emit(step.ID, contracts.EventPolicyChecked, contracts.EventLevelInfo, strings.Join(decision.Reasons, ","))
			if decision.Decision == contracts.PolicyDeny {
				execErr = fmt.Errorf("target out of scope: %s", strings.Join(decision.Reasons, ","))
			}
		case "builtin.target_normalizer", "builtin.doctor":
			// Already handled during setup; retained as explicit plan steps.
		case "builtin.http_probe":
			if decision := guard.CheckURL("policy_url_"+step.ID, job.ID, step.ID, target.Normalized.BaseURL, scope); decision.Decision == contracts.PolicyDeny {
				execErr = fmt.Errorf("probe URL out of scope: %s", strings.Join(decision.Reasons, ","))
				break
			}
			probe, err := builtin.HTTPProbe(ctx, builtin.HTTPProbeOptions{URL: target.Normalized.BaseURL, Timeout: timeout(step), MaxRedirects: 5})
			if err != nil {
				execErr = err
				break
			}
			artifact, err := artifactStore.SaveJSON("artifact_http_probe_001", job.ID, step.ID, "artifacts/http/probe.json", probe)
			if err != nil {
				execErr = err
				break
			}
			artifactsOut = append(artifactsOut, artifact)
			evidenceOut = append(evidenceOut, contracts.Evidence{ID: "evidence_http_status_001", JobID: job.ID, StepID: step.ID, ArtifactID: artifact.ID, Type: contracts.EvidenceHTTPResponse, Title: "HTTP response status", Summary: probe.Status, TargetRef: probe.FinalURL, Data: map[string]any{"status_code": probe.StatusCode, "method": probe.Method}, Confidence: contracts.ConfidenceHigh, CreatedAt: contracts.Now()})
		case "builtin.http_fetcher":
			if decision := guard.CheckURL("policy_url_"+step.ID, job.ID, step.ID, target.Normalized.BaseURL, scope); decision.Decision == contracts.PolicyDeny {
				execErr = fmt.Errorf("fetch URL out of scope: %s", strings.Join(decision.Reasons, ","))
				break
			}
			fetch, err = builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: target.Normalized.BaseURL, Timeout: timeout(step), MaxBytes: 2 * 1024 * 1024, MaxRedirects: 5})
			if err != nil {
				execErr = err
				break
			}
			bodyArtifact, err := artifactStore.SaveBytes("artifact_homepage_html_001", job.ID, step.ID, contracts.ArtifactHTML, "artifacts/http/homepage.html", fetch.Body, fetch.ContentType)
			if err != nil {
				execErr = err
				break
			}
			headersArtifact, err := artifactStore.SaveJSON("artifact_homepage_headers_001", job.ID, step.ID, "artifacts/http/homepage_headers.json", fetch.Headers)
			if err != nil {
				execErr = err
				break
			}
			artifactsOut = append(artifactsOut, bodyArtifact, headersArtifact)
		case "builtin.header_checker":
			result := builtin.CheckSecurityHeaders(builtin.HeaderCheckInput{JobID: job.ID, StepID: step.ID, ArtifactID: "artifact_homepage_headers_001", TargetRef: target.Normalized.BaseURL, Headers: http.Header(fetch.Headers), HTTPS: target.Normalized.Scheme == "https"})
			evidenceOut = append(evidenceOut, result.Evidence...)
			findingsOut = append(findingsOut, result.Findings...)
		case "builtin.cookie_checker":
			result := builtin.CheckCookies(builtin.CookieCheckInput{JobID: job.ID, StepID: step.ID, ArtifactID: "artifact_homepage_headers_001", TargetRef: target.Normalized.BaseURL, Headers: http.Header(fetch.Headers), HTTPS: target.Normalized.Scheme == "https"})
			evidenceOut = append(evidenceOut, result.Evidence...)
			findingsOut = append(findingsOut, result.Findings...)
		case "builtin.robots_fetcher":
			robotsURL := joinURLPath(target.Normalized.BaseURL, "/robots.txt")
			if decision := guard.CheckURL("policy_url_"+step.ID, job.ID, step.ID, robotsURL, scope); decision.Decision == contracts.PolicyDeny {
				execErr = fmt.Errorf("robots URL out of scope: %s", strings.Join(decision.Reasons, ","))
				break
			}
			robotsFetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: robotsURL, Timeout: timeout(step), MaxBytes: 256 * 1024, MaxRedirects: 2})
			if err == nil && robotsFetch.StatusCode == http.StatusOK {
				artifact, err := artifactStore.SaveBytes("artifact_robots_txt_001", job.ID, step.ID, contracts.ArtifactText, "artifacts/http/robots.txt", robotsFetch.Body, robotsFetch.ContentType)
				if err != nil {
					execErr = err
					break
				}
				artifactsOut = append(artifactsOut, artifact)
				parsed := builtin.ParseRobots(job.ID, step.ID, artifact.ID, robotsURL, robotsFetch.Body)
				evidenceOut = append(evidenceOut, parsed.Evidence...)
			}
		case "builtin.sitemap_fetcher":
			sitemapURL := joinURLPath(target.Normalized.BaseURL, "/sitemap.xml")
			if decision := guard.CheckURL("policy_url_"+step.ID, job.ID, step.ID, sitemapURL, scope); decision.Decision == contracts.PolicyDeny {
				execErr = fmt.Errorf("sitemap URL out of scope: %s", strings.Join(decision.Reasons, ","))
				break
			}
			sitemapFetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: sitemapURL, Timeout: timeout(step), MaxBytes: 512 * 1024, MaxRedirects: 2})
			if err == nil && sitemapFetch.StatusCode == http.StatusOK {
				artifact, err := artifactStore.SaveBytes("artifact_sitemap_xml_001", job.ID, step.ID, contracts.ArtifactXML, "artifacts/http/sitemap.xml", sitemapFetch.Body, sitemapFetch.ContentType)
				if err != nil {
					execErr = err
					break
				}
				artifactsOut = append(artifactsOut, artifact)
				parsed := builtin.ParseSitemap(job.ID, step.ID, artifact.ID, sitemapURL, sitemapFetch.Body)
				evidenceOut = append(evidenceOut, parsed.Evidence...)
			}
		case "builtin.html_extractor":
			result, err := builtin.ExtractHTML(builtin.HTMLExtractInput{JobID: job.ID, StepID: step.ID, ArtifactID: "artifact_homepage_html_001", BaseURL: target.Normalized.BaseURL, HTML: fetch.Body})
			if err != nil {
				execErr = err
				break
			}
			htmlResult = result
			evidenceOut = append(evidenceOut, result.Evidence...)
		case "builtin.js_asset_collector":
			for i, script := range htmlResult.Scripts {
				if decision := guard.CheckURL("policy_url_"+step.ID, job.ID, step.ID, script.Resolved, scope); decision.Decision == contracts.PolicyDeny {
					continue
				}
				jsFetch, err := builtin.HTTPFetch(ctx, builtin.HTTPFetchOptions{URL: script.Resolved, Timeout: timeout(step), MaxBytes: 1024 * 1024, MaxRedirects: 2})
				if err != nil || jsFetch.StatusCode >= 400 {
					continue
				}
				artifactID := fmt.Sprintf("artifact_js_asset_%03d", i+1)
				artifact, err := artifactStore.SaveBytes(artifactID, job.ID, step.ID, contracts.ArtifactText, fmt.Sprintf("artifacts/http/js_%03d.js", i+1), jsFetch.Body, jsFetch.ContentType)
				if err != nil {
					execErr = err
					break
				}
				artifactsOut = append(artifactsOut, artifact)
				jsSources[artifact.ID] = jsFetch.Body
			}
		case "builtin.js_route_extractor_light":
			result, err := builtin.ExtractJSRoutesLight(builtin.JSRouteInput{JobID: job.ID, StepID: step.ID, ArtifactID: "artifact_js_assets", BaseURL: target.Normalized.BaseURL, Sources: jsSources})
			if err != nil {
				execErr = err
				break
			}
			jsRoutes = result.Routes
			evidenceOut = append(evidenceOut, result.Evidence...)
		case "builtin.api_hint_detector":
			evidenceOut = append(evidenceOut, builtin.DetectAPIHints(job.ID, step.ID, jsRoutes)...)
		case "builtin.content_discovery_small":
			result := builtin.DiscoverContentSmall(ctx, builtin.ContentDiscoveryInput{JobID: job.ID, StepID: step.ID, BaseURL: target.Normalized.BaseURL, Timeout: timeout(step), Allowed: func(raw string) bool {
				return guard.CheckURL("policy_url_"+step.ID, job.ID, step.ID, raw, scope).Decision != contracts.PolicyDeny
			}})
			artifact, err := artifactStore.SaveJSON("artifact_content_discovery_001", job.ID, step.ID, "artifacts/http/content_discovery.json", result.Results)
			if err != nil {
				execErr = err
				break
			}
			artifactsOut = append(artifactsOut, artifact)
			evidenceOut = append(evidenceOut, result.Evidence...)
		case "builtin.tls_inspector":
			if target.Normalized.Scheme != "https" {
				step, err = stepMachine.Skip(step, "target is not HTTPS, skipping TLS inspection")
				if err != nil {
					return WebScanResult{}, err
				}
				skipped = append(skipped, step.ID)
				_ = stateStore.SaveStep(step)
				if err := commandRecorder.Append(command.Finish(toolCall, contracts.StatusSkipped, fmt.Errorf("%s", step.Error))); err != nil {
					return WebScanResult{}, err
				}
				if networkAcquired {
					limiter.Release()
					networkAcquired = false
				}
				toolCallRecorded = true
				emit(step.ID, contracts.EventToolFinished, contracts.EventLevelInfo, step.Tool.Name+" skipped (not HTTPS)")
				emit(step.ID, contracts.EventStepSkipped, contracts.EventLevelInfo, step.Error)
				continue
			}
			tlsResult := builtin.TLSInspect(ctx, builtin.TLSInspectOptions{URL: target.Normalized.BaseURL, Timeout: timeout(step)}, job.ID, step.ID)
			if tlsResult.Error != "" && !tlsResult.Connected {
				execErr = fmt.Errorf("TLS inspection failed: %s", tlsResult.Error)
				break
			}
			evidenceOut = append(evidenceOut, tlsResult.Evidence...)
			findingsOut = append(findingsOut, tlsResult.Findings...)
		case "builtin.tcp_port_check":
			host := target.Normalized.Host
			if host == "" {
				u, _ := url.Parse(target.Normalized.BaseURL)
				if u != nil {
					host = u.Hostname()
				}
			}
			if host == "" {
				execErr = fmt.Errorf("cannot determine host for port check")
				break
			}
			portResult := builtin.TCPPortCheck(ctx, builtin.TCPPortCheckOptions{Host: host, Ports: builtin.CommonWebPorts, Timeout: timeout(step)}, job.ID, step.ID)
			evidenceOut = append(evidenceOut, portResult.Evidence...)
		case "builtin.evidence_indexer":
			// Written after loop so all evidence/artifacts are included.
		case "builtin.finding_normalizer", "builtin.report_reflection_gate", "builtin.report_generator":
			// Written after loop.
		case "plugin.nuclei-safe":
			manifest, ok, err := findPluginManifest("nuclei-safe")
			if err != nil {
				execErr = err
				break
			}
			if !ok {
				step, err = stepMachine.Skip(step, "plugin manifest not found: nuclei-safe")
				if err != nil {
					return WebScanResult{}, err
				}
				skipped = append(skipped, step.ID)
				_ = stateStore.SaveStep(step)
				if err := commandRecorder.Append(command.Finish(toolCall, contracts.StatusSkipped, fmt.Errorf("%s", step.Error))); err != nil {
					return WebScanResult{}, err
				}
				toolCallRecorded = true
				emit(step.ID, contracts.EventToolFinished, contracts.EventLevelWarn, step.Tool.Name+" skipped")
				emit(step.ID, contracts.EventStepSkipped, contracts.EventLevelWarn, step.Error)
				continue
			}
			result, err := pluginruntime.NewRunner(commandRecorder).Run(ctx, manifest, pluginruntime.RunRequest{ID: toolCall.ID, JobID: job.ID, StepID: step.ID, Target: target.Normalized.BaseURL, Scope: scope, WorkDir: layout.Path, Timeout: timeout(step), Recorder: commandRecorder})
			toolCallRecorded = true
			if result.Skipped {
				step, err = stepMachine.Skip(step, result.Reason)
				if err != nil {
					return WebScanResult{}, err
				}
				skipped = append(skipped, step.ID)
				_ = stateStore.SaveStep(step)
				emit(step.ID, contracts.EventToolFinished, contracts.EventLevelWarn, step.Tool.Name+" skipped")
				emit(step.ID, contracts.EventStepSkipped, contracts.EventLevelWarn, step.Error)
				continue
			}
			if err != nil {
				execErr = err
				break
			}
			stdoutArtifact, stderrArtifact, err := savePluginOutputArtifacts(artifactStore, job.ID, step.ID, "nuclei-safe", result)
			if err != nil {
				execErr = err
				break
			}
			if stdoutArtifact.ID != "" {
				artifactsOut = append(artifactsOut, stdoutArtifact)
			}
			if stderrArtifact.ID != "" {
				artifactsOut = append(artifactsOut, stderrArtifact)
			}
			pluginEvidence, pluginFindings := normalizePluginFindings("nuclei-safe", job.ID, step.ID, target.Normalized.BaseURL, stdoutArtifact.ID, result.Findings)
			evidenceOut = append(evidenceOut, pluginEvidence...)
			findingsOut = append(findingsOut, pluginFindings...)
		case "plugin.python-js-route-analyzer":
			manifest, ok, err := findPluginManifest("python-js-route-analyzer")
			if err != nil {
				execErr = err
				break
			}
			if !ok {
				step, err = stepMachine.Skip(step, "plugin manifest not found: python-js-route-analyzer")
				if err != nil {
					return WebScanResult{}, err
				}
				skipped = append(skipped, step.ID)
				_ = stateStore.SaveStep(step)
				if err := commandRecorder.Append(command.Finish(toolCall, contracts.StatusSkipped, fmt.Errorf("%s", step.Error))); err != nil {
					return WebScanResult{}, err
				}
				toolCallRecorded = true
				emit(step.ID, contracts.EventToolFinished, contracts.EventLevelWarn, step.Tool.Name+" skipped")
				emit(step.ID, contracts.EventStepSkipped, contracts.EventLevelWarn, step.Error)
				continue
			}
			result, err := pluginruntime.NewRunner(commandRecorder).Run(ctx, manifest, pluginruntime.RunRequest{
				ID:        toolCall.ID,
				JobID:     job.ID,
				StepID:    step.ID,
				Target:    target.Normalized.BaseURL,
				Scope:     scope,
				WorkDir:   layout.Path,
				Timeout:   timeout(step),
				Recorder:  commandRecorder,
				Input:     map[string]any{"mode": "js_route_analysis"},
				Artifacts: artifactsOut,
				Evidence:  evidenceOut,
			})
			toolCallRecorded = true
			if result.Skipped {
				step, err = stepMachine.Skip(step, result.Reason)
				if err != nil {
					return WebScanResult{}, err
				}
				skipped = append(skipped, step.ID)
				_ = stateStore.SaveStep(step)
				emit(step.ID, contracts.EventToolFinished, contracts.EventLevelWarn, step.Tool.Name+" skipped")
				emit(step.ID, contracts.EventStepSkipped, contracts.EventLevelWarn, step.Error)
				continue
			}
			if err != nil {
				execErr = err
				break
			}
			stdoutArtifact, stderrArtifact, err := savePluginOutputArtifacts(artifactStore, job.ID, step.ID, "python-js-route-analyzer", result)
			if err != nil {
				execErr = err
				break
			}
			if stdoutArtifact.ID != "" {
				artifactsOut = append(artifactsOut, stdoutArtifact)
			}
			if stderrArtifact.ID != "" {
				artifactsOut = append(artifactsOut, stderrArtifact)
			}
			evidenceOut = append(evidenceOut, normalizePluginRoutes("python-js-route-analyzer", job.ID, step.ID, stdoutArtifact.ID, result.Routes)...)
			pluginEvidence, pluginFindings := normalizePluginFindings("python-js-route-analyzer", job.ID, step.ID, target.Normalized.BaseURL, stdoutArtifact.ID, result.Findings)
			evidenceOut = append(evidenceOut, pluginEvidence...)
			findingsOut = append(findingsOut, pluginFindings...)
		default:
			step, err = stepMachine.Skip(step, "not implemented in MVP runtime yet")
			if err != nil {
				return WebScanResult{}, err
			}
			skipped = append(skipped, step.ID)
			_ = stateStore.SaveStep(step)
			if err := commandRecorder.Append(command.Finish(toolCall, contracts.StatusSkipped, fmt.Errorf("%s", step.Error))); err != nil {
				return WebScanResult{}, err
			}
			toolCallRecorded = true
			emit(step.ID, contracts.EventToolFinished, contracts.EventLevelWarn, step.Tool.Name+" skipped")
			emit(step.ID, contracts.EventStepSkipped, contracts.EventLevelWarn, step.Error)
			continue
		}

		if networkAcquired {
			limiter.Release()
		}

		if execErr != nil {
			step, err = stepMachine.Fail(step, execErr)
			if err != nil {
				return WebScanResult{}, err
			}
			job.Summary.StepsFailed++
			_ = stateStore.SaveStep(step)
			if !toolCallRecorded {
				if err := commandRecorder.Append(command.Finish(toolCall, contracts.StatusFailed, execErr)); err != nil {
					return WebScanResult{}, err
				}
				toolCallRecorded = true
			}
			emit(step.ID, contracts.EventToolFinished, contracts.EventLevelError, step.Tool.Name+" failed")
			emit(step.ID, contracts.EventStepFailed, contracts.EventLevelError, execErr.Error())
			return WebScanResult{}, execErr
		}
		step, err = stepMachine.Succeed(step)
		if err != nil {
			return WebScanResult{}, err
		}
		job.Summary.StepsSucceeded++
		_ = stateStore.SaveStep(step)
		if !toolCallRecorded {
			if err := commandRecorder.Append(command.Finish(toolCall, contracts.StatusSucceeded, nil)); err != nil {
				return WebScanResult{}, err
			}
			toolCallRecorded = true
		}
		emit(step.ID, contracts.EventToolFinished, contracts.EventLevelInfo, step.Tool.Name+" finished")
		emit(step.ID, contracts.EventStepFinished, contracts.EventLevelInfo, step.Name+" finished")
		progress(step.Name, "done")
	}

	hasher := evidence.NewHasher()
	evidenceOut, _ = hasher.Dedup(evidenceOut)

	findingStore := findings.NewStore(layout.Path)
	if err := findingStore.WriteEvidence(findings.EvidenceIndex{Artifacts: artifactsOut, Evidence: evidenceOut}); err != nil {
		return WebScanResult{}, err
	}
	if err := findingStore.WriteFindings(findingsOut); err != nil {
		return WebScanResult{}, err
	}
	job.Status = contracts.StatusSucceeded
	finished := contracts.Now()
	job.FinishedAt = &finished
	job.Summary.FindingsTotal = len(findingsOut)

	reportOutput, err := reporting.Generate(layout.Path, reporting.Input{Job: job, Target: target, Scope: scope, Plan: pln.Plan, Artifacts: artifactsOut, Evidence: evidenceOut, Findings: findingsOut, SkippedSteps: skipped})
	if err != nil {
		return WebScanResult{}, err
	}

	if err := stateStore.SaveJob(job); err != nil {
		return WebScanResult{}, err
	}
	if err := artifactStore.WriteJSON("job.json", job); err != nil {
		return WebScanResult{}, err
	}
	emit("", contracts.EventReportCreated, contracts.EventLevelInfo, "report generated")
	emit("", contracts.EventJobFinished, contracts.EventLevelInfo, "job finished")

	return WebScanResult{Job: job, Target: target, Scope: scope, Plan: pln.Plan, Workspace: layout.Path, ReportMD: reportOutput.MDPath, Findings: findingsOut, Evidence: evidenceOut, Artifacts: artifactsOut}, nil
}

func findPluginManifest(name string) (contracts.PluginManifest, bool, error) {
	roots := []string{"plugins", filepath.Join("..", "..", "..", "plugins")}
	for _, root := range roots {
		manifests, err := pluginruntime.NewManager(root).Discover()
		if err != nil {
			return contracts.PluginManifest{}, false, err
		}
		for _, manifest := range manifests {
			if strings.EqualFold(manifest.Name, name) {
				return rebasePluginEntrypoint(root, manifest), true, nil
			}
		}
	}
	return contracts.PluginManifest{}, false, nil
}

func rebasePluginEntrypoint(root string, manifest contracts.PluginManifest) contracts.PluginManifest {
	if filepath.IsAbs(manifest.Entrypoint.Path) {
		return manifest
	}
	if _, err := os.Stat(manifest.Entrypoint.Path); err == nil {
		return manifest
	}
	candidate := filepath.Join(filepath.Dir(root), manifest.Entrypoint.Path)
	if _, err := os.Stat(candidate); err == nil {
		manifest.Entrypoint.Path = candidate
	}
	return manifest
}

func savePluginOutputArtifacts(store artifacts.Store, jobID, stepID, pluginSlug string, result pluginruntime.RunResult) (contracts.Artifact, contracts.Artifact, error) {
	pathSlug := pluginPathSlug(pluginSlug)
	idSlug := pluginIDSlug(pluginSlug)
	var stdoutArtifact contracts.Artifact
	var stderrArtifact contracts.Artifact
	var err error
	if len(result.Stdout) > 0 {
		stdoutArtifact, err = store.SaveBytes("artifact_"+idSlug+"_stdout_001", jobID, stepID, contracts.ArtifactJSONL, "artifacts/tool-results/"+pathSlug+".jsonl", result.Stdout, "application/x-jsonlines")
		if err != nil {
			return contracts.Artifact{}, contracts.Artifact{}, err
		}
		stdoutArtifact.ToolCallID = result.Call.ID
	}
	if len(result.Stderr) > 0 {
		stderrArtifact, err = store.SaveBytes("artifact_"+idSlug+"_stderr_001", jobID, stepID, contracts.ArtifactStderr, "artifacts/stderr/"+pathSlug+".stderr.txt", result.Stderr, "text/plain")
		if err != nil {
			return contracts.Artifact{}, contracts.Artifact{}, err
		}
		stderrArtifact.ToolCallID = result.Call.ID
	}
	return stdoutArtifact, stderrArtifact, nil
}

func normalizePluginRoutes(pluginSlug, jobID, stepID, artifactID string, routes []pluginruntime.PluginRoute) []contracts.Evidence {
	evidence := make([]contracts.Evidence, 0, len(routes))
	idSlug := pluginIDSlug(pluginSlug)
	for i, route := range routes {
		targetRef := route.Resolved
		if targetRef == "" {
			targetRef = route.Raw
		}
		if strings.TrimSpace(targetRef) == "" {
			continue
		}
		data := map[string]any{
			"raw":      route.Raw,
			"resolved": route.Resolved,
			"source":   route.Source,
			"method":   route.Method,
		}
		if route.Data != nil {
			data["plugin_data"] = route.Data
		}
		evidence = append(evidence, contracts.Evidence{
			ID:         fmt.Sprintf("evidence_%s_route_%03d", idSlug, i+1),
			JobID:      jobID,
			StepID:     stepID,
			ArtifactID: artifactID,
			Type:       contracts.EvidenceJSRoute,
			Title:      "JavaScript route discovered by " + pluginSlug,
			Summary:    "JavaScript route discovered by " + pluginSlug + ": " + targetRef,
			TargetRef:  targetRef,
			Data:       data,
			Confidence: contracts.ConfidenceMedium,
			CreatedAt:  contracts.Now(),
			Metadata: contracts.Metadata{
				"plugin": pluginSlug,
			},
		})
	}
	return evidence
}

func normalizePluginFindings(pluginSlug, jobID, stepID, targetRef, artifactID string, pluginFindings []pluginruntime.PluginFinding) ([]contracts.Evidence, []contracts.Finding) {
	evidence := make([]contracts.Evidence, 0, len(pluginFindings))
	findings := make([]contracts.Finding, 0, len(pluginFindings))
	idSlug := pluginIDSlug(pluginSlug)
	for i, pluginFinding := range pluginFindings {
		evidenceID := fmt.Sprintf("evidence_%s_%03d", idSlug, i+1)
		title := pluginFinding.Name
		if title == "" {
			title = pluginFinding.TemplateID
		}
		if title == "" {
			title = pluginSlug + " finding"
		}
		matched := pluginFinding.Matched
		if matched == "" {
			matched = targetRef
		}
		evidence = append(evidence, contracts.Evidence{ID: evidenceID, JobID: jobID, StepID: stepID, ArtifactID: artifactID, Type: contracts.EvidenceToolFinding, Title: title, Summary: pluginSlug + " reported " + title, TargetRef: matched, Data: pluginFinding.Data, Confidence: contracts.ConfidenceMedium, CreatedAt: contracts.Now(), Metadata: contracts.Metadata{"plugin": pluginSlug}})
		findings = append(findings, contracts.Finding{ID: fmt.Sprintf("finding_%s_%03d", idSlug, i+1), JobID: jobID, Title: title, Severity: pluginSeverity(pluginFinding.Severity), Confidence: contracts.ConfidenceMedium, Category: "plugin/" + pluginSlug, AffectedTargets: []string{matched}, EvidenceIDs: []string{evidenceID}, Description: pluginSlug + " reported a low-risk plugin result.", Impact: "Review the matched plugin result and validate applicability in the authorized target context.", Reproduction: []string{"cyberagent scan web --use-plugin " + pluginSlug + " --target <target> --scope <scope>"}, Remediation: "Review the referenced evidence and apply the remediation recommended by the underlying plugin where applicable.", Source: contracts.FindingSource{Tool: "plugin." + pluginSlug, StepID: stepID}, CreatedAt: contracts.Now(), Metadata: contracts.Metadata{"template_id": pluginFinding.TemplateID, "plugin": pluginSlug}})
	}
	return evidence, findings
}

func pluginIDSlug(raw string) string {
	slug := strings.ToLower(strings.TrimSpace(raw))
	replacer := strings.NewReplacer("-", "_", ".", "_", "/", "_", "\\", "_", " ", "_")
	slug = replacer.Replace(slug)
	slug = strings.Trim(slug, "_")
	if slug == "" {
		return "plugin"
	}
	return slug
}

func pluginPathSlug(raw string) string {
	slug := strings.ToLower(strings.TrimSpace(raw))
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-")
	slug = replacer.Replace(slug)
	slug = strings.Trim(slug, "-.")
	if slug == "" || strings.Contains(slug, "..") {
		return "plugin"
	}
	return slug
}

func pluginSeverity(raw string) contracts.Severity {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "low":
		return contracts.SeverityLow
	case "medium":
		return contracts.SeverityMedium
	case "high":
		return contracts.SeverityHigh
	case "critical":
		return contracts.SeverityCritical
	default:
		return contracts.SeverityInfo
	}
}

// BuildScopeFromString parses a comma-separated scope list into a Scope.
func BuildScopeFromString(id, raw string) (contracts.Scope, error) {
	rules := contracts.ScopeRules{Paths: []string{"/*"}, Schemes: []string{"http", "https"}}
	for _, part := range strings.Split(raw, ",") {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		if strings.Contains(item, "://") {
			u, err := url.Parse(item)
			if err != nil {
				return contracts.Scope{}, fmt.Errorf("parse scope URL %q: %w", item, err)
			}
			if u.Hostname() != "" {
				rules.Hosts = append(rules.Hosts, u.Hostname())
			}
			if u.Port() != "" {
				if port, err := parsePort(u.Port()); err == nil {
					rules.Ports = append(rules.Ports, port)
				}
			}
			continue
		}
		if strings.Contains(item, "/") {
			rules.CIDRs = append(rules.CIDRs, item)
			continue
		}
		if host, port, err := net.SplitHostPort(item); err == nil {
			rules.Hosts = append(rules.Hosts, host)
			if p, err := parsePort(port); err == nil {
				rules.Ports = append(rules.Ports, p)
			}
			continue
		}
		if net.ParseIP(item) != nil || item == "localhost" || !strings.Contains(item, ".") {
			rules.Hosts = append(rules.Hosts, item)
		} else {
			rules.Domains = append(rules.Domains, item)
		}
	}
	if len(rules.Hosts) == 0 && len(rules.Domains) == 0 && len(rules.CIDRs) == 0 {
		return contracts.Scope{}, fmt.Errorf("scope has no usable host/domain/cidr boundary")
	}
	return contracts.NewScope(id, rules), nil
}

func joinURLPath(baseURL, p string) string {
	base, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}
	base.Path = p
	base.RawQuery = ""
	return base.String()
}

func parsePort(raw string) (int, error) {
	var port int
	_, err := fmt.Sscanf(raw, "%d", &port)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q", raw)
	}
	return port, nil
}

func outputRootAndJobID(out string) (string, string) {
	if strings.TrimSpace(out) == "" {
		return "runs", ""
	}
	clean := filepath.Clean(out)
	return filepath.Dir(clean), filepath.Base(clean)
}

func enabledPlugins(req WebScanRequest) []string {
	if req.NoExternalTools {
		return nil
	}
	return req.EnabledPlugins
}

func timeout(step contracts.Step) time.Duration {
	if step.TimeoutSeconds <= 0 {
		return 10 * time.Second
	}
	return time.Duration(step.TimeoutSeconds) * time.Second
}

var eventCounter atomic.Int64
var toolCallCounter atomic.Int64

func nextEventID() string {
	return fmt.Sprintf("event_%06d", eventCounter.Add(1))
}

func nextToolCallID() string {
	return fmt.Sprintf("toolcall_%06d", toolCallCounter.Add(1))
}

func isNetworkStep(toolName string) bool {
	switch toolName {
	case "builtin.http_probe", "builtin.http_fetcher", "builtin.robots_fetcher",
		"builtin.sitemap_fetcher", "builtin.js_asset_collector",
		"builtin.content_discovery_small", "builtin.tls_inspector",
		"builtin.tcp_port_check":
		return true
	default:
		return false
	}
}
