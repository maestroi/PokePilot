package main

import (
	"bytes"
	"io"
	"strings"
)

// agentTraceLog tees planner/round log lines to w and into the watch trace.
// note is typically (*emu.Emu).TraceNote. Partial writes are buffered until
// a newline so a split Write still becomes one trace entry.
type agentTraceLog struct {
	w    io.Writer
	note func(kind, text string)
	buf  []byte
}

func (l *agentTraceLog) Write(p []byte) (int, error) {
	n, err := l.w.Write(p)
	if n <= 0 {
		return n, err
	}
	l.buf = append(l.buf, p[:n]...)
	for {
		i := bytes.IndexByte(l.buf, '\n')
		if i < 0 {
			break
		}
		line := string(bytes.TrimSpace(l.buf[:i]))
		l.buf = l.buf[i+1:]
		// Run already emits one controlled, structured line for every
		// objective outcome. Observe it here before the watch-only filter so
		// failure telemetry is collected even when TraceNote is disabled.
		observeAgentLogLine(line)
		if l.note != nil {
			if kind := agentTraceKind(line); kind != "" {
				l.note(kind, line)
			}
		}
	}
	return n, err
}

// agentTraceKind maps a log line to a watch-trace kind. Only llm and round
// lines are mirrored; other stdout chatter stays terminal-only.
func agentTraceKind(line string) string {
	switch {
	case strings.HasPrefix(line, "llm:"):
		return "llm"
	case strings.HasPrefix(line, "round "):
		return "round"
	default:
		return ""
	}
}
