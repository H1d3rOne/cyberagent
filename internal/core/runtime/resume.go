package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
	"github.com/H1d3rOne/cyberagent/internal/core/events"
	"github.com/H1d3rOne/cyberagent/internal/core/evidence"
	"github.com/H1d3rOne/cyberagent/internal/core/findings"
	"github.com/H1d3rOne/cyberagent/internal/core/policy"
	"github.com/H1d3rOne/cyberagent/internal/core/reporting"
	"github.com/H1d3rOne/cyberagent/internal/core/scheduler"
	pluginruntime "github.com/H1d3rOne/cyberagent/internal/plugins"
	"github.com/H1d3rOne/cyberagent/internal/runtime/artifacts"
	"github.com/H1d3rOne/cyberagent/internal/runtime/command"
	"github.com/H1d3rOne/cyberagent/internal/runtime/runs"
	"github.com/H1d3rOne/cyberagent/internal/runtime/state"
)

// ResumeApprovedRequest describes a request to resume approved steps in an existing run workspace.
type ResumeApprovedRequest struct {
	RunDir string
}

// ResumeApprovedResult summarizes what the resume pass did.
type ResumeApprovedResult struct {
	Resumed []string
	Skipped []string
}

// ResumeApproved resumes approved, recoverable steps in an existing run workspace.
//
// v0.1.1 MVP intentionally supports only plugin.ffuf-low-rate. Other approved
// steps are reported as skipped/unsupported without mutating their state.
func ResumeApproved(ctx context.Context, req ResumeApprovedRequest) (ResumeApprovedResult, error) {
	runDir := strings.TrimSpace(req.RunDir)
	if runDir == "" {
		return ResumeApprovedResult{}, fmt.Errorf("run dir is required")
	}
	layout := layoutFromRunDir(runDir)

	var target contracts.Target
	if err := readJSONFile(layout.TargetPath, &target); err != nil {
		return ResumeApprovedResult{}, err
	}
	var scope contracts.Scope
	if err := readJSONFile(layout.ScopePath, &scope); err != nil {
		return ResumeApprovedResult{}, err
	}
	var job contracts.Job
	if err := readJSONFile(layout.JobPath, &job); err != nil {
		return ResumeApprovedResult{}, err
	}
	var plan contracts.Plan
	if err := readJSONFile(layout.PlanPath, &plan); err != nil {
		return ResumeApprovedResult{}, err
	}
	evidenceIndex, err := readEvidenceIndex(layout.EvidenceIndex)
	if err != nil {
		return ResumeApprovedResult{}, err
	}
	existingFindings, err := readFindings(layout.FindingsPath)
	if err != nil {
		return ResumeApprovedResult{}, err
	}
	approvals, err := readApprovals(layout.ApprovalsPath)
	if err != nil {
		return ResumeApprovedResult{}, err
	}

	stateStore, err := state.Open(layout.BoltPath)
	if err != nil {
		return ResumeApprovedResult{}, fmt.Errorf("open state store: %w", err)
	}
	defer stateStore.Close()

	eventStore := events.NewStore(layout.EventsPath)
	commandRecorder := command.NewRecorder(layout.CommandsPath)
	artifactStore := artifacts.NewStore(layout.Path)
	guard := policy.NewEngine()
	stepMachine := scheduler.NewStepStateMachine()

	result := ResumeApprovedResult{}
	for _, approval := range approvals {
		if approval.Status != contracts.HITLApproved {
			continue
		}
		step, err := stateStore.LoadStep(approval.StepID)
		if err != nil {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: load step: %v", approval.StepID, err))
			continue
		}
		persistedApproval, err := stateStore.LoadHITLRequest(approval.ID)
		if err != nil {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: load approval state: %v", step.ID, err))
			continue
		}
		if persistedApproval.Status != contracts.HITLApproved {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: approval state is %s", step.ID, persistedApproval.Status))
			continue
		}
		previousDecision, err := stateStore.LoadPolicyDecision(approval.PolicyDecisionID)
		if err != nil {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: load policy decision: %v", step.ID, err))
			continue
		}
		if previousDecision.Decision != contracts.PolicyRequireApproval {
			result.Skipped = append(result.Skipped, fmt.Sprintf("%s: previous policy decision is %s", step.ID, previousDecision.Decision))
			continue
		}
		if step.Status == contracts.StatusSucceeded {
			result.Skipped = append(result.Skipped, step.ID+": already succeeded")
			continue
		}
		if step.Tool.Name != "plugin.ffuf-low-rate" {
			result.Skipped = append(result.Skipped, step.ID+": unsupported approved step "+step.Tool.Name)
			continue
		}

		resumed, skipReason, err := resumeFFUFLowRate(ctx, resumeContext{
			layout:           layout,
			target:           target,
			scope:            scope,
			job:              &job,
			plan:             plan,
			step:             step,
			approval:         approval,
			previousDecision: previousDecision,
			stateStore:       stateStore,
			eventStore:       eventStore,
			commandRecorder:  commandRecorder,
			artifactStore:    artifactStore,
			guard:            guard,
			stepMachine:      stepMachine,
			evidenceIndex:    &evidenceIndex,
			findings:         &existingFindings,
		})
		if err != nil {
			return result, err
		}
		if resumed {
			result.Resumed = append(result.Resumed, step.ID)
		} else {
			result.Skipped = append(result.Skipped, step.ID+": "+skipReason)
		}
	}
	return result, nil
}

