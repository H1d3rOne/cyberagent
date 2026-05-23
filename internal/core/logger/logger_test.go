package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestInfoNormal(t *testing.T) {
	var buf bytes.Buffer
	l := New(LevelNormal, &buf)
	l.Info("hello %s", "world")
	if !strings.Contains(buf.String(), "hello world") {
		t.Errorf("expected info output, got %q", buf.String())
	}
}

func TestDebugHiddenAtNormal(t *testing.T) {
	var buf bytes.Buffer
	l := New(LevelNormal, &buf)
	l.Debug("secret")
	if buf.Len() != 0 {
		t.Errorf("expected no debug output at normal level, got %q", buf.String())
	}
}

func TestDebugShownAtVerbose(t *testing.T) {
	var buf bytes.Buffer
	l := New(LevelVerbose, &buf)
	l.Debug("detail")
	if !strings.Contains(buf.String(), "[debug] detail") {
		t.Errorf("expected debug output, got %q", buf.String())
	}
}

func TestQuietSuppressesInfo(t *testing.T) {
	var buf bytes.Buffer
	l := New(LevelQuiet, &buf)
	l.Info("should not appear")
	l.Warn("should not appear")
	if buf.Len() != 0 {
		t.Errorf("expected no output at quiet level, got %q", buf.String())
	}
}

func TestErrorAlwaysShown(t *testing.T) {
	var buf bytes.Buffer
	l := New(LevelQuiet, &buf)
	l.Error("critical")
	if !strings.Contains(buf.String(), "[error] critical") {
		t.Errorf("expected error output even at quiet level, got %q", buf.String())
	}
}

func TestGlobalSetLevel(t *testing.T) {
	SetLevel(LevelVerbose)
	if GetLevel() != LevelVerbose {
		t.Error("expected verbose level")
	}
	SetLevel(LevelNormal)
}
