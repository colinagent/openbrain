package run

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	"github.com/colinagent/openbrain/opagent-runtime/internal/config"
	"github.com/colinagent/openbrain/opagent-runtime/internal/core"
	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

const (
	runtimeUpdatePhaseDisabled    = "disabled"
	runtimeUpdatePhaseIdle        = "idle"
	runtimeUpdatePhaseChecking    = "checking"
	runtimeUpdatePhaseDownloading = "downloading"
	runtimeUpdatePhaseStaged      = "staged"
	runtimeUpdatePhaseApplying    = "applying"
	runtimeUpdatePhaseError       = "error"

	runtimeUpdateDownloadTimeout = 2 * time.Minute
	runtimeUpdateApplyPollPeriod = 5 * time.Second

	runtimeLatestVersionFile  = "latest.version"
	runtimeRunningVersionFile = "running.version"
	runtimeStageStateFile     = "state.json"
)

type runtimeUpdateSettings struct {
	enabled         bool
	baseDir         string
	manifestURL     string
	checkInterval   time.Duration
	checkTimeout    time.Duration
	idleGracePeriod time.Duration
	downloadRoot    string
}

type runtimeManifest struct {
	Version string                          `json:"version"`
	Assets  map[string]runtimeManifestAsset `json:"assets"`
}

type runtimeManifestAsset struct {
	Bundle runtimeBundleAsset `json:"bundle"`
}

type runtimeBundleAsset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type stagedRuntimeBundle struct {
	Version    string `json:"version"`
	BundleURL  string `json:"bundleURL,omitempty"`
	BundleSHA  string `json:"bundleSHA,omitempty"`
	BundlePath string `json:"bundlePath"`
	ExtractDir string `json:"extractDir"`
}

type managedRuntimeFile struct {
	BundleRelativePath string
	TargetRelativePath string
	Executable         bool
	Directory          bool
	MergeDirectory     bool
}

type runtimeUpdaterManager struct {
	mu       sync.RWMutex
	settings runtimeUpdateSettings
	state    op.RuntimeUpdateState
	stage    *stagedRuntimeBundle
}

var runtimeUpdaterState = &runtimeUpdaterManager{
	state: op.RuntimeUpdateState{
		Phase: runtimeUpdatePhaseDisabled,
	},
}

func StartRuntimeUpdater(ctx context.Context) {
	sysCfg := config.GetSystem()
	if sysCfg == nil {
		return
	}

	settings := resolveRuntimeUpdateSettings(sysCfg)
	currentVersion := readCurrentRuntimeVersion(settings.baseDir)

	runtimeUpdaterState.mu.Lock()
	runtimeUpdaterState.settings = settings
	runtimeUpdaterState.stage = nil
	runtimeUpdaterState.state = op.RuntimeUpdateState{
		CurrentVersion: currentVersion,
		Phase:          runtimeUpdatePhaseDisabled,
	}
	runtimeUpdaterState.mu.Unlock()

	if !settings.enabled {
		return
	}
	if currentVersion == "" {
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.Phase = runtimeUpdatePhaseDisabled
			state.LastError = ""
		})
		return
	}

	if stage, err := loadPersistedRuntimeStage(settings, currentVersion); err != nil {
		slog.Warn("runtime updater: failed to load persisted stage", "error", err)
	} else if stage != nil {
		runtimeUpdaterState.mu.Lock()
		runtimeUpdaterState.stage = stage
		runtimeUpdaterState.mu.Unlock()
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.TargetVersion = stage.Version
			state.StagedVersion = stage.Version
			state.Downloaded = true
			state.Applying = false
			state.Phase = runtimeUpdatePhaseStaged
			state.LastError = ""
		})
	} else {
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.Phase = runtimeUpdatePhaseIdle
			state.LastError = ""
		})
	}

	go runRuntimeUpdaterLoop(ctx, settings)
}

func GetRuntimeUpdateSnapshot() *op.RuntimeUpdateState {
	runtimeUpdaterState.mu.RLock()
	defer runtimeUpdaterState.mu.RUnlock()
	snapshot := runtimeUpdaterState.state
	return &snapshot
}

