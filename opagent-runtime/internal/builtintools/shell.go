package builtintools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

type shellInput struct {
	Command        string `json:"command" jsonschema:"Shell command/script to execute in non-interactive mode"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty" jsonschema:"Timeout in seconds (0 uses default, negative disables timeout)"`
}

const (
	defaultShellTimeoutSeconds = 60
	maxOutputLines             = 2000
	maxOutputBytes             = 50 * 1024
	outputStoreFilePattern     = "opagent-shell-output-*.log"
	shellReadChunkSize         = 4 * 1024
)

const shellLiveOutputTruncatedNotice = "[Output truncated. Further live shell output suppressed.]"

type shellNotifyFunc = NotifyFunc

type liveOutputLimiter struct {
	maxLines  int
	maxBytes  int
	lines     int
	bytes     int
	truncated bool
}

type shellLineAssembler struct {
	emitLine func(string) error
	pending  bytes.Buffer
}

func HandleShell(ctx context.Context, req *op.CallToolRequest, input shellInput) (*op.CallToolResult, any, error) {
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		return nil, nil, fmt.Errorf("command is required")
	}

	timeoutSeconds := input.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = defaultShellTimeoutSeconds
	}

	var execCtx context.Context
	var cancel context.CancelFunc
	if timeoutSeconds > 0 {
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	} else {
		execCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	workdir, err := resolveWorkdir(req)
	if err != nil {
		return nil, nil, err
	}

	output, meta, err := execCommandStream(execCtx, req, input, workdir, true, true)
	if err != nil {
		return nil, nil, err
	}
	return &op.CallToolResult{
		Meta: meta,
		Content: []op.Content{
			&op.TextContent{Text: output},
		},
	}, nil, nil
}

