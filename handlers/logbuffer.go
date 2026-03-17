package handlers

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const logBufferSize = 500

// LogLine is one captured log entry.
type LogLine struct {
	Time    time.Time
	Raw     string // full line as written by log package
	Message string // stripped of timestamp prefix
	Kind    string // "step", "done", "error", "info", "progress"
	Label   string // human-friendly prefix label
}

// LogBuffer is a fixed-size ring buffer that captures log output.
type LogBuffer struct {
	mu    sync.Mutex
	lines []LogLine
	pos   int      // next write position
	count int      // total lines written (capped at logBufferSize)
	w     io.Writer // underlying writer (os.Stderr + log file)
}

var globalLogBuffer = &LogBuffer{
	lines: make([]LogLine, logBufferSize),
	w:     os.Stderr,
}

// InitLogCapture replaces the default logger's output with a tee to the buffer
// and a persistent log file at dataDir/nisaba.log. On startup the log file is
// read to seed the in-memory buffer so previous-session logs are visible.
func InitLogCapture(dataDir string) {
	writers := []io.Writer{os.Stderr, globalLogBuffer}

	logPath := filepath.Join(dataDir, "nisaba.log")
	if f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		writers = append(writers, f)
		// Seed the in-memory buffer from the existing log file so logs from
		// previous server sessions are immediately visible on the /logs page.
		seedBufferFromFile(logPath)
	} else {
		log.Printf("warn: could not open log file %s: %v", logPath, err)
	}

	log.SetOutput(io.MultiWriter(writers...))
}

// seedBufferFromFile reads the last logBufferSize lines from path into the
// in-memory buffer. Called once at startup before the log writer is replaced,
// so we read directly rather than going through the log package.
func seedBufferFromFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) == 0 {
		return
	}
	start := 0
	if len(lines) > logBufferSize {
		start = len(lines) - logBufferSize
	}
	for _, raw := range lines[start:] {
		parsed := parseLogLine(raw)
		globalLogBuffer.mu.Lock()
		globalLogBuffer.lines[globalLogBuffer.pos] = parsed
		globalLogBuffer.pos = (globalLogBuffer.pos + 1) % logBufferSize
		if globalLogBuffer.count < logBufferSize {
			globalLogBuffer.count++
		}
		globalLogBuffer.mu.Unlock()
	}
}

// Write implements io.Writer. Called by the log package for every line.
func (b *LogBuffer) Write(p []byte) (n int, err error) {
	line := strings.TrimRight(string(p), "\n")
	parsed := parseLogLine(line)
	b.mu.Lock()
	b.lines[b.pos] = parsed
	b.pos = (b.pos + 1) % logBufferSize
	if b.count < logBufferSize {
		b.count++
	}
	b.mu.Unlock()
	return len(p), nil
}

// Lines returns captured lines in chronological order (oldest first).
func (b *LogBuffer) Lines() []LogLine {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.count == 0 {
		return nil
	}
	out := make([]LogLine, b.count)
	if b.count < logBufferSize {
		copy(out, b.lines[:b.count])
	} else {
		// Ring is full: oldest entry is at b.pos
		n := copy(out, b.lines[b.pos:])
		copy(out[n:], b.lines[:b.pos])
	}
	return out
}

// parseLogLine splits a Go log line (e.g. "2006/01/02 15:04:05 message") into parts.
func parseLogLine(raw string) LogLine {
	ll := LogLine{Raw: raw, Time: time.Now(), Kind: "info"}

	// Go's default log format: "2006/01/02 15:04:05 message"
	msg := raw
	if len(raw) >= 20 && raw[4] == '/' && raw[7] == '/' && raw[10] == ' ' && raw[13] == ':' && raw[16] == ':' {
		if t, err := time.ParseInLocation("2006/01/02 15:04:05", raw[:19], time.Local); err == nil {
			ll.Time = t
		}
		if len(raw) > 20 {
			msg = raw[20:]
		}
	}
	ll.Message = msg
	ll.Label, ll.Kind = classifyMessage(msg)
	return ll
}

