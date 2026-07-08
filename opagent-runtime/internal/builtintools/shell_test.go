package builtintools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
)

type shellTestEvent struct {
	meta op.Meta
	text string
}

type shellTestNotifier struct {
	mu     sync.Mutex
	events []shellTestEvent
	hook   func(shellTestEvent)
}

func (n *shellTestNotifier) Notify(_ context.Context, params *op.InfoNotificationParams) {
	if n == nil || params == nil {
		return
	}
	event := shellTestEvent{
		meta: params.Meta.Clone(),
		text: shellTestContentText(params.Content),
	}

	n.mu.Lock()
	n.events = append(n.events, event)
	hook := n.hook
	n.mu.Unlock()

	if hook != nil {
		hook(event)
	}
}

func (n *shellTestNotifier) textsByType(typ string) []string {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()

	out := make([]string, 0)
	for _, event := range n.events {
		gotType, _ := event.meta["type"].(string)
		if gotType == typ {
			out = append(out, event.text)
		}
	}
	return out
}

func shellTestContentText(content op.Content) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case *op.TextContent:
		return typed.Text
	case *op.JsonContent:
		return string(typed.Raw)
	default:
		return ""
	}
}

func runShellTestCommand(
	t *testing.T,
	ctx context.Context,
	command string,
	notifier *shellTestNotifier,
) (string, error) {
	t.Helper()

	workdir := t.TempDir()
	baseMeta := op.Meta{"cwd": workdir}
	var notify shellNotifyFunc
	if notifier != nil {
		notify = notifier.Notify
	}
	output, _, err := execCommandStreamWithNotify(
		ctx,
		shellInput{Command: command},
		workdir,
		baseMeta,
		notify,
		false,
		true,
	)
	return output, err
}