func executeShell(ctx context.Context, meta op.Meta, notifier shellNotifyFunc, input shellInput) (*op.CallToolResult, error) {
	input.Command = strings.TrimSpace(input.Command)
	if input.Command == "" {
		return nil, fmt.Errorf("command is required")
	}

	timeoutSeconds := input.TimeoutSeconds
	if timeoutSeconds == 0 {
		timeoutSeconds = defaultShellTimeoutSeconds
	}

	var execCtx context.Context
	var cancel context.CancelFunc
	if timeoutSeconds > 0 {
		execCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	} else {
		execCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	workdir, err := resolveWorkdirFromMeta(meta)
	if err != nil {
		return nil, err
	}

	output, resultMeta, err := execCommandStreamWithNotify(execCtx, input, workdir, meta, notifier, true, true)
	if err != nil {
		return nil, err
	}
	return &op.CallToolResult{
		Meta: resultMeta,
		Content: []op.Content{
			&op.TextContent{Text: output},
		},
	}, nil
}

func execCommandStream(execCtx context.Context, req *op.CallToolRequest, input shellInput, workdir string, keyResult bool, notify bool) (string, op.Meta, error) {
	return execCommandStreamWithNotify(
		execCtx,
		input,
		workdir,
		requestMeta(req),
		requestShellNotifier(req),
		keyResult,
		notify,
	)
}

func execCommandStreamWithNotify(
	execCtx context.Context,
	input shellInput,
	workdir string,
	baseMeta op.Meta,
	notifier shellNotifyFunc,
	keyResult bool,
	notify bool,
) (string, op.Meta, error) {
	invocation := resolveShellInvocation(input.Command)
	cmd := exec.CommandContext(execCtx, invocation.Executable, invocation.Args...)
	cmd.Dir = workdir
	prepareShellCommand(cmd)
	cmd.Env = common.WithPrependedPath(nil, common.OpagentBinDir())

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, fmt.Errorf("stderr pipe: %w", err)
	}

	collector, err := newOutputCollector()
	if err != nil {
		return "", nil, err
	}
	defer collector.Close()

	liveLimiter := newLiveOutputLimiter(maxOutputLines, maxOutputBytes)
	var collectMu sync.Mutex
	appendLine := func(prefix string, line string) error {
		cleaned := cleanANSI(line)
		if cleaned == "" {
			return nil
		}

		var emitLine bool
		var emitNotice bool
		collectMu.Lock()
		appendErr := collector.AppendLine(cleaned)
		if appendErr == nil {
			emitLine, emitNotice = liveLimiter.record(cleaned)
		}
		collectMu.Unlock()
		if appendErr != nil {
			return appendErr
		}
		if emitLine {
			meta := baseMeta.Clone()
			meta["type"] = "toolOutputStream"
			meta["stream"] = prefix
			notifyShellInfo(execCtx, notifier, meta, &op.TextContent{Text: cleaned})
			return nil
		}
		if emitNotice {
			meta := baseMeta.Clone()
			meta["type"] = "toolOutputStream"
			meta["stream"] = prefix
			notifyShellInfo(execCtx, notifier, meta, &op.TextContent{Text: shellLiveOutputTruncatedNotice})
		}
		return nil
	}

	if err = cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("start command: %w", err)
	}
	defer killProcessTree(cmd)

	done := make(chan struct{})
	go func() {
		select {
		case <-execCtx.Done():
			killProcessTree(cmd)
		case <-done:
		}
	}()
	defer close(done)

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	go readShellPipe(stdoutPipe, "stdout", appendLine, errCh, &wg)
	go readShellPipe(stderrPipe, "stderr", appendLine, errCh, &wg)

	wg.Wait()
	waitErr := cmd.Wait()
	close(errCh)

	for readErr := range errCh {
		if readErr != nil {
			return "", nil, collector.ExecError(readErr, input.Command, invocation.DisplayName, workdir)
		}
	}

	if execCtx.Err() != nil {
		return "", nil, collector.ExecError(execCtx.Err(), input.Command, invocation.DisplayName, workdir)
	}
	if waitErr != nil {
		return "", nil, collector.ExecError(waitErr, input.Command, invocation.DisplayName, workdir)
	}

	finalOutput := collector.FinalOutput()
	rawData, err := op.NewJsonContent(map[string]any{
		"command":          input.Command,
		"shell":            invocation.DisplayName,
		"workdir":          workdir,
		"output":           finalOutput,
		"truncated":        collector.Truncated(),
		"full_output_path": collector.FullOutputPath(),
	})
	if err != nil {
		return "", nil, fmt.Errorf("failed to create json content: %v", err)
	}

	meta := baseMeta.Clone()
	if keyResult {
		notifyShellInfo(execCtx, notifier, meta.Add(op.Meta{"type": "keyResult"}), rawData)
	}

	if !notify {
		meta = meta.Add(op.Meta{"type": "ignore"})
	} else {
		meta = meta.Add(op.Meta{"type": "toolOutput"})
	}
	return finalOutput, meta, nil
}

func requestShellNotifier(req *op.CallToolRequest) shellNotifyFunc {
	if req == nil || req.Session == nil {
		return nil
	}
	return func(ctx context.Context, params *op.InfoNotificationParams) {
		_ = req.Session.NotifyInfo(ctx, params)
	}
}

func notifyShellInfo(ctx context.Context, notifier shellNotifyFunc, meta op.Meta, content op.Content) {
	if notifier == nil {
		return
	}
	notifier(ctx, &op.InfoNotificationParams{
		Meta:    meta,
		Content: content,
	})
}

func newLiveOutputLimiter(maxLines int, maxBytes int) *liveOutputLimiter {
	return &liveOutputLimiter{
		maxLines: maxLines,
		maxBytes: maxBytes,
	}
}

func (l *liveOutputLimiter) record(line string) (bool, bool) {
	if l == nil {
		return true, false
	}
	if l.truncated {
		return false, false
	}

	nextBytes := len([]byte(line))
	if l.lines > 0 {
		nextBytes++
	}
	if l.lines+1 > l.maxLines || l.bytes+nextBytes > l.maxBytes {
		l.truncated = true
		return false, true
	}

	l.lines++
	l.bytes += nextBytes
	return true, false
}