func markRuntimeProcessStarted(baseDir string) {
	version := readVersionFile(baseDir, runtimeLatestVersionFile)
	if strings.TrimSpace(version) == "" {
		return
	}
	if err := writeVersionFile(baseDir, runtimeRunningVersionFile, version); err != nil {
		slog.Warn("runtime updater: failed to mark running version", "baseDir", baseDir, "error", err)
	}
}

func runRuntimeUpdaterLoop(ctx context.Context, settings runtimeUpdateSettings) {
	checkTicker := time.NewTicker(settings.checkInterval)
	defer checkTicker.Stop()
	applyTicker := time.NewTicker(runtimeUpdateApplyPollPeriod)
	defer applyTicker.Stop()

	runUpdateCycle(ctx, settings)

	for {
		select {
		case <-ctx.Done():
			return
		case <-checkTicker.C:
			runUpdateCycle(ctx, settings)
		case <-applyTicker.C:
			applyStagedRuntimeUpdate(settings)
		}
	}
}

// runUpdateCycle checks for runtime self-update first; if the runtime is
// already up to date, it proceeds to reconcile marketplace-managed agents,
// skills and tools.
func runUpdateCycle(ctx context.Context, settings runtimeUpdateSettings) {
	runtimeNeedsUpdate := checkAndStageRuntimeUpdate(ctx, settings)
	if runtimeNeedsUpdate {
		return
	}
	if err := reconcileManagedMarketplaceNodes(ctx, config.GetSystem()); err != nil {
		slog.Warn("marketplace reconcile failed", "error", err)
	}
}

// checkAndStageRuntimeUpdate returns true when a runtime update is staged or
// in progress, meaning the caller should skip marketplace reconciliation.
func checkAndStageRuntimeUpdate(ctx context.Context, settings runtimeUpdateSettings) bool {
	currentVersion := readCurrentRuntimeVersion(settings.baseDir)
	if currentVersion == "" {
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.Phase = runtimeUpdatePhaseDisabled
			state.CurrentVersion = ""
			state.LastError = ""
		})
		return false
	}

	setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
		state.CurrentVersion = currentVersion
		state.Phase = runtimeUpdatePhaseChecking
		state.LastCheckedAt = time.Now().UTC().Format(time.RFC3339)
		state.LastError = ""
		if state.StagedVersion == "" {
			state.TargetVersion = ""
			state.Downloaded = false
		}
	})

	manifest, err := fetchRuntimeManifest(ctx, settings)
	if err != nil {
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.Phase = runtimeUpdatePhaseError
			state.LastError = err.Error()
		})
		return false
	}

	targetVersion := strings.TrimSpace(manifest.Version)
	if targetVersion == "" || !shouldUpdateRuntimeVersion(currentVersion, targetVersion) {
		if targetVersion != "" && targetVersion != currentVersion {
			slog.Info("runtime updater: ignoring non-newer manifest version", "currentVersion", currentVersion, "targetVersion", targetVersion)
		}
		staleVersion := targetVersion
		if stage := currentStagedRuntimeBundle(); stage != nil {
			staleVersion = stage.Version
		}
		if err := clearPersistedRuntimeStage(settings, staleVersion); err != nil {
			slog.Warn("runtime updater: failed to clear stale stage", "error", err)
		}
		runtimeUpdaterState.mu.Lock()
		runtimeUpdaterState.stage = nil
		runtimeUpdaterState.mu.Unlock()
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.CurrentVersion = currentVersion
			state.TargetVersion = ""
			state.StagedVersion = ""
			state.Downloaded = false
			state.Applying = false
			state.Phase = runtimeUpdatePhaseIdle
			state.LastError = ""
		})
		return false
	}

	stage := currentStagedRuntimeBundle()
	if stage != nil && stage.Version == targetVersion && runtimeStageReady(stage) {
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.CurrentVersion = currentVersion
			state.TargetVersion = targetVersion
			state.StagedVersion = targetVersion
			state.Downloaded = true
			state.Applying = false
			state.Phase = runtimeUpdatePhaseStaged
			state.LastError = ""
		})
		return true
	}

	asset, err := pickRuntimeManifestAsset(manifest)
	if err != nil {
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.Phase = runtimeUpdatePhaseError
			state.LastError = err.Error()
		})
		return false
	}

	setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
		state.TargetVersion = targetVersion
		state.Phase = runtimeUpdatePhaseDownloading
		state.Downloaded = false
		state.Applying = false
	})

	stagedBundle, err := stageRuntimeBundle(ctx, settings, targetVersion, asset)
	if err != nil {
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.Phase = runtimeUpdatePhaseError
			state.LastError = err.Error()
		})
		return false
	}

	runtimeUpdaterState.mu.Lock()
	runtimeUpdaterState.stage = stagedBundle
	runtimeUpdaterState.mu.Unlock()
	setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
		state.CurrentVersion = currentVersion
		state.TargetVersion = targetVersion
		state.StagedVersion = targetVersion
		state.Downloaded = true
		state.Applying = false
		state.Phase = runtimeUpdatePhaseStaged
		state.LastError = ""
	})
	return true
}