type resumeContext struct {
	layout           runs.Layout
	target           contracts.Target
	scope            contracts.Scope
	job              *contracts.Job
	plan             contracts.Plan
	step             contracts.Step
	approval         contracts.HITLRequest
	previousDecision contracts.PolicyDecision
	stateStore       *state.Store
	eventStore       *events.Store
	commandRecorder  *command.Recorder
	artifactStore    artifacts.Store
	guard            policy.Engine
	stepMachine      scheduler.StepStateMachine
	evidenceIndex    *findings.EvidenceIndex
	findings         *[]contracts.Finding
}

func resumeFFUFLowRate(ctx context.Context, rc resumeContext) (bool, string, error) {
	step := rc.step
	emit := func(typ contracts.EventType, level contracts.EventLevel, message string, data map[string]any) error {
		event := contracts.NewEvent(resumeEventID(step.ID, string(typ)), rc.job.ID, step.ID, typ, level, message)
		event.Data = data
		return rc.eventStore.Append(event)
	}

	started := contracts.Now()
	step.Status = contracts.StatusRunning
	step.Error = ""
	step.StartedAt = &started
	step.FinishedAt = nil
	if err := rc.stateStore.SaveStep(step); err != nil {
		return false, "", fmt.Errorf("save resumed step: %w", err)
	}
	if err := emit(contracts.EventStepStarted, contracts.EventLevelInfo, step.Name+" resumed after approval", map[string]any{"approval_request_id": rc.approval.ID}); err != nil {
		return false, "", err
	}

	scopeDecision := rc.guard.CheckURL("policy_resume_scope_"+step.ID, rc.job.ID, step.ID, rc.target.Normalized.BaseURL, rc.scope)
	if err := rc.stateStore.SavePolicyDecision(scopeDecision); err != nil {
		return false, "", fmt.Errorf("save resume scope decision: %w", err)
	}
	if err := emit(contracts.EventPolicyChecked, contracts.EventLevelInfo, string(scopeDecision.Decision), map[string]any{
		"policy_decision_id": scopeDecision.ID,
		"decision":           string(scopeDecision.Decision),
		"reasons":            scopeDecision.Reasons,
	}); err != nil {
		return false, "", err
	}
	if scopeDecision.Decision == contracts.PolicyDeny {
		failed, transitionErr := rc.stepMachine.Fail(step, fmt.Errorf("resume denied by scope guard: %s", strings.Join(scopeDecision.Reasons, ",")))
		if transitionErr != nil {
			return false, "", transitionErr
		}
		if err := rc.stateStore.SaveStep(failed); err != nil {
			return false, "", err
		}
		if err := emit(contracts.EventStepFailed, contracts.EventLevelError, failed.Error, nil); err != nil {
			return false, "", err
		}
		return false, failed.Error, nil
	}

	approvalDecision := contracts.PolicyDecision{
		ID:            "policy_resume_approval_" + step.ID,
		JobID:         rc.job.ID,
		StepID:        step.ID,
		Decision:      contracts.PolicyAllowWithLimits,
		Reasons:       []string{"approved_by_human", "previous_requires_approval"},
		AppliedLimits: map[string]any{},
		CreatedAt:     contracts.Now(),
	}
	if rc.scope.Limits.MaxRequestsPerSecond > 0 {
		approvalDecision.AppliedLimits["max_requests_per_second"] = rc.scope.Limits.MaxRequestsPerSecond
	}
	if rc.scope.Limits.MaxConcurrencyPerHost > 0 {
		approvalDecision.AppliedLimits["max_concurrency_per_host"] = rc.scope.Limits.MaxConcurrencyPerHost
	}
	if rc.scope.Limits.MaxTotalRequests > 0 {
		approvalDecision.AppliedLimits["max_total_requests"] = rc.scope.Limits.MaxTotalRequests
	}
	if rc.scope.Limits.MaxRuntimeSeconds > 0 {
		approvalDecision.AppliedLimits["max_runtime_seconds"] = rc.scope.Limits.MaxRuntimeSeconds
	}
	if err := rc.stateStore.SavePolicyDecision(approvalDecision); err != nil {
		return false, "", fmt.Errorf("save resume approval decision: %w", err)
	}
	if err := emit(contracts.EventPolicyChecked, contracts.EventLevelInfo, string(approvalDecision.Decision), map[string]any{
		"policy_decision_id":          approvalDecision.ID,
		"decision":                    string(approvalDecision.Decision),
		"reasons":                     approvalDecision.Reasons,
		"approval_request_id":         rc.approval.ID,
		"previous_policy_decision_id": rc.previousDecision.ID,
	}); err != nil {
		return false, "", err
	}

	manifest, ok, err := findPluginManifest("ffuf-low-rate")
	if err != nil {
		return false, "", err
	}
	if !ok {
		skipped, transitionErr := rc.stepMachine.Skip(step, "plugin manifest not found: ffuf-low-rate")
		if transitionErr != nil {
			return false, "", transitionErr
		}
		if err := rc.stateStore.SaveStep(skipped); err != nil {
			return false, "", err
		}
		if err := emit(contracts.EventStepSkipped, contracts.EventLevelWarn, skipped.Error, nil); err != nil {
			return false, "", err
		}
		return false, skipped.Error, nil
	}

	toolCallID := "toolcall_resume_" + pluginIDSlug(step.ID) + "_" + fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	if err := emit(contracts.EventToolStarted, contracts.EventLevelInfo, step.Tool.Name+" started", map[string]any{"tool_call_id": toolCallID}); err != nil {
		return false, "", err
	}
	pluginResult, err := pluginruntime.NewRunner(rc.commandRecorder).Run(ctx, manifest, pluginruntime.RunRequest{
		ID:       toolCallID,
		JobID:    rc.job.ID,
		StepID:   step.ID,
		Target:   rc.target.Normalized.BaseURL,
		Scope:    rc.scope,
		WorkDir:  rc.layout.Path,
		Timeout:  timeout(step),
		Recorder: rc.commandRecorder,
	})
	if pluginResult.Skipped {
		skipped, transitionErr := rc.stepMachine.Skip(step, pluginResult.Reason)
		if transitionErr != nil {
			return false, "", transitionErr
		}
		if err := rc.stateStore.SaveStep(skipped); err != nil {
			return false, "", err
		}
		if err := emit(contracts.EventToolFinished, contracts.EventLevelWarn, step.Tool.Name+" skipped", map[string]any{"tool_call_id": pluginResult.Call.ID}); err != nil {
			return false, "", err
		}
		if err := emit(contracts.EventStepSkipped, contracts.EventLevelWarn, skipped.Error, nil); err != nil {
			return false, "", err
		}
		return false, pluginResult.Reason, nil
	}
	if err != nil {
		failed, transitionErr := rc.stepMachine.Fail(step, err)
		if transitionErr != nil {
			return false, "", transitionErr
		}
		if err := rc.stateStore.SaveStep(failed); err != nil {
			return false, "", err
		}
		if emitErr := emit(contracts.EventToolFinished, contracts.EventLevelError, step.Tool.Name+" failed", map[string]any{"tool_call_id": pluginResult.Call.ID}); emitErr != nil {
			return false, "", emitErr
		}
		if emitErr := emit(contracts.EventStepFailed, contracts.EventLevelError, err.Error(), nil); emitErr != nil {
			return false, "", emitErr
		}
		return false, "", err
	}

	stdoutArtifact, stderrArtifact, err := savePluginOutputArtifacts(rc.artifactStore, rc.job.ID, step.ID, "ffuf-low-rate", pluginResult)
	if err != nil {
		return false, "", err
	}
	removeStepData(rc.evidenceIndex, rc.findings, step.ID)
	if stdoutArtifact.ID != "" {
		rc.evidenceIndex.Artifacts = append(rc.evidenceIndex.Artifacts, stdoutArtifact)
	}
	if stderrArtifact.ID != "" {
		rc.evidenceIndex.Artifacts = append(rc.evidenceIndex.Artifacts, stderrArtifact)
	}
	pluginEvidence, pluginFindings := normalizePluginFindings("ffuf-low-rate", rc.job.ID, step.ID, rc.target.Normalized.BaseURL, stdoutArtifact.ID, pluginResult.Findings)
	rc.evidenceIndex.Evidence = append(rc.evidenceIndex.Evidence, pluginEvidence...)
	*rc.findings = append(*rc.findings, pluginFindings...)

	hasher := evidence.NewHasher()
	rc.evidenceIndex.Evidence, _ = hasher.Dedup(rc.evidenceIndex.Evidence)
	findingStore := findings.NewStore(rc.layout.Path)
	if err := findingStore.WriteEvidence(*rc.evidenceIndex); err != nil {
		return false, "", err
	}
	if err := findingStore.WriteFindings(*rc.findings); err != nil {
		return false, "", err
	}

	succeeded, err := rc.stepMachine.Succeed(step)
	if err != nil {
		return false, "", err
	}
	if err := rc.stateStore.SaveStep(succeeded); err != nil {
		return false, "", err
	}
	if err := updateJobSummaryAndReport(rc, len(*rc.findings)); err != nil {
		return false, "", err
	}
	if err := emit(contracts.EventToolFinished, contracts.EventLevelInfo, step.Tool.Name+" finished", map[string]any{"tool_call_id": pluginResult.Call.ID}); err != nil {
		return false, "", err
	}
	if err := emit(contracts.EventStepFinished, contracts.EventLevelInfo, step.Name+" finished after approval", map[string]any{"approval_request_id": rc.approval.ID}); err != nil {
		return false, "", err
	}
	if err := rc.eventStore.Append(contracts.NewEvent(resumeEventID("", string(contracts.EventReportCreated)), rc.job.ID, "", contracts.EventReportCreated, contracts.EventLevelInfo, "report regenerated after resume")); err != nil {
		return false, "", err
	}
	return true, "", nil
}