func newShellLineAssembler(emitLine func(string) error) *shellLineAssembler {
	return &shellLineAssembler{emitLine: emitLine}
}

func (a *shellLineAssembler) Append(chunk []byte) error {
	for len(chunk) > 0 {
		idx := bytes.IndexByte(chunk, '\n')
		if idx < 0 {
			_, _ = a.pending.Write(chunk)
			return nil
		}

		_, _ = a.pending.Write(chunk[:idx])
		if err := a.emitLine(string(a.pending.Bytes())); err != nil {
			return err
		}
		a.pending.Reset()
		chunk = chunk[idx+1:]
	}
	return nil
}

func (a *shellLineAssembler) Flush() error {
	if a.pending.Len() == 0 {
		return nil
	}
	if err := a.emitLine(string(a.pending.Bytes())); err != nil {
		return err
	}
	a.pending.Reset()
	return nil
}

func readShellPipe(
	reader io.ReadCloser,
	stream string,
	appendLine func(string, string) error,
	errCh chan<- error,
	wg *sync.WaitGroup,
) {
	defer wg.Done()
	defer func() {
		_ = reader.Close()
	}()

	assembler := newShellLineAssembler(func(line string) error {
		return appendLine(stream, line)
	})
	buf := make([]byte, shellReadChunkSize)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if appendErr := assembler.Append(buf[:n]); appendErr != nil {
				errCh <- appendErr
				return
			}
		}
		if errors.Is(err, io.EOF) {
			if flushErr := assembler.Flush(); flushErr != nil {
				errCh <- flushErr
			}
			return
		}
		if err != nil {
			if !errors.Is(err, os.ErrClosed) && !strings.Contains(err.Error(), "file already closed") {
				errCh <- err
			}
			return
		}
	}
}

func resolveWorkdir(req *op.CallToolRequest) (string, error) {
	if req == nil {
		return resolveWorkdirFromMeta(nil)
	}
	return resolveWorkdirFromMeta(req.Params.Meta)
}

func resolveWorkdirFromMeta(meta op.Meta) (string, error) {
	fallback, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get current working directory: %w", err)
	}
	if meta == nil {
		return fallback, nil
	}
	raw, ok := meta["cwd"]
	if !ok || raw == nil {
		return fallback, nil
	}
	cwd := strings.TrimSpace(fmt.Sprint(raw))
	if cwd == "" {
		return fallback, nil
	}
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(fallback, cwd)
	}
	cwd = filepath.Clean(cwd)
	info, statErr := os.Stat(cwd)
	if statErr != nil {
		return "", fmt.Errorf("invalid cwd %q: %w", cwd, statErr)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cwd is not a directory: %s", cwd)
	}
	return cwd, nil
}

type outputCollector struct {
	totalLines           int
	totalBytes           int
	tailLines            []string
	tailBytes            int
	tailFirstLinePartial bool
	fullBuilder          strings.Builder
	fullHasContent       bool
	fullFile             *os.File
	fullOutputPath       string
}

func newOutputCollector() (*outputCollector, error) {
	return &outputCollector{}, nil
}

func (c *outputCollector) ensureFullOutputFile() error {
	if c.fullFile != nil {
		return nil
	}
	f, err := os.CreateTemp("", outputStoreFilePattern)
	if err != nil {
		return fmt.Errorf("create full output file: %w", err)
	}
	c.fullFile = f
	c.fullOutputPath = f.Name()
	if c.fullBuilder.Len() > 0 {
		if _, err = c.fullFile.WriteString(c.fullBuilder.String()); err != nil {
			return fmt.Errorf("write buffered output: %w", err)
		}
		c.fullBuilder.Reset()
	}
	return nil
}