func applyStagedRuntimeUpdate(settings runtimeUpdateSettings) {
	stage := currentStagedRuntimeBundle()
	if stage == nil {
		return
	}

	currentVersion := readCurrentRuntimeVersion(settings.baseDir)
	if currentVersion != "" && !shouldUpdateRuntimeVersion(currentVersion, stage.Version) {
		if stage.Version != currentVersion {
			slog.Info("runtime updater: ignoring non-newer staged version", "currentVersion", currentVersion, "stagedVersion", stage.Version)
		}
		if err := clearPersistedRuntimeStage(settings, stage.Version); err != nil {
			slog.Warn("runtime updater: failed to clear current stage", "error", err)
		}
		runtimeUpdaterState.mu.Lock()
		runtimeUpdaterState.stage = nil
		runtimeUpdaterState.mu.Unlock()
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.CurrentVersion = currentVersion
			state.TargetVersion = ""
			state.StagedVersion = ""
			state.Downloaded = false
			state.Applying = false
			state.Phase = runtimeUpdatePhaseIdle
			state.LastError = ""
		})
		return
	}

	if !core.IsRuntimeIdle(settings.idleGracePeriod) {
		return
	}

	setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
		state.Phase = runtimeUpdatePhaseApplying
		state.Applying = true
		state.LastError = ""
	})

	if err := applyRuntimeStage(settings, stage); err != nil {
		setRuntimeUpdateState(func(state *op.RuntimeUpdateState) {
			state.Phase = runtimeUpdatePhaseError
			state.Applying = false
			state.LastError = err.Error()
		})
	}
}

func resolveRuntimeUpdateSettings(sysCfg *op.SystemConfig) runtimeUpdateSettings {
	settings := runtimeUpdateSettings{
		enabled:         true,
		baseDir:         strings.TrimSpace(sysCfg.BaseDir),
		manifestURL:     strings.TrimSpace(sysCfg.RuntimeUpdate.ManifestURL),
		checkInterval:   parseDurationOrDefault(sysCfg.RuntimeUpdate.CheckInterval, config.DefaultRuntimeUpdateCheckInterval),
		checkTimeout:    parseDurationOrDefault(sysCfg.RuntimeUpdate.CheckTimeout, config.DefaultRuntimeUpdateCheckTimeout),
		idleGracePeriod: parseDurationOrDefault(sysCfg.RuntimeUpdate.IdleGracePeriod, config.DefaultRuntimeUpdateIdleGracePeriod),
	}
	if sysCfg.RuntimeUpdate.Enabled != nil {
		settings.enabled = *sysCfg.RuntimeUpdate.Enabled
	}
	if settings.manifestURL == "" {
		settings.manifestURL = config.DefaultRuntimeUpdateManifestURL
	}
	downloadDir := strings.TrimSpace(sysCfg.RuntimeUpdate.DownloadDir)
	if downloadDir == "" {
		downloadDir = config.DefaultRuntimeUpdateDownloadDir
	}
	if filepath.IsAbs(downloadDir) {
		settings.downloadRoot = downloadDir
	} else {
		settings.downloadRoot = filepath.Join(settings.baseDir, downloadDir)
	}
	return settings
}