func updateJobSummaryAndReport(rc resumeContext, findingsTotal int) error {
	steps, err := rc.stateStore.ListSteps(rc.job.ID)
	if err != nil {
		return fmt.Errorf("list steps for summary: %w", err)
	}
	skippedSteps := make([]string, 0)
	succeeded := 0
	failed := 0
	for _, step := range steps {
		switch step.Status {
		case contracts.StatusSucceeded:
			succeeded++
		case contracts.StatusFailed:
			failed++
		case contracts.StatusSkipped:
			skippedSteps = append(skippedSteps, step.ID)
		}
	}
	rc.job.Summary.StepsTotal = len(rc.plan.Steps)
	rc.job.Summary.StepsSucceeded = succeeded
	rc.job.Summary.StepsFailed = failed
	rc.job.Summary.FindingsTotal = findingsTotal
	rc.job.Status = contracts.StatusSucceeded
	finished := contracts.Now()
	rc.job.FinishedAt = &finished

	if err := rc.stateStore.SaveJob(*rc.job); err != nil {
		return fmt.Errorf("save job after resume: %w", err)
	}
	if err := rc.artifactStore.WriteJSON("job.json", *rc.job); err != nil {
		return fmt.Errorf("write job.json after resume: %w", err)
	}
	if _, err := reporting.Generate(rc.layout.Path, reporting.Input{
		Job:          *rc.job,
		Target:       rc.target,
		Scope:        rc.scope,
		Plan:         rc.plan,
		Artifacts:    rc.evidenceIndex.Artifacts,
		Evidence:     rc.evidenceIndex.Evidence,
		Findings:     *rc.findings,
		SkippedSteps: skippedSteps,
		Limitations:  []string{"Approved ffuf-low-rate step was resumed explicitly with replay --resume-approved."},
	}); err != nil {
		return fmt.Errorf("regenerate report after resume: %w", err)
	}
	return nil
}

