package doctor

import (
	"context"
	"os"
	"os/exec"
	"runtime"
)

// Status is the health status of a doctor check.
type Status string

const (
	StatusOK      Status = "ok"
	StatusWarn    Status = "warn"
	StatusMissing Status = "missing"
	StatusError   Status = "error"
)

// CheckResult is a single environment/dependency check result.
type CheckResult struct {
	Name     string `json:"name"`
	Status   Status `json:"status"`
	Required bool   `json:"required"`
	Detail   string `json:"detail"`
}

// Report is the complete doctor report.
type Report struct {
	OS      string        `json:"os"`
	Arch    string        `json:"arch"`
	Checks  []CheckResult `json:"checks"`
	Healthy bool          `json:"healthy"`
}

// Run executes MVP environment checks.
func Run(ctx context.Context) Report {
	checks := []CheckResult{
		checkWritableDir("workspace", ".", true),
		checkBinary(ctx, "python3", false),
		checkBinary(ctx, "nuclei", false),
		checkBinary(ctx, "ffuf", false),
		checkBinary(ctx, "nmap", false),
	}

	healthy := true
	for _, check := range checks {
		if check.Required && check.Status != StatusOK {
			healthy = false
			break
		}
	}

	return Report{
		OS:      runtime.GOOS,
		Arch:    runtime.GOARCH,
		Checks:  checks,
		Healthy: healthy,
	}
}

func checkWritableDir(name, path string, required bool) CheckResult {
	file, err := os.CreateTemp(path, ".cyberagent-doctor-*")
	if err != nil {
		return CheckResult{Name: name, Status: StatusError, Required: required, Detail: err.Error()}
	}
	filename := file.Name()
	_ = file.Close()
	_ = os.Remove(filename)
	return CheckResult{Name: name, Status: StatusOK, Required: required, Detail: path + " is writable"}
}

func checkBinary(ctx context.Context, name string, required bool) CheckResult {
	path, err := exec.LookPath(name)
	if err != nil {
		status := StatusMissing
		if required {
			status = StatusError
		}
		return CheckResult{Name: name, Status: status, Required: required, Detail: "not found in PATH"}
	}

	select {
	case <-ctx.Done():
		return CheckResult{Name: name, Status: StatusWarn, Required: required, Detail: ctx.Err().Error()}
	default:
	}

	return CheckResult{Name: name, Status: StatusOK, Required: required, Detail: path}
}