func parseDurationOrDefault(raw string, fallback string) time.Duration {
	if duration, err := time.ParseDuration(strings.TrimSpace(raw)); err == nil && duration > 0 {
		return duration
	}
	duration, err := time.ParseDuration(strings.TrimSpace(fallback))
	if err != nil || duration <= 0 {
		return time.Minute
	}
	return duration
}

func fetchRuntimeManifest(ctx context.Context, settings runtimeUpdateSettings) (*runtimeManifest, error) {
	requestCtx, cancel := context.WithTimeout(ctx, settings.checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, settings.manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("runtime updater: create manifest request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("runtime updater: fetch manifest: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return nil, fmt.Errorf("runtime updater: manifest response %s: %s", res.Status, strings.TrimSpace(string(body)))
	}

	var manifest runtimeManifest
	if err := json.NewDecoder(res.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("runtime updater: decode manifest: %w", err)
	}
	if strings.TrimSpace(manifest.Version) == "" {
		return nil, errors.New("runtime updater: manifest is missing version")
	}
	if len(manifest.Assets) == 0 {
		return nil, errors.New("runtime updater: manifest is missing assets")
	}
	return &manifest, nil
}

func pickRuntimeManifestAsset(manifest *runtimeManifest) (runtimeBundleAsset, error) {
	if manifest == nil {
		return runtimeBundleAsset{}, errors.New("runtime updater: manifest is nil")
	}
	key, err := runtimePlatformKey()
	if err != nil {
		return runtimeBundleAsset{}, err
	}
	entry, ok := manifest.Assets[key]
	if !ok {
		return runtimeBundleAsset{}, fmt.Errorf("runtime updater: manifest missing assets for %s", key)
	}
	if strings.TrimSpace(entry.Bundle.URL) == "" || strings.TrimSpace(entry.Bundle.SHA256) == "" {
		return runtimeBundleAsset{}, fmt.Errorf("runtime updater: manifest has invalid bundle asset for %s", key)
	}
	return entry.Bundle, nil
}

func runtimePlatformKey() (string, error) {
	var osName string
	switch goruntime.GOOS {
	case "darwin":
		osName = "darwin"
	case "linux":
		osName = "linux"
	case "windows":
		osName = "windows"
	default:
		return "", fmt.Errorf("runtime updater: unsupported GOOS %s", goruntime.GOOS)
	}

	var arch string
	switch goruntime.GOARCH {
	case "amd64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", fmt.Errorf("runtime updater: unsupported GOARCH %s", goruntime.GOARCH)
	}
	return fmt.Sprintf("%s-%s", osName, arch), nil
}

func stageRuntimeBundle(ctx context.Context, settings runtimeUpdateSettings, version string, asset runtimeBundleAsset) (*stagedRuntimeBundle, error) {
	stageDir := filepath.Join(settings.downloadRoot, sanitizeRuntimeVersion(version))
	bundlePath := filepath.Join(stageDir, "bundle.tar.gz")
	extractDir := filepath.Join(stageDir, "extract")

	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return nil, fmt.Errorf("runtime updater: create stage dir: %w", err)
	}

	if err := downloadRuntimeBundle(ctx, asset.URL, bundlePath, asset.SHA256); err != nil {
		return nil, err
	}
	if err := os.RemoveAll(extractDir); err != nil {
		return nil, fmt.Errorf("runtime updater: reset extract dir: %w", err)
	}
	if err := extractRuntimeArchive(bundlePath, extractDir); err != nil {
		return nil, err
	}

	stage := &stagedRuntimeBundle{
		Version:    version,
		BundleURL:  strings.TrimSpace(asset.URL),
		BundleSHA:  strings.TrimSpace(asset.SHA256),
		BundlePath: bundlePath,
		ExtractDir: extractDir,
	}
	if err := validateRuntimeStage(stage); err != nil {
		return nil, err
	}
	if err := persistRuntimeStage(settings, stage); err != nil {
		return nil, err
	}
	return stage, nil
}