func (c *outputCollector) AppendLine(line string) error {
	lineBytes := len([]byte(line))
	if c.totalLines > 0 {
		c.totalBytes++
	}
	c.totalLines++
	c.totalBytes += lineBytes

	c.appendTailLine(line)
	c.appendBufferedLine(line)
	if c.fullFile == nil && c.Truncated() {
		if err := c.ensureFullOutputFile(); err != nil {
			return err
		}
	}
	return nil
}

func (c *outputCollector) appendTailLine(line string) {
	lineBytes := len([]byte(line))
	if len(c.tailLines) == 0 {
		c.tailLines = []string{line}
		c.tailBytes = lineBytes
		if lineBytes > maxOutputBytes {
			truncated := truncateStringToBytesFromEnd(line, maxOutputBytes)
			c.tailLines[0] = truncated
			c.tailBytes = len([]byte(truncated))
			c.tailFirstLinePartial = true
		}
		return
	}

	c.tailLines = append(c.tailLines, line)
	c.tailBytes += 1 + lineBytes
	for len(c.tailLines) > maxOutputLines || c.tailBytes > maxOutputBytes {
		if len(c.tailLines) == 1 {
			truncated := truncateStringToBytesFromEnd(c.tailLines[0], maxOutputBytes)
			c.tailFirstLinePartial = truncated != c.tailLines[0]
			c.tailLines[0] = truncated
			c.tailBytes = len([]byte(truncated))
			break
		}
		removed := c.tailLines[0]
		c.tailLines = c.tailLines[1:]
		c.tailBytes -= len([]byte(removed)) + 1
		c.tailFirstLinePartial = false
	}
}

func (c *outputCollector) appendBufferedLine(line string) {
	if c.fullHasContent {
		if c.fullFile != nil {
			_, _ = c.fullFile.WriteString("\n")
		} else {
			c.fullBuilder.WriteString("\n")
		}
	}
	if c.fullFile != nil {
		_, _ = c.fullFile.WriteString(line)
	} else {
		c.fullBuilder.WriteString(line)
	}
	c.fullHasContent = true
}

func (c *outputCollector) Truncated() bool {
	return c.totalLines > maxOutputLines || c.totalBytes > maxOutputBytes
}

func (c *outputCollector) FullOutputPath() string {
	return c.fullOutputPath
}

func (c *outputCollector) FinalOutput() string {
	if !c.Truncated() {
		return c.fullBuilder.String()
	}
	if c.fullFile == nil {
		_ = c.ensureFullOutputFile()
	}

	out := strings.Join(c.tailLines, "\n")
	if out == "" {
		out = "(no output)"
	}

	notice := "\n[Output truncated."
	if c.tailFirstLinePartial && len(c.tailLines) == 1 {
		notice += " Showing last " + formatSize(c.tailBytes) + " of line " + strconv.Itoa(c.totalLines) + " (line is " + formatSize(c.totalBytes) + ")."
	} else if len(c.tailLines) > 0 {
		startLine := c.totalLines - len(c.tailLines) + 1
		if startLine < 1 {
			startLine = 1
		}
		notice += " Showing lines " + strconv.Itoa(startLine) + "-" + strconv.Itoa(c.totalLines) + " of " + strconv.Itoa(c.totalLines) + "."
	}
	if c.fullOutputPath != "" {
		notice += " Full output saved to: " + c.fullOutputPath + "]"
	} else {
		notice += "]"
	}
	return out + notice
}

func (c *outputCollector) ExecError(err error, command string, shell string, workdir string) error {
	output := c.FinalOutput()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("command timed out: %s (shell=%s, workdir=%s)\n%s", command, shell, workdir, output)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("command cancelled: %s (shell=%s, workdir=%s)\n%s", command, shell, workdir, output)
	default:
		if output != "" {
			return fmt.Errorf("command failed: %w (shell=%s, workdir=%s)\n%s", err, shell, workdir, output)
		}
		return fmt.Errorf("command failed: %w (shell=%s, workdir=%s)", err, shell, workdir)
	}
}

func (c *outputCollector) Close() {
	if c.fullFile != nil {
		_ = c.fullFile.Close()
	}
}
