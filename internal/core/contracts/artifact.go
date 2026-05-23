package contracts

import "time"

// ArtifactType identifies raw or semi-structured output kind.
type ArtifactType string

const (
	ArtifactHTML        ArtifactType = "html"
	ArtifactHTTPHeaders ArtifactType = "http_headers"
	ArtifactJSON        ArtifactType = "json"
	ArtifactJSONL       ArtifactType = "jsonl"
	ArtifactText        ArtifactType = "text"
	ArtifactXML         ArtifactType = "xml"
	ArtifactScreenshot  ArtifactType = "screenshot"
	ArtifactStdout      ArtifactType = "stdout"
	ArtifactStderr      ArtifactType = "stderr"
	ArtifactBinary      ArtifactType = "binary"
	ArtifactReport      ArtifactType = "report"
)

// Artifact describes a stored output file.
type Artifact struct {
	ID         string       `json:"id"`
	JobID      string       `json:"job_id"`
	StepID     string       `json:"step_id,omitempty"`
	ToolCallID string       `json:"toolcall_id,omitempty"`
	Type       ArtifactType `json:"type"`
	Path       string       `json:"path"`
	SHA256     string       `json:"sha256,omitempty"`
	SizeBytes  int64        `json:"size_bytes,omitempty"`
	MIMEType   string       `json:"mime_type,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	Redacted   bool         `json:"redacted"`
	Metadata   Metadata     `json:"metadata,omitempty"`
}