func downloadRuntimeBundle(ctx context.Context, url string, destination string, expectedSHA string) error {
	requestCtx, cancel := context.WithTimeout(ctx, runtimeUpdateDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("runtime updater: create bundle request: %w", err)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("runtime updater: download bundle: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("runtime updater: bundle response %s: %s", res.Status, strings.TrimSpace(string(body)))
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("runtime updater: create bundle dir: %w", err)
	}
	tmpFile, err := os.CreateTemp(filepath.Dir(destination), "bundle-*.tmp")
	if err != nil {
		return fmt.Errorf("runtime updater: create temp bundle: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}()

	hash := sha256.New()
	writer := io.MultiWriter(tmpFile, hash)
	if _, err := io.Copy(writer, res.Body); err != nil {
		return fmt.Errorf("runtime updater: write bundle: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("runtime updater: finalize bundle: %w", err)
	}

	actualSHA := hex.EncodeToString(hash.Sum(nil))
	if !strings.EqualFold(actualSHA, strings.TrimSpace(expectedSHA)) {
		return fmt.Errorf("runtime updater: sha256 mismatch: expected %s got %s", expectedSHA, actualSHA)
	}
	if err := os.Rename(tmpPath, destination); err != nil {
		return fmt.Errorf("runtime updater: move bundle into place: %w", err)
	}
	return nil
}

func extractRuntimeArchive(archivePath string, destination string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("runtime updater: open archive: %w", err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("runtime updater: create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fmt.Errorf("runtime updater: create extract dir: %w", err)
	}

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("runtime updater: read archive entry: %w", err)
		}

		targetPath := filepath.Join(destination, header.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destination)+string(os.PathSeparator)) {
			return fmt.Errorf("runtime updater: invalid archive path %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("runtime updater: create archive dir: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("runtime updater: create parent dir: %w", err)
			}
			fileMode := os.FileMode(0o644)
			if header.Mode&0o111 != 0 {
				fileMode = 0o755
			}
			outFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode)
			if err != nil {
				return fmt.Errorf("runtime updater: create extracted file: %w", err)
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				_ = outFile.Close()
				return fmt.Errorf("runtime updater: write extracted file: %w", err)
			}
			if err := outFile.Close(); err != nil {
				return fmt.Errorf("runtime updater: close extracted file: %w", err)
			}
		}
	}
}

func validateRuntimeStage(stage *stagedRuntimeBundle) error {
	if stage == nil {
		return errors.New("runtime updater: staged bundle is nil")
	}
	if err := runtimeStageReadyError(stage); err != nil {
		return err
	}
	return nil
}

func runtimeStageReady(stage *stagedRuntimeBundle) bool {
	return runtimeStageReadyError(stage) == nil
}

func runtimeStageReadyError(stage *stagedRuntimeBundle) error {
	if stage == nil {
		return errors.New("runtime updater: staged bundle is nil")
	}
	for _, spec := range runtimeManagedFiles() {
		filePath := filepath.Join(stage.ExtractDir, spec.BundleRelativePath)
		stat, err := os.Stat(filePath)
		if err != nil {
			return fmt.Errorf("runtime updater: staged bundle is missing %s", spec.BundleRelativePath)
		}
		if spec.Directory && !stat.IsDir() {
			return fmt.Errorf("runtime updater: staged bundle path is not a directory: %s", spec.BundleRelativePath)
		}
		if !spec.Directory && stat.IsDir() {
			return fmt.Errorf("runtime updater: staged bundle path is not a file: %s", spec.BundleRelativePath)
		}
	}
	return nil
}

func applyRuntimeStage(settings runtimeUpdateSettings, stage *stagedRuntimeBundle) error {
	if err := applyRuntimeStageFiles(settings, stage); err != nil {
		return err
	}

	closed := core.CloseDaemonConnections()
	if closed > 0 {
		slog.Info("runtime updater: closed daemon connections before restart", "count", closed)
	}

	executablePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("runtime updater: resolve executable: %w", err)
	}
	slog.Info("runtime updater: restarting runtime", "version", stage.Version, "executable", executablePath)
	return restartRuntimeProcess(executablePath, os.Args, os.Environ())
}

