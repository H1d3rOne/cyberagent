package contracts

import "time"

// Metadata stores optional implementation-specific fields without changing the public contract.
type Metadata map[string]any

// Status describes lifecycle state for jobs, plans, steps, tool calls, and reports.
type Status string

const (
	StatusCreated            Status = "created"
	StatusPlanning           Status = "planning"
	StatusReady              Status = "ready"
	StatusRunning            Status = "running"
	StatusWaitingApproval    Status = "waiting_approval"
	StatusPaused             Status = "paused"
	StatusSucceeded          Status = "succeeded"
	StatusFailed             Status = "failed"
	StatusCancelled          Status = "cancelled"
	StatusPartiallySucceeded Status = "partially_succeeded"
	StatusPending            Status = "pending"
	StatusBlocked            Status = "blocked"
	StatusSkipped            Status = "skipped"
)

// RiskLevel describes action risk before a step/tool is executed.
type RiskLevel string

const (
	RiskInfo      RiskLevel = "info"
	RiskLow       RiskLevel = "low"
	RiskMedium    RiskLevel = "medium"
	RiskHigh      RiskLevel = "high"
	RiskCritical  RiskLevel = "critical"
	RiskForbidden RiskLevel = "forbidden"
)

// Severity describes finding severity.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Confidence describes how reliable a derived evidence/finding is.
type Confidence string

const (
	ConfidenceLow       Confidence = "low"
	ConfidenceMedium    Confidence = "medium"
	ConfidenceHigh      Confidence = "high"
	ConfidenceConfirmed Confidence = "confirmed"
)

// Now is a small seam for constructors and tests.
var Now = func() time.Time { return time.Now().UTC() }
