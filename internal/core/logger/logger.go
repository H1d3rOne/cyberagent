package logger

import (
	"fmt"
	"io"
	"os"
	"sync"
)

type Level int

const (
	LevelQuiet Level = iota
	LevelNormal
	LevelVerbose
)

type Logger struct {
	mu    sync.Mutex
	level Level
	out   io.Writer
}

var std = &Logger{level: LevelNormal, out: os.Stderr}

func Std() *Logger          { return std }
func SetLevel(l Level)      { std.mu.Lock(); std.level = l; std.mu.Unlock() }
func GetLevel() Level       { std.mu.Lock(); defer std.mu.Unlock(); return std.level }
func SetOutput(w io.Writer) { std.mu.Lock(); std.out = w; std.mu.Unlock() }

func New(level Level, out io.Writer) *Logger {
	return &Logger{level: level, out: out}
}

func (l *Logger) Level() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

func (l *Logger) Info(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level >= LevelNormal {
		fmt.Fprintf(l.out, format+"\n", args...)
	}
}

func (l *Logger) Debug(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level >= LevelVerbose {
		fmt.Fprintf(l.out, "[debug] "+format+"\n", args...)
	}
}

func (l *Logger) Warn(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level >= LevelNormal {
		fmt.Fprintf(l.out, "[warn] "+format+"\n", args...)
	}
}

func (l *Logger) Error(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.out, "[error] "+format+"\n", args...)
}

func Info(format string, args ...any)  { std.Info(format, args...) }
func Debug(format string, args ...any) { std.Debug(format, args...) }
func Warn(format string, args ...any)  { std.Warn(format, args...) }
func Error(format string, args ...any) { std.Error(format, args...) }
