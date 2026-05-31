package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type agentSecurityToolCatalogFile struct {
	SchemaVersion string                         `json:"schema_version"`
	Description   string                         `json:"description,omitempty"`
	Sources       map[string]string              `json:"sources,omitempty"`
	Tools         []agentSecurityToolCatalogItem `json:"tools"`
}

type agentSecurityToolCatalogItem struct {
	Name   string `json:"name"`
	Source string `json:"source"`
	Path   string `json:"path"`
}

type agentSecurityToolSpec struct {
	Name          string          `json:"name"`
	Source        string          `json:"source"`
	Path          string          `json:"path,omitempty"`
	PluginPath    string          `json:"plugin_path,omitempty"`
	IntentTags    []string        `json:"intent_tags,omitempty"`
	CanDo         []string        `json:"can_do,omitempty"`
	CannotDo      []string        `json:"cannot_do,omitempty"`
	Risk          string          `json:"risk,omitempty"`
	RequiredLevel string          `json:"required_level,omitempty"`
	AutoRun       bool            `json:"auto_run"`
	InputSchema   json.RawMessage `json:"input_schema,omitempty"`
	Examples      []string        `json:"examples,omitempty"`
}

func agentSecurityToolCatalog() []agentSecurityToolSpec {
	catalogPath := agentAssetPath("security-tools", "catalog.json")
	data, err := os.ReadFile(catalogPath)
	if err != nil {
		return nil
	}
	var catalog agentSecurityToolCatalogFile
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil
	}

	out := make([]agentSecurityToolSpec, 0, len(catalog.Tools))
	for _, item := range catalog.Tools {
		path := strings.TrimSpace(item.Path)
		if path == "" {
			continue
		}
		resolved := path
		if !filepath.IsAbs(resolved) {
			resolved = agentAssetPath(path)
		}
		specData, err := os.ReadFile(resolved)
		if err != nil {
			continue
		}
		var spec agentSecurityToolSpec
		if err := json.Unmarshal(specData, &spec); err != nil {
			continue
		}
		if strings.TrimSpace(spec.Name) == "" {
			spec.Name = strings.TrimSpace(item.Name)
		}
		if strings.TrimSpace(spec.Source) == "" {
			spec.Source = strings.TrimSpace(item.Source)
		}
		spec.Path = path
		out = append(out, spec)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Source == out[j].Source {
			return out[i].Name < out[j].Name
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func agentSecurityToolCatalogByName() map[string]agentSecurityToolSpec {
	tools := agentSecurityToolCatalog()
	out := make(map[string]agentSecurityToolSpec, len(tools))
	for _, tool := range tools {
		name := normalizeAgentPlannerActionName(tool.Name)
		if name == "" {
			name = strings.TrimSpace(tool.Name)
		}
		if name != "" {
			out[name] = tool
		}
	}
	return out
}