// classifyMessage maps known log message prefixes to a human label and kind.
func classifyMessage(msg string) (label, kind string) {
	type rule struct {
		prefix string
		label  string
		kind   string
	}
	rules := []rule{
		// sync-all steps
		{"sync-all: step — Steam ownership", "Starting Steam ownership sync", "step"},
		{"sync-all: step — Steam wishlist", "Starting Steam wishlist sync", "step"},
		{"sync-all: step — GOG wishlist", "Starting GOG wishlist sync", "step"},
		{"sync-all: step — GG.deals pricing", "Starting GG.deals pricing sync", "step"},
		{"sync-all: step — reseller pricing", "Starting reseller pricing sync", "step"},
		{"sync-all: step — IGDB enrichment", "Starting IGDB enrichment", "step"},
		// sync-all completions
		{"sync-all: ownership done", "Steam ownership complete", "done"},
		{"sync-all: Steam wishlist done", "Steam wishlist complete", "done"},
		{"sync-all: GOG wishlist done", "GOG wishlist complete", "done"},
		{"sync-all: GOG wishlist skipped", "GOG wishlist skipped", "info"},
		{"sync-all: linked", "Linked wishlist entries to library", "info"},
		{"sync-all: GG.deals done", "GG.deals pricing complete", "done"},
		{"sync-all: resellers done", "Reseller pricing complete", "done"},
		{"sync-all: enrichment progress", "Enrichment progress", "progress"},
		{"sync all done", "Full sync complete", "done"},
		// server lifecycle
		{"NISABA running on", "Server started", "done"},
		// individual syncs — completions
		{"pricing done", "Pricing sync complete", "done"},
		{"wishlist done", "Wishlist sync complete", "done"},
		{"enrichment done", "Enrichment complete", "done"},
		{"wishlist enrichment done", "Wishlist enrichment complete", "done"},
		{"deck sync done", "Steam Deck status sync complete", "done"},
		{"proton sync done", "ProtonDB sync complete", "done"},
		{"install sync:", "Install state synced", "done"},
		// individual syncs — progress/info
		{"wishlist sync: linked", "Wishlist linked entries to library", "info"},
		{"gog wishlist sync: linked", "GOG wishlist linked entries to library", "info"},
		// individual syncs — errors
		{"steam sync:", "Steam ownership error", "error"},
		{"sync install:", "Install sync error", "error"},
		{"wishlist sync:", "Wishlist sync error", "error"},
		{"gog wishlist sync:", "GOG wishlist sync error", "error"},
		{"heroic upload", "Heroic upload error", "error"},
		{"heroic import", "Heroic import error", "error"},
		{"reseller pricing:", "Reseller pricing error", "error"},
		// sync-all errors
		{"sync-all ownership", "Ownership error", "error"},
		{"sync-all wishlist", "Wishlist error", "error"},
		{"sync-all gog wishlist", "GOG wishlist error", "error"},
		{"sync-all pricing", "Pricing error", "error"},
		{"sync-all reseller", "Reseller error", "error"},
		{"ggdeals pricing", "GG.deals error", "error"},
		{"reseller pricing fatal", "Reseller fatal error", "error"},
		{"protondb ", "ProtonDB error", "error"},
		{"persist pricing errors", "Error saving pricing log", "error"},
		{"persist proton errors", "Error saving ProtonDB log", "error"},
	}
	for _, r := range rules {
		if strings.HasPrefix(msg, r.prefix) {
			detail := strings.TrimPrefix(msg, r.prefix)
			detail = strings.TrimPrefix(detail, " — ")
			detail = strings.TrimPrefix(detail, ": ")
			if detail != "" {
				return fmt.Sprintf("%s — %s", r.label, detail), r.kind
			}
			return r.label, r.kind
		}
	}
	return msg, "info"
}

// kindIcon returns an inline indicator for the log line kind.
func kindIcon(kind string) string {
	switch kind {
	case "step":
		return "▶"
	case "done":
		return "✓"
	case "error":
		return "✗"
	case "progress":
		return "·"
	default:
		return "·"
	}
}

// ConsoleLogHTML renders the console log lines as an HTML fragment.
// mode is "human" or "raw".
func ConsoleLogHTML(lines []LogLine, mode string) string {
	if len(lines) == 0 {
		return `<div class="text-gray-600 text-xs p-4">No log output captured yet.</div>`
	}
	var b bytes.Buffer
	// Show most recent first
	for i := len(lines) - 1; i >= 0; i-- {
		ll := lines[i]
		ts := ll.Time.Format("Jan 2, 15:04:05")
		if mode == "raw" {
			fmt.Fprintf(&b,
				`<div class="font-mono text-xs py-0.5 text-gray-400 whitespace-pre-wrap break-all">%s</div>`,
				htmlEscape(ll.Raw))
		} else {
			colorClass := kindColorClass(ll.Kind)
			icon := kindIcon(ll.Kind)
			fmt.Fprintf(&b,
				`<div class="flex items-baseline gap-2 py-0.5 border-b border-gray-800/50 last:border-0">`+
					`<span class="text-gray-600 text-xs shrink-0 tabular-nums w-32">%s</span>`+
					`<span class="%s shrink-0 text-xs w-3">%s</span>`+
					`<span class="text-xs %s break-all">%s</span>`+
					`</div>`,
				ts, colorClass, icon, colorClass, htmlEscape(ll.Label))
		}
	}
	return b.String()
}

func kindColorClass(kind string) string {
	switch kind {
	case "step":
		return "text-amber-400"
	case "done":
		return "text-green-400"
	case "error":
		return "text-red-400"
	case "progress":
		return "text-gray-500"
	default:
		return "text-gray-500"
	}
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&#34;")
	return s
}