func applyRuntimeStageFiles(settings runtimeUpdateSettings, stage *stagedRuntimeBundle) error {
	if err := validateRuntimeStage(stage); err != nil {
		return err
	}

	type preparedFile struct {
		targetPath string
		tempPath   string
		directory  bool
	}
	prepared := make([]preparedFile, 0, len(runtimeManagedFiles()))
	for _, spec := range runtimeManagedFiles() {
		sourcePath := filepath.Join(stage.ExtractDir, spec.BundleRelativePath)
		targetPath := filepath.Join(settings.baseDir, spec.TargetRelativePath)
		var tempPath string
		var err error
		if spec.Directory {
			if spec.MergeDirectory {
				tempPath, err = prepareRuntimeMergedDirectory(sourcePath, targetPath)
			} else {
				tempPath, err = prepareRuntimeManagedDirectory(sourcePath, targetPath)
			}
		} else {
			tempPath, err = prepareRuntimeManagedFile(sourcePath, targetPath, spec.Executable)
		}
		if err != nil {
			for _, item := range prepared {
				_ = os.RemoveAll(item.tempPath)
			}
			return err
		}
		prepared = append(prepared, preparedFile{
			targetPath: targetPath,
			tempPath:   tempPath,
			directory:  spec.Directory,
		})
	}

	for _, item := range prepared {
		if item.directory {
			if err := replaceRuntimeManagedDirectory(item.tempPath, item.targetPath); err != nil {
				return fmt.Errorf("runtime updater: replace %s: %w", item.targetPath, err)
			}
			continue
		}
		if err := os.Rename(item.tempPath, item.targetPath); err != nil {
			return fmt.Errorf("runtime updater: replace %s: %w", item.targetPath, err)
		}
	}
	if err := common.ProjectSystemToolBins(settings.baseDir); err != nil {
		return fmt.Errorf("runtime updater: project system bins: %w", err)
	}

	if err := writeVersionFile(settings.baseDir, runtimeLatestVersionFile, stage.Version); err != nil {
		return fmt.Errorf("runtime updater: write latest version: %w", err)
	}
	return nil
}

func runtimeExeSuffix() string {
	if goruntime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func replaceRuntimeManagedDirectory(sourceDir string, targetDir string) error {
	backupDir := targetDir + fmt.Sprintf(".backup-%d", time.Now().UnixNano())
	hasBackup := false
	if _, err := os.Stat(targetDir); err == nil {
		if err := os.Rename(targetDir, backupDir); err != nil {
			return err
		}
		hasBackup = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(sourceDir, targetDir); err != nil {
		if hasBackup {
			_ = os.Rename(backupDir, targetDir)
		}
		return err
	}
	if hasBackup {
		_ = os.RemoveAll(backupDir)
	}
	return nil
}

func prepareRuntimeManagedFile(sourcePath string, targetPath string, executable bool) (string, error) {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("runtime updater: create target dir: %w", err)
	}
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("runtime updater: open source %s: %w", sourcePath, err)
	}
	defer sourceFile.Close()

	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".update-*")
	if err != nil {
		return "", fmt.Errorf("runtime updater: create temp file for %s: %w", targetPath, err)
	}
	tempPath := tempFile.Name()
	if _, err := io.Copy(tempFile, sourceFile); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("runtime updater: copy %s: %w", targetPath, err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("runtime updater: close temp file %s: %w", targetPath, err)
	}
	if executable && goruntime.GOOS != "windows" {
		if err := os.Chmod(tempPath, 0o755); err != nil {
			_ = os.Remove(tempPath)
			return "", fmt.Errorf("runtime updater: chmod temp file %s: %w", targetPath, err)
		}
	}
	return tempPath, nil
}

func prepareRuntimeManagedDirectory(sourcePath string, targetPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("runtime updater: create target dir: %w", err)
	}
	tempDir, err := os.MkdirTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".update-*")
	if err != nil {
		return "", fmt.Errorf("runtime updater: create temp dir for %s: %w", targetPath, err)
	}
	if err := copyRuntimeTree(sourcePath, tempDir); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}
	return tempDir, nil
}

