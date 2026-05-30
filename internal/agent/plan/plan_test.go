package plan

import "testing"

func TestCoverageAndNextPending(t *testing.T) {
	p := Plan{Goal: "scan", Todos: []Todo{
		{ID: "t1", Status: TodoDone},
		{ID: "t2", Status: TodoPending},
		{ID: "t3", Status: TodoSkipped},
		{ID: "t4", Status: TodoPending},
	}}
	resolved, total := p.Coverage()
	if resolved != 2 || total != 4 {
		t.Fatalf("coverage = %d/%d, want 2/4", resolved, total)
	}
	next := p.NextPending()
	if next == nil || next.ID != "t2" {
		t.Fatalf("NextPending = %v, want t2", next)
	}
	if !p.Mark("t2", TodoDone) {
		t.Fatal("Mark t2 failed")
	}
	if next := p.NextPending(); next == nil || next.ID != "t4" {
		t.Fatalf("after mark, NextPending = %v, want t4", next)
	}
	if p.Mark("missing", TodoDone) {
		t.Fatal("Mark missing should return false")
	}
}

func TestLevelAtLeast(t *testing.T) {
	cases := []struct {
		level, required string
		want            bool
	}{
		{"advanced", "safe_scan", true},
		{"read_only", "safe_scan", false},
		{"safe_scan", "safe_scan", true},
		{"approval_ops", "workspace_write", true},
		{"工作区写入", "read_only", true},
		{"扫描", "approval_ops", false},
		{"", "advanced", true}, // empty normalizes to advanced
	}
	for _, c := range cases {
		if got := LevelAtLeast(c.level, c.required); got != c.want {
			t.Errorf("LevelAtLeast(%q,%q)=%v want %v", c.level, c.required, got, c.want)
		}
	}
}
