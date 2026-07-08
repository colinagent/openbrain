package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

const (
	defaultPort            = "19530"
	defaultHealthAttempts  = 120
	defaultHealthInterval  = 500 * time.Millisecond
	downloadTimeout        = 2 * time.Minute
	latestVersionFile      = "latest.version"
	runningVersionFile     = "running.version"
	pidFileName            = "opagent-runtime.pid"
	bundleArchiveFileName  = "bundle.tar.gz"
	defaultRuntimeHost     = "127.0.0.1"
	defaultRuntimeHTTPPath = "/health"
)

type options struct {
	command      string
	baseDir      string
	port         string
	version      string
	bundleFile   string
	bundleURL    string
	bundleSHA256 string
	jsonEvents   bool
}

type event struct {
	Type             string `json:"type"`
	Phase            string `json:"phase"`
	OK               bool   `json:"ok"`
	Message          string `json:"message,omitempty"`
	Error            string `json:"error,omitempty"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	RunningVersion   string `json:"runningVersion,omitempty"`
	LatestVersion    string `json:"latestVersion,omitempty"`
	NeedsInstall     bool   `json:"needsInstall"`
	NeedsUpdate      bool   `json:"needsUpdate"`
	NeedsStart       bool   `json:"needsStart"`
	Healthy          bool   `json:"healthy"`
}

type state struct {
	InstalledVersion string
	RunningVersion   string
	LatestVersion    string
	NeedsInstall     bool
	NeedsUpdate      bool
	NeedsStart       bool
	Healthy          bool
}

type managedFile struct {
	BundleRelativePath string
	TargetRelativePath string
	Executable         bool
	Directory          bool
}

func main() {
	opts, err := parseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := run(context.Background(), opts); err != nil {
		emit(opts, "result", "error", false, "Runtime bootstrap failed", err.Error(), state{})
		os.Exit(1)
	}
}

func parseOptions(args []string) (options, error) {
	opts := options{port: defaultPort}
	if len(args) == 0 {
		return opts, errors.New("usage: opagent-bootstrap <status|ensure|start|stop> [options]")
	}
	opts.command = strings.TrimSpace(args[0])
	fs := flag.NewFlagSet("opagent-bootstrap "+opts.command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&opts.baseDir, "base-dir", "", "OpAgent base directory")
	fs.StringVar(&opts.port, "port", defaultPort, "runtime server port")
	fs.StringVar(&opts.version, "version", "", "target runtime version")
	fs.StringVar(&opts.bundleFile, "bundle-file", "", "local runtime bundle tar.gz")
	fs.StringVar(&opts.bundleURL, "bundle-url", "", "runtime bundle URL")
	fs.StringVar(&opts.bundleSHA256, "bundle-sha256", "", "runtime bundle sha256")
	fs.BoolVar(&opts.jsonEvents, "json-events", false, "emit JSON event lines")
	if err := fs.Parse(args[1:]); err != nil {
		return opts, err
	}
	switch opts.command {
	case "status", "ensure", "start", "stop":
	default:
		return opts, fmt.Errorf("unsupported command: %s", opts.command)
	}
	if strings.TrimSpace(opts.baseDir) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return opts, fmt.Errorf("resolve home dir: %w", err)
		}
		opts.baseDir = filepath.Join(home, ".openbrain")
	}
	opts.baseDir = filepath.Clean(opts.baseDir)
	return opts, nil
}

func run(ctx context.Context, opts options) error {
	switch opts.command {
	case "status":
		st := loadState(opts)
		phase := "ready"
		message := "Runtime is ready"
		if st.NeedsInstall {
			phase = "install"
			message = "Runtime needs installation"
		} else if st.NeedsStart {
			phase = "start"
			message = "Runtime needs start"
		}
		emit(opts, "result", phase, true, message, "", st)
		return nil
	case "stop":
		if err := stopRuntime(opts.baseDir); err != nil {
			return err
		}
		st := loadState(opts)
		emit(opts, "result", "stopped", true, "Runtime stopped", "", st)
		return nil
	case "start":
		st := loadState(opts)
		if err := stopRuntime(opts.baseDir); err != nil {
			return err
		}
		if err := startRuntime(opts); err != nil {
			return err
		}
		st = loadState(opts)
		emit(opts, "result", "ready", true, "Runtime is ready", "", st)
		return nil
	case "ensure":
		st := loadState(opts)
		if st.NeedsInstall {
			emit(opts, "event", "installing", true, "Installing runtime", "", st)
			if err := installRuntimeBundle(ctx, opts); err != nil {
				return err
			}
		}
		if err := stopRuntime(opts.baseDir); err != nil {
			return err
		}
		if err := startRuntime(opts); err != nil {
			return err
		}
		st = loadState(opts)
		emit(opts, "result", "ready", true, "Runtime is ready", "", st)
		return nil
	default:
		return fmt.Errorf("unsupported command: %s", opts.command)
	}
}

func loadState(opts options) state {
	installed := readText(filepath.Join(opts.baseDir, "run", latestVersionFile))
	running := readText(filepath.Join(opts.baseDir, "run", runningVersionFile))
	target := strings.TrimSpace(opts.version)
	if target == "" {
		target = installed
	}
	healthy := healthOK(opts.port)
	needsInstall := false
	needsUpdate := false
	if target != "" && installed != target && shouldUpdateRuntimeVersion(installed, target) {
		needsInstall = true
		needsUpdate = installed != ""
	}
	if target != "" && installed != "" && !shouldUpdateRuntimeVersion(installed, target) {
		target = installed
	}
	for _, spec := range managedFiles() {
		stat, err := os.Stat(filepath.Join(opts.baseDir, spec.TargetRelativePath))
		if err != nil {
			needsInstall = true
			break
		}
		if spec.Directory && !stat.IsDir() {
			needsInstall = true
			break
		}
		if !spec.Directory && stat.IsDir() {
			needsInstall = true
			break
		}
	}
	return state{
		InstalledVersion: installed,
		RunningVersion:   running,
		LatestVersion:    target,
		NeedsInstall:     needsInstall,
		NeedsUpdate:      needsUpdate,
		NeedsStart:       !healthy || (target != "" && running != target),
		Healthy:          healthy,
	}
}

func emit(opts options, typ string, phase string, ok bool, message string, errText string, st state) {
	payload := event{
		Type:             typ,
		Phase:            phase,
		OK:               ok,
		Message:          message,
		Error:            errText,
		InstalledVersion: st.InstalledVersion,
		RunningVersion:   st.RunningVersion,
		LatestVersion:    st.LatestVersion,
		NeedsInstall:     st.NeedsInstall,
		NeedsUpdate:      st.NeedsUpdate,
		NeedsStart:       st.NeedsStart,
		Healthy:          st.Healthy,
	}
	if opts.jsonEvents {
		raw, _ := json.Marshal(payload)
		fmt.Println(string(raw))
		return
	}
	if errText != "" {
		fmt.Fprintf(os.Stderr, "%s: %s\n", phase, errText)
		return
	}
	if message != "" {
		fmt.Println(message)
	}
}

func installRuntimeBundle(ctx context.Context, opts options) error {
	if strings.TrimSpace(opts.version) == "" {
		return errors.New("--version is required for ensure when installation is needed")
	}
	workDir, err := os.MkdirTemp("", "opagent-bootstrap-*")
	if err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	defer os.RemoveAll(workDir)
	archivePath := filepath.Join(workDir, bundleArchiveFileName)
	extractDir := filepath.Join(workDir, "extract")

	if strings.TrimSpace(opts.bundleFile) != "" {
		if err := copyFile(opts.bundleFile, archivePath, 0o644); err != nil {
			return fmt.Errorf("copy bundle file: %w", err)
		}
	} else {
		if strings.TrimSpace(opts.bundleURL) == "" || strings.TrimSpace(opts.bundleSHA256) == "" {
			return errors.New("--bundle-file or --bundle-url with --bundle-sha256 is required")
		}
		if err := downloadFile(ctx, opts.bundleURL, archivePath, opts.bundleSHA256); err != nil {
			return err
		}
	}
	if opts.bundleSHA256 != "" && opts.bundleFile != "" {
		if err := verifySHA256(archivePath, opts.bundleSHA256); err != nil {
			return err
		}
	}
	if err := extractTarGz(archivePath, extractDir); err != nil {
		return err
	}
	for _, spec := range managedFiles() {
		stat, err := os.Stat(filepath.Join(extractDir, spec.BundleRelativePath))
		if err != nil {
			return fmt.Errorf("bundle is missing %s", spec.BundleRelativePath)
		}
		if spec.Directory && !stat.IsDir() {
			return fmt.Errorf("bundle path is not a directory: %s", spec.BundleRelativePath)
		}
		if !spec.Directory && stat.IsDir() {
			return fmt.Errorf("bundle path is not a file: %s", spec.BundleRelativePath)
		}
	}
	if err := stopRuntime(opts.baseDir); err != nil {
		return err
	}
	currentExecutable, _ := os.Executable()
	for _, dir := range []string{"bin", "agents", "tools", "skills"} {
		if err := copyDir(filepath.Join(extractDir, dir), filepath.Join(opts.baseDir, dir), func(target string) bool {
			return samePath(currentExecutable, target)
		}); err != nil {
			return err
		}
	}
	if err := copyFile(filepath.Join(extractDir, "configs", "config.json"), filepath.Join(opts.baseDir, "configs", "config.json"), 0o644); err != nil {
		return err
	}
	if err := common.ProjectSystemToolBins(opts.baseDir); err != nil {
		return err
	}
	return writeText(filepath.Join(opts.baseDir, "run", latestVersionFile), opts.version)
}

func startRuntime(opts options) error {
	if err := os.MkdirAll(filepath.Join(opts.baseDir, "run"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(opts.baseDir, "logs", "opagent-runtime"), 0o755); err != nil {
		return err
	}
	runtimePath := filepath.Join(opts.baseDir, "bin", "opagent-runtime"+exeSuffix())
	if _, err := os.Stat(runtimePath); err != nil {
		return fmt.Errorf("runtime binary missing: %w", err)
	}
	logPath := filepath.Join(opts.baseDir, "logs", "opagent-runtime", "opagent-runtime.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open runtime log: %w", err)
	}
	defer logFile.Close()
	cmd := exec.Command(runtimePath, "--base-dir", opts.baseDir)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(), "OPENBRAIN_BASE_DIR="+opts.baseDir)
	prepareRuntimeCommand(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start runtime: %w", err)
	}
	if err := writeText(filepath.Join(opts.baseDir, "run", pidFileName), fmt.Sprintf("%d", cmd.Process.Pid)); err != nil {
		return err
	}
	if err := waitHealth(opts.port, defaultHealthAttempts, defaultHealthInterval); err != nil {
		return err
	}
	version := strings.TrimSpace(opts.version)
	if version == "" {
		version = readText(filepath.Join(opts.baseDir, "run", latestVersionFile))
	}
	if version != "" {
		return writeText(filepath.Join(opts.baseDir, "run", runningVersionFile), version)
	}
	return nil
}

func stopRuntime(baseDir string) error {
	pidPath := filepath.Join(baseDir, "run", pidFileName)
	raw := readText(pidPath)
	if raw != "" {
		if pid, err := parsePID(raw); err == nil {
			_ = terminatePID(pid)
		}
	}
	_ = os.Remove(pidPath)
	_ = os.Remove(filepath.Join(baseDir, "run", runningVersionFile))
	return nil
}

func parsePID(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("empty pid")
	}
	var pid int
	if _, err := fmt.Sscanf(raw, "%d", &pid); err != nil {
		return 0, err
	}
	if pid <= 0 {
		return 0, fmt.Errorf("invalid pid: %d", pid)
	}
	return pid, nil
}

func healthOK(port string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL(port), nil)
	if err != nil {
		return false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode/100 == 2
}

func waitHealth(port string, attempts int, interval time.Duration) error {
	for i := 0; i < attempts; i++ {
		if healthOK(port) {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("runtime health check failed at %s", healthURL(port))
}

func healthURL(port string) string {
	return "http://" + defaultRuntimeHost + ":" + strings.TrimSpace(port) + defaultRuntimeHTTPPath
}

func managedFiles() []managedFile {
	exe := exeSuffix()
	return []managedFile{
		{BundleRelativePath: filepath.Join("bin", "opagent-runtime"+exe), TargetRelativePath: filepath.Join("bin", "opagent-runtime"+exe), Executable: true},
		{BundleRelativePath: filepath.Join("bin", "opagent-bootstrap"+exe), TargetRelativePath: filepath.Join("bin", "opagent-bootstrap"+exe), Executable: true},
		{BundleRelativePath: filepath.Join("bin", "gbrain"+exe), TargetRelativePath: filepath.Join("bin", "gbrain"+exe), Executable: true},
		{BundleRelativePath: "agents", TargetRelativePath: "agents", Directory: true},
		{BundleRelativePath: "tools", TargetRelativePath: "tools", Directory: true},
		{BundleRelativePath: "skills", TargetRelativePath: "skills", Directory: true},
		{BundleRelativePath: filepath.Join("configs", "config.json"), TargetRelativePath: filepath.Join("configs", "config.json")},
	}
}

func exeSuffix() string {
	if goruntime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func shouldUpdateRuntimeVersion(currentVersion string, targetVersion string) bool {
	currentVersion = strings.TrimSpace(currentVersion)
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" || targetVersion == currentVersion {
		return false
	}
	if currentVersion == "" {
		return true
	}
	if cmp, ok := compareRuntimeSemver(targetVersion, currentVersion); ok {
		return cmp > 0
	}
	return true
}

type runtimeSemver struct {
	major int
	minor int
	patch int
	pre   []string
}

func compareRuntimeSemver(a string, b string) (int, bool) {
	av, ok := parseRuntimeSemver(a)
	if !ok {
		return 0, false
	}
	bv, ok := parseRuntimeSemver(b)
	if !ok {
		return 0, false
	}
	if av.major != bv.major {
		return compareInt(av.major, bv.major), true
	}
	if av.minor != bv.minor {
		return compareInt(av.minor, bv.minor), true
	}
	if av.patch != bv.patch {
		return compareInt(av.patch, bv.patch), true
	}
	return compareRuntimePrerelease(av.pre, bv.pre), true
}

func parseRuntimeSemver(raw string) (runtimeSemver, bool) {
	value := strings.TrimSpace(raw)
	if strings.HasPrefix(value, "v") || strings.HasPrefix(value, "V") {
		value = value[1:]
	}
	if value == "" {
		return runtimeSemver{}, false
	}
	if buildIdx := strings.IndexByte(value, '+'); buildIdx >= 0 {
		value = value[:buildIdx]
	}

	core := value
	var pre []string
	if preIdx := strings.IndexByte(value, '-'); preIdx >= 0 {
		core = value[:preIdx]
		preValue := value[preIdx+1:]
		if preValue == "" {
			return runtimeSemver{}, false
		}
		pre = strings.Split(preValue, ".")
		for _, part := range pre {
			if part == "" {
				return runtimeSemver{}, false
			}
		}
	}

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return runtimeSemver{}, false
	}
	major, ok := parseRuntimeVersionNumber(parts[0])
	if !ok {
		return runtimeSemver{}, false
	}
	minor, ok := parseRuntimeVersionNumber(parts[1])
	if !ok {
		return runtimeSemver{}, false
	}
	patch, ok := parseRuntimeVersionNumber(parts[2])
	if !ok {
		return runtimeSemver{}, false
	}
	return runtimeSemver{major: major, minor: minor, patch: patch, pre: pre}, true
}

func parseRuntimeVersionNumber(raw string) (int, bool) {
	if raw == "" {
		return 0, false
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	return value, true
}

func compareRuntimePrerelease(a []string, b []string) int {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	if len(a) == 0 {
		return 1
	}
	if len(b) == 0 {
		return -1
	}
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	for i := 0; i < limit; i++ {
		if a[i] == b[i] {
			continue
		}
		av, aNumeric := parseRuntimeVersionNumber(a[i])
		bv, bNumeric := parseRuntimeVersionNumber(b[i])
		switch {
		case aNumeric && bNumeric:
			return compareInt(av, bv)
		case aNumeric:
			return -1
		case bNumeric:
			return 1
		case a[i] < b[i]:
			return -1
		default:
			return 1
		}
	}
	return compareInt(len(a), len(b))
}

func compareInt(a int, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func readText(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func writeText(path string, value string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(value)+"\n"), 0o644)
}

func downloadFile(ctx context.Context, url string, destination string, expectedSHA string) error {
	requestCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, strings.TrimSpace(url), nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download bundle: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("download response %s: %s", res.Status, strings.TrimSpace(string(body)))
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), "download-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()
	hash := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, hash), res.Body); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(expectedSHA)) {
		return fmt.Errorf("sha256 mismatch: expected %s got %s", expectedSHA, actual)
	}
	return os.Rename(tmpPath, destination)
}

func verifySHA256(path string, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return err
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actual, strings.TrimSpace(expected)) {
		return fmt.Errorf("sha256 mismatch: expected %s got %s", expected, actual)
	}
	return nil
}

func extractTarGz(archivePath string, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return err
	}
	cleanDest := filepath.Clean(destination)
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(destination, header.Name)
		cleanTarget := filepath.Clean(target)
		if cleanTarget != cleanDest && !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) {
			return fmt.Errorf("archive path escapes destination: %s", header.Name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			mode := os.FileMode(header.Mode).Perm()
			if mode == 0 {
				mode = 0o644
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}

func copyDir(src string, dst string, skipTarget func(string) bool) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if skipTarget != nil && skipTarget(target) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode().Perm())
	})
}

func copyFile(src string, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if mode == 0 {
		mode = 0o644
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if goruntime.GOOS != "windows" {
		if err := os.Chmod(tmpPath, mode); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
	}
	return os.Rename(tmpPath, dst)
}

func samePath(a string, b string) bool {
	a = normalizePathForCompare(a)
	b = normalizePathForCompare(b)
	if a == "" || b == "" {
		return false
	}
	if goruntime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func normalizePathForCompare(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return filepath.Clean(path)
}