func prepareRuntimeMergedDirectory(sourcePath string, targetPath string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", fmt.Errorf("runtime updater: create target dir: %w", err)
	}
	tempDir, err := os.MkdirTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".update-*")
	if err != nil {
		return "", fmt.Errorf("runtime updater: create temp dir for %s: %w", targetPath, err)
	}
	if stat, err := os.Stat(targetPath); err == nil {
		if !stat.IsDir() {
			_ = os.RemoveAll(tempDir)
			return "", fmt.Errorf("runtime updater: target path is not a directory: %s", targetPath)
		}
		if err := copyRuntimeTree(targetPath, tempDir); err != nil {
			_ = os.RemoveAll(tempDir)
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		_ = os.RemoveAll(tempDir)
		return "", err
	}
	entries, err := os.ReadDir(sourcePath)
	if err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(tempDir, entry.Name())); err != nil {
			_ = os.RemoveAll(tempDir)
			return "", err
		}
	}
	if err := copyRuntimeTree(sourcePath, tempDir); err != nil {
		_ = os.RemoveAll(tempDir)
		return "", err
	}
	return tempDir, nil
}

func copyRuntimeTree(sourceDir string, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := targetDir
		if rel != "." {
			targetPath = filepath.Join(targetDir, rel)
		}
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyRuntimeFileWithMode(path, targetPath, info.Mode().Perm())
	})
}

func copyRuntimeFileWithMode(sourcePath string, targetPath string, mode os.FileMode) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("runtime updater: open source %s: %w", sourcePath, err)
	}
	defer sourceFile.Close()
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return fmt.Errorf("runtime updater: create target dir: %w", err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("runtime updater: create temp file for %s: %w", targetPath, err)
	}
	tempPath := tempFile.Name()
	if _, err := io.Copy(tempFile, sourceFile); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return fmt.Errorf("runtime updater: copy %s: %w", targetPath, err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("runtime updater: close temp file %s: %w", targetPath, err)
	}
	if mode == 0 {
		mode = 0o644
	}
	if err := os.Chmod(tempPath, mode); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("runtime updater: chmod temp file %s: %w", targetPath, err)
	}
	return os.Rename(tempPath, targetPath)
}

func runtimeManagedFiles() []managedRuntimeFile {
	exeSuffix := runtimeExeSuffix()
	return []managedRuntimeFile{
		{BundleRelativePath: filepath.Join("bin", "opagent-runtime"+exeSuffix), TargetRelativePath: filepath.Join("bin", "opagent-runtime"+exeSuffix), Executable: true},
		{BundleRelativePath: filepath.Join("bin", "opagent-bootstrap"+exeSuffix), TargetRelativePath: filepath.Join("bin", "opagent-bootstrap"+exeSuffix), Executable: true},
		{BundleRelativePath: filepath.Join("bin", "gbrain"+exeSuffix), TargetRelativePath: filepath.Join("bin", "gbrain"+exeSuffix), Executable: true},
		{BundleRelativePath: "agents", TargetRelativePath: "agents", Directory: true, MergeDirectory: true},
		{BundleRelativePath: "tools", TargetRelativePath: "tools", Directory: true, MergeDirectory: true},
		{BundleRelativePath: "skills", TargetRelativePath: "skills", Directory: true, MergeDirectory: true},
		{BundleRelativePath: filepath.Join("configs", "config.json"), TargetRelativePath: filepath.Join("configs", "config.json"), Executable: false},
	}
}

func currentStagedRuntimeBundle() *stagedRuntimeBundle {
	runtimeUpdaterState.mu.RLock()
	defer runtimeUpdaterState.mu.RUnlock()
	if runtimeUpdaterState.stage == nil {
		return nil
	}
	stage := *runtimeUpdaterState.stage
	return &stage
}

