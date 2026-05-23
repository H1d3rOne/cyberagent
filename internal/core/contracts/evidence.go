package contracts

import "time"

// EvidenceType identifies the extracted evidence kind.
type EvidenceType string

const (
	EvidenceHTTPResponse    EvidenceType = "http_response"
	EvidenceHTTPHeader      EvidenceType = "http_header"
	EvidenceCookieAttribute EvidenceType = "cookie_attribute"
	EvidenceHTMLLink        EvidenceType = "html_link"
	EvidenceHTMLForm        EvidenceType = "html_form"
	EvidenceJSRoute         EvidenceType = "js_route"
	EvidenceAPIHint         EvidenceType = "api_hint"
	EvidencePortStatus      EvidenceType = "port_status"
	EvidenceTLSCertificate  EvidenceType = "tls_certificate"
	EvidenceToolFinding     EvidenceType = "tool_finding"
)

// Evidence is extracted and citable proof.
type Evidence struct {
	ID         string         `json:"id"`
	JobID      string         `json:"job_id"`
	StepID     string         `json:"step_id,omitempty"`
	ArtifactID string         `json:"artifact_id,omitempty"`
	Type       EvidenceType   `json:"type"`
	Title      string         `json:"title"`
	Summary    string         `json:"summary"`
	TargetRef  string         `json:"target_ref,omitempty"`
	Data       map[string]any `json:"data,omitempty"`
	Confidence Confidence     `json:"confidence"`
	CreatedAt  time.Time      `json:"created_at"`
	Metadata   Metadata       `json:"metadata,omitempty"`
}