func layoutFromRunDir(runDir string) runs.Layout {
	clean := filepath.Clean(runDir)
	return runs.NewLayout(filepath.Dir(clean), filepath.Base(clean))
}

func readJSONFile(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func readEvidenceIndex(path string) (findings.EvidenceIndex, error) {
	var index findings.EvidenceIndex
	if err := readJSONFile(path, &index); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return findings.EvidenceIndex{}, nil
		}
		return findings.EvidenceIndex{}, err
	}
	return index, nil
}

func readFindings(path string) ([]contracts.Finding, error) {
	var items []contracts.Finding
	if err := readJSONFile(path, &items); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return items, nil
}

func readApprovals(path string) ([]contracts.HITLRequest, error) {
	var approvals []contracts.HITLRequest
	if err := readJSONFile(path, &approvals); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return approvals, nil
}

func removeStepData(index *findings.EvidenceIndex, findingList *[]contracts.Finding, stepID string) {
	artifactsOut := index.Artifacts[:0]
	for _, artifact := range index.Artifacts {
		if artifact.StepID != stepID {
			artifactsOut = append(artifactsOut, artifact)
		}
	}
	index.Artifacts = artifactsOut

	evidenceOut := index.Evidence[:0]
	for _, item := range index.Evidence {
		if item.StepID != stepID {
			evidenceOut = append(evidenceOut, item)
		}
	}
	index.Evidence = evidenceOut

	findingsOut := (*findingList)[:0]
	for _, item := range *findingList {
		if item.Source.StepID != stepID {
			findingsOut = append(findingsOut, item)
		}
	}
	*findingList = findingsOut
}

func resumeEventID(stepID, typ string) string {
	slug := pluginIDSlug(stepID)
	if slug == "" {
		slug = "job"
	}
	return fmt.Sprintf("event_resume_%s_%s_%d", slug, pluginIDSlug(typ), time.Now().UTC().UnixNano())
}