func loadPersistedRuntimeStage(settings runtimeUpdateSettings, currentVersion string) (*stagedRuntimeBundle, error) {
	statePath := filepath.Join(settings.downloadRoot, runtimeStageStateFile)
	raw, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("runtime updater: read stage state: %w", err)
	}
	var stage stagedRuntimeBundle
	if err := json.Unmarshal(raw, &stage); err != nil {
		return nil, fmt.Errorf("runtime updater: decode stage state: %w", err)
	}
	if strings.TrimSpace(stage.Version) == "" || !shouldUpdateRuntimeVersion(currentVersion, stage.Version) {
		if stage.Version != "" && stage.Version != currentVersion {
			slog.Info("runtime updater: ignoring non-newer persisted stage", "currentVersion", currentVersion, "stagedVersion", stage.Version)
		}
		if err := clearPersistedRuntimeStage(settings, stage.Version); err != nil {
			slog.Warn("runtime updater: failed to clear stale persisted stage", "error", err)
		}
		return nil, nil
	}
	if err := validateRuntimeStage(&stage); err != nil {
		if clearErr := clearPersistedRuntimeStage(settings, stage.Version); clearErr != nil {
			slog.Warn("runtime updater: failed to clear invalid persisted stage", "error", clearErr)
		}
		return nil, err
	}
	return &stage, nil
}

func persistRuntimeStage(settings runtimeUpdateSettings, stage *stagedRuntimeBundle) error {
	if stage == nil {
		return errors.New("runtime updater: stage is nil")
	}
	if err := os.MkdirAll(settings.downloadRoot, 0o755); err != nil {
		return fmt.Errorf("runtime updater: create download root: %w", err)
	}
	payload, err := json.MarshalIndent(stage, "", "  ")
	if err != nil {
		return fmt.Errorf("runtime updater: encode stage state: %w", err)
	}
	statePath := filepath.Join(settings.downloadRoot, runtimeStageStateFile)
	return writeTextFileAtomic(statePath, string(payload))
}

func clearPersistedRuntimeStage(settings runtimeUpdateSettings, version string) error {
	statePath := filepath.Join(settings.downloadRoot, runtimeStageStateFile)
	_ = os.Remove(statePath)
	if strings.TrimSpace(version) != "" {
		_ = os.RemoveAll(filepath.Join(settings.downloadRoot, sanitizeRuntimeVersion(version)))
	}
	return nil
}

func setRuntimeUpdateState(update func(state *op.RuntimeUpdateState)) {
	runtimeUpdaterState.mu.Lock()
	defer runtimeUpdaterState.mu.Unlock()
	next := runtimeUpdaterState.state
	update(&next)
	runtimeUpdaterState.state = next
}

func shouldUpdateRuntimeVersion(currentVersion string, targetVersion string) bool {
	currentVersion = strings.TrimSpace(currentVersion)
	targetVersion = strings.TrimSpace(targetVersion)
	if targetVersion == "" || targetVersion == currentVersion {
		return false
	}
	if cmp, ok := compareRuntimeSemver(targetVersion, currentVersion); ok {
		return cmp > 0
	}
	// Preserve legacy behavior for non-semver custom builds, but never downgrade
	// normal release versions such as 0.6.4 -> 0.6.3.
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

func readCurrentRuntimeVersion(baseDir string) string {
	if version := readVersionFile(baseDir, runtimeRunningVersionFile); version != "" {
		return version
	}
	return readVersionFile(baseDir, runtimeLatestVersionFile)
}

func readVersionFile(baseDir string, filename string) string {
	filePath := filepath.Join(baseDir, "run", filename)
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func writeVersionFile(baseDir string, filename string, value string) error {
	return writeTextFileAtomic(filepath.Join(baseDir, "run", filename), strings.TrimSpace(value)+"\n")
}

func clearVersionFile(baseDir string, filename string) error {
	return os.Remove(filepath.Join(baseDir, "run", filename))
}

func writeTextFileAtomic(targetPath string, value string) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(filepath.Dir(targetPath), filepath.Base(targetPath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	if _, err := io.WriteString(tempFile, value); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return os.Rename(tempPath, targetPath)
}

func sanitizeRuntimeVersion(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" {
		return "unknown"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, trimmed)
}