func extractFullOutputPath(output string) string {
	const marker = "Full output saved to: "
	idx := strings.LastIndex(output, marker)
	if idx < 0 {
		return ""
	}
	rest := output[idx+len(marker):]
	end := strings.Index(rest, "]")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

func TestShellLongLineDoesNotFail(t *testing.T) {
	output, err := runShellTestCommand(
		t,
		context.Background(),
		`head -c 1100000 /dev/zero | tr '\0' 'a'`,
		nil,
	)
	if err != nil {
		t.Fatalf("runShellTestCommand() error = %v", err)
	}
	if strings.Contains(output, "token too long") {
		t.Fatalf("expected scanner failure to be gone, got %q", output)
	}
	if !strings.Contains(output, "[Output truncated.") {
		t.Fatalf("expected truncated output notice, got %q", output)
	}
	path := extractFullOutputPath(output)
	if path == "" {
		t.Fatalf("expected full output path in %q", output)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatalf("stat full output path: %v", statErr)
	}
	if filepath.Clean(filepath.Dir(path)) != filepath.Clean(os.TempDir()) {
		t.Fatalf("expected full output path in system temp dir, got %q (temp=%q)", path, os.TempDir())
	}
	if info.Size() < 1100000 {
		t.Fatalf("expected full output file to contain full line, got size %d", info.Size())
	}
}

func TestShellOutputTailTruncatesByBytes(t *testing.T) {
	output, err := runShellTestCommand(
		t,
		context.Background(),
		`i=1; while [ "$i" -le 700 ]; do printf 'chunk-%04d-%080d\n' "$i" 0; i=$((i+1)); done; printf 'tail-sentinel\n'`,
		nil,
	)
	if err != nil {
		t.Fatalf("runShellTestCommand() error = %v", err)
	}
	if strings.Contains(output, "chunk-0001-") {
		t.Fatalf("expected head lines to be trimmed from final output, got %q", output)
	}
	if !strings.Contains(output, "tail-sentinel") {
		t.Fatalf("expected tail lines to remain in final output, got %q", output)
	}
	if extractFullOutputPath(output) == "" {
		t.Fatalf("expected spill file path in %q", output)
	}
}

func TestShellOutputTailTruncatesByLines(t *testing.T) {
	output, err := runShellTestCommand(
		t,
		context.Background(),
		`i=1; while [ "$i" -le 2105 ]; do printf 'line-%04d\n' "$i"; i=$((i+1)); done`,
		nil,
	)
	if err != nil {
		t.Fatalf("runShellTestCommand() error = %v", err)
	}
	if strings.Contains(output, "line-0001\n") || strings.HasPrefix(output, "line-0001") {
		t.Fatalf("expected oldest lines to be omitted, got %q", output)
	}
	if !strings.Contains(output, "line-2105") {
		t.Fatalf("expected latest line in final output, got %q", output)
	}
	if !strings.Contains(output, "Showing lines 106-2105 of 2105.") {
		t.Fatalf("expected tail line notice, got %q", output)
	}
}

func TestShellUltraLongSingleLineReturnsTailSlice(t *testing.T) {
	output, err := runShellTestCommand(
		t,
		context.Background(),
		`head -c 70000 /dev/zero | tr '\0' 'x'`,
		nil,
	)
	if err != nil {
		t.Fatalf("runShellTestCommand() error = %v", err)
	}
	if strings.Contains(output, "(no output)") {
		t.Fatalf("expected retained tail content for a long single line, got %q", output)
	}
	if !strings.Contains(output, "Showing last") {
		t.Fatalf("expected partial-line truncation notice, got %q", output)
	}
}

func TestShellFlushesUnterminatedFinalLine(t *testing.T) {
	notifier := &shellTestNotifier{}
	output, err := runShellTestCommand(t, context.Background(), `printf 'hello'`, notifier)
	if err != nil {
		t.Fatalf("runShellTestCommand() error = %v", err)
	}
	if output != "hello" {
		t.Fatalf("expected final line to flush without newline, got %q", output)
	}
	streams := notifier.textsByType("toolOutputStream")
	if len(streams) != 1 || streams[0] != "hello" {
		t.Fatalf("expected a single flushed live line, got %#v", streams)
	}
}

func TestShellErrorsRetainCapturedOutput(t *testing.T) {
	t.Run("exit", func(t *testing.T) {
		_, err := runShellTestCommand(
			t,
			context.Background(),
			`i=1; while [ "$i" -le 2105 ]; do printf 'line-%04d\n' "$i"; i=$((i+1)); done; exit 7`,
			nil,
		)
		if err == nil {
			t.Fatal("expected non-zero exit error")
		}
		if !strings.Contains(err.Error(), "line-2105") {
			t.Fatalf("expected exit error to include tail output, got %v", err)
		}
		if strings.Contains(err.Error(), "line-0001\n") || strings.Contains(err.Error(), "\nline-0001\n") {
			t.Fatalf("expected exit error to omit trimmed head lines, got %v", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		_, err := runShellTestCommand(
			t,
			ctx,
			`printf 'before\n'; sleep 5`,
			nil,
		)
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if !strings.Contains(err.Error(), "command timed out") {
			t.Fatalf("expected timeout error, got %v", err)
		}
		if !strings.Contains(err.Error(), "before") {
			t.Fatalf("expected timeout error to include captured output, got %v", err)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		notifier := &shellTestNotifier{
			hook: func(event shellTestEvent) {
				gotType, _ := event.meta["type"].(string)
				if gotType == "toolOutputStream" && event.text == "before" {
					cancel()
				}
			},
		}
		defer cancel()

		_, err := runShellTestCommand(
			t,
			ctx,
			`printf 'before\n'; sleep 5`,
			notifier,
		)
		if err == nil {
			t.Fatal("expected cancellation error")
		}
		if !strings.Contains(err.Error(), "command cancelled") {
			t.Fatalf("expected cancellation error, got %v", err)
		}
		if !strings.Contains(err.Error(), "before") {
			t.Fatalf("expected cancellation error to include captured output, got %v", err)
		}
	})
}

func TestShellLiveOutputStopsAfterSingleTruncationNotice(t *testing.T) {
	notifier := &shellTestNotifier{}
	_, err := runShellTestCommand(
		t,
		context.Background(),
		`i=1; while [ "$i" -le 2105 ]; do printf 'x\n'; i=$((i+1)); done`,
		notifier,
	)
	if err != nil {
		t.Fatalf("runShellTestCommand() error = %v", err)
	}

	streams := notifier.textsByType("toolOutputStream")
	if len(streams) != 2001 {
		t.Fatalf("expected 2000 live lines plus one truncation notice, got %d", len(streams))
	}
	if streams[0] != "x" {
		t.Fatalf("expected live output to start from first line, got %q", streams[0])
	}
	if streams[1999] != "x" {
		t.Fatalf("expected live output to stop at line 2000, got %q", streams[1999])
	}
	if streams[2000] != shellLiveOutputTruncatedNotice {
		t.Fatalf("expected a single truncation notice, got %q", streams[2000])
	}
}
