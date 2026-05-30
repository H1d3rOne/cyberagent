package plan

import "strings"

// Operation levels in ascending privilege order.
const (
	LevelReadOnly       = "read_only"
	LevelWorkspaceWrite = "workspace_write"
	LevelSafeScan       = "safe_scan"
	LevelApprovalOps    = "approval_ops"
	LevelAdvanced       = "advanced"
)

// NormalizeLevel maps user/LLM-provided level strings to a canonical level.
// Mirrors daemon.normalizeAgentOperationLevel.
func NormalizeLevel(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "read_only", "readonly", "read-only", "ro", "只读":
		return LevelReadOnly
	case "workspace_write", "workspace-write", "workspace", "write", "工作区写入":
		return LevelWorkspaceWrite
	case "safe_scan", "safe-scan", "scan", "扫描", "安全扫描":
		return LevelSafeScan
	case "approval_ops", "approval-ops", "approval", "approvals", "approve", "审批":
		return LevelApprovalOps
	case "", "advanced", "full", "高权限", "高级":
		return LevelAdvanced
	default:
		return LevelAdvanced
	}
}

func levelRank(level string) int {
	switch NormalizeLevel(level) {
	case LevelReadOnly:
		return 0
	case LevelWorkspaceWrite:
		return 1
	case LevelSafeScan:
		return 2
	case LevelApprovalOps:
		return 3
	case LevelAdvanced:
		return 4
	default:
		return 0
	}
}

// LevelAtLeast reports whether level satisfies the required level.
func LevelAtLeast(level, required string) bool {
	return levelRank(level) >= levelRank(required)
}
