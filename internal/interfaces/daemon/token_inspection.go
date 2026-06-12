package daemon

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// TokenFileInspection is a safe-to-log diagnostic view of a daemon bearer token
// file. It never includes the token value itself.
type TokenFileInspection struct {
	Path              string `json:"path"`
	Exists            bool   `json:"exists"`
	Readable          bool   `json:"readable"`
	Empty             bool   `json:"empty"`
	Length            int    `json:"length"`
	Mode              string `json:"mode,omitempty"`
	SecurePermissions bool   `json:"secure_permissions"`
	Error             string `json:"error,omitempty"`
}

// InspectTokenFile returns a stable, non-secret diagnostic view of a daemon
// token file. Missing files are not errors because NewServer can create them;
// empty, unreadable, directory, or loose-permission files are reported without
// returning or logging the token contents.
func InspectTokenFile(path string) TokenFileInspection {
	path = strings.TrimSpace(path)
	inspection := TokenFileInspection{Path: path}
	if path == "" {
		inspection.Error = "token file path is required"
		inspection.Empty = true
		return inspection
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return inspection
		}
		inspection.Error = err.Error()
		return inspection
	}
	inspection.Exists = true
	inspection.Mode = fmt.Sprintf("%04o", info.Mode().Perm())
	inspection.SecurePermissions = tokenFilePermissionsSecure(info.Mode().Perm())
	if info.IsDir() {
		inspection.Error = "token file path is a directory"
		return inspection
	}
	data, err := os.ReadFile(path)
	if err != nil {
		inspection.Error = err.Error()
		return inspection
	}
	inspection.Readable = true
	token := strings.TrimSpace(string(data))
	inspection.Empty = token == ""
	inspection.Length = len(token)
	return inspection
}

// RedactBearerTokenForLog returns a stable log-safe placeholder for a bearer
// token. It preserves only the trimmed token length for diagnostics.
func RedactBearerTokenForLog(token string) string {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "<redacted:empty>"
	}
	return fmt.Sprintf("<redacted:len=%d>", len(trimmed))
}

func tokenFilePermissionsSecure(perm os.FileMode) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return perm&0o077 == 0 && perm&0o400 != 0
}
