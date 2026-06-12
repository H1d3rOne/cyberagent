package daemon

import (
	"os"
	"strings"
)

type adminBootstrapState struct {
	Configured bool
	Ready      bool
	Token      string
	Message    string
}

type adminBootstrapStatus struct {
	Configured bool   `json:"configured"`
	Ready      bool   `json:"ready"`
	Message    string `json:"message,omitempty"`
}

func loadAdminBootstrapState(cfg AdminConfig) adminBootstrapState {
	state := adminBootstrapState{}
	path := strings.TrimSpace(cfg.BootstrapTokenFile)
	if path == "" {
		state.Message = "admin bootstrap is not configured"
		return state
	}
	data, err := os.ReadFile(path)
	if err != nil {
		state.Configured = true
		state.Ready = false
		state.Message = "admin bootstrap token file is not readable"
		return state
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		state.Configured = true
		state.Ready = false
		state.Message = "admin bootstrap token file is empty"
		return state
	}
	state.Configured = true
	state.Ready = true
	state.Token = token
	state.Message = "admin bootstrap token loaded"
	return state
}

func (s adminBootstrapState) Status() adminBootstrapStatus {
	return adminBootstrapStatus{
		Configured: s.Configured,
		Ready:      s.Ready,
		Message:    s.Message,
	}
}
