package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// DependencyStatus describes one plugin dependency check.
type DependencyStatus struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail"`
}

// SchemaStatus describes one plugin input/output schema check.
type SchemaStatus struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Required bool   `json:"required"`
	OK       bool   `json:"ok"`
	Detail   string `json:"detail"`
}

// Health describes plugin dependency and contract health.
type Health struct {
	Plugin       contracts.PluginManifest `json:"plugin"`
	Dependencies []DependencyStatus       `json:"dependencies"`
	Schemas      []SchemaStatus           `json:"schemas"`
	OK           bool                     `json:"ok"`
}

// Doctor checks plugin dependencies and declared schema contracts.
func Doctor(manifest contracts.PluginManifest) Health {
	health := Health{Plugin: manifest, OK: true}
	for _, dep := range manifest.Dependencies {
		status := DependencyStatus{Name: dep.Name, Kind: dep.Kind, Required: dep.Required}
		switch dep.Kind {
		case "binary":
			path, err := exec.LookPath(dep.Name)
			if err != nil {
				status.OK = false
				status.Detail = "not found in PATH"
			} else {
				status.OK = true
				status.Detail = path
			}
		default:
			status.OK = !dep.Required
			status.Detail = "unsupported dependency kind"
		}
		if dep.Required && !status.OK {
			health.OK = false
		}
		health.Dependencies = append(health.Dependencies, status)
	}

	for _, status := range checkDeclaredSchemas(manifest) {
		if status.Required && !status.OK {
			health.OK = false
		}
		health.Schemas = append(health.Schemas, status)
	}
	return health
}

func checkDeclaredSchemas(manifest contracts.PluginManifest) []SchemaStatus {
	checks := []SchemaStatus{}
	if strings.TrimSpace(manifest.InputSchema) != "" {
		checks = append(checks, checkSchema("input_schema", manifest.InputSchema, true))
	}
	if strings.TrimSpace(manifest.OutputSchema) != "" {
		checks = append(checks, checkSchema("output_schema", manifest.OutputSchema, true))
	}
	return checks
}

func checkSchema(name, path string, required bool) SchemaStatus {
	status := SchemaStatus{Name: name, Path: path, Required: required}
	body, err := os.ReadFile(path)
	if err != nil {
		status.OK = false
		if os.IsNotExist(err) {
			status.Detail = "schema file not found"
		} else {
			status.Detail = err.Error()
		}
		return status
	}

	var decoded any
	if err := json.Unmarshal(body, &decoded); err != nil {
		status.OK = false
		status.Detail = "invalid JSON schema: " + err.Error()
		return status
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		status.OK = false
		status.Detail = "schema root must be a JSON object"
		return status
	}
	if _, ok := object["type"]; !ok {
		status.OK = false
		status.Detail = "schema root missing type"
		return status
	}
	status.OK = true
	status.Detail = fmt.Sprintf("valid JSON schema (%d bytes)", len(body))
	return status
}
