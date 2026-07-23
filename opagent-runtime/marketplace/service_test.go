package marketplace

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

type testArchiveFile struct {
	Path string
	Body string
	Mode int64
}

func TestServiceReconcileInstallsManagedBuiltins(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	baseDir := filepath.Join(homeDir, ".openbrain")
	assetDir := t.TempDir()

	coderArchive := writeTestTarGz(t, filepath.Join(assetDir, "coder.tar.gz"), []testArchiveFile{
		{Path: ".agent/AGENT.md", Body: "---\nid: agent-coder\nname: coder\nrun:\n  command: [\"./bin/coder\"]\n  daemon: true\n---\n", Mode: 0o644},
		{Path: ".agent/bin/coder", Body: "#!/bin/sh\n", Mode: 0o755},
	})
	completionArchive := writeTestTarGz(t, filepath.Join(assetDir, "completion.tar.gz"), []testArchiveFile{
		{Path: ".agent/AGENT.md", Body: "---\nname: completion\nopcodes:\n  - editor/completion\n  - editor/completion/cancel\n---\n", Mode: 0o644},
	})
	gbrainArchive := writeTestTarGz(t, filepath.Join(assetDir, "gbrain.tar.gz"), []testArchiveFile{
		{Path: ".agent/AGENT.md", Body: "---\nid: agent-gbrain\nname: GBrain\ntools:\n  - read\n  - shell\n---\n", Mode: 0o644},
		{Path: ".agent/skills/query/SKILL.md", Body: "---\nname: query\ndescription: test\n---\n", Mode: 0o644},
	})
	researcherArchive := writeTestTarGz(t, filepath.Join(assetDir, "researcher.tar.gz"), []testArchiveFile{
		{Path: ".agent/AGENT.md", Body: "---\nname: researcher\nskills:\n  - ./skills/deep-research/SKILL.md\n  - ./skills/report-synthesis/SKILL.md\ntools:\n  - ./tools/research-tools\nrun:\n  command: [\"./bin/researcher\"]\n  daemon: true\n---\n", Mode: 0o644},
		{Path: ".agent/bin/researcher", Body: "#!/bin/sh\n", Mode: 0o755},
		{Path: ".agent/skills/deep-research/SKILL.md", Body: "---\nname: Deep Research\ndescription: test\n---\n", Mode: 0o644},
		{Path: ".agent/skills/report-synthesis/SKILL.md", Body: "---\nname: Report Synthesis\ndescription: test\n---\n", Mode: 0o644},
		{Path: ".agent/tools/research-tools/TOOL.md", Body: "---\nname: research-tools\nrun:\n  command: [\"./bin/research-tools\"]\n  daemon: true\n---\n", Mode: 0o644},
		{Path: ".agent/tools/research-tools/bin/research-tools", Body: "#!/bin/sh\n", Mode: 0o755},
	})
	planArchive := writeTestTarGz(t, filepath.Join(assetDir, "plan.tar.gz"), []testArchiveFile{{Path: "SKILL.md", Body: "---\nname: Plan\ndescription: test\n---\n", Mode: 0o644}})
	executePlanArchive := writeTestTarGz(t, filepath.Join(assetDir, "execute-plan.tar.gz"), []testArchiveFile{{Path: "SKILL.md", Body: "---\nname: Execute Plan\ndescription: test\n---\n", Mode: 0o644}})
	skillCreatorArchive := writeTestTarGz(t, filepath.Join(assetDir, "skill-creator.tar.gz"), []testArchiveFile{{Path: "SKILL.md", Body: "---\nname: Skill Creator\ndescription: test\n---\n", Mode: 0o644}})
	agentBrowserSearchArchive := writeTestTarGz(t, filepath.Join(assetDir, "agent-browser-search.tar.gz"), []testArchiveFile{{Path: "SKILL.md", Body: "---\nname: Agent Browser Search\ndescription: test\n---\n", Mode: 0o644}})
	rgSearchArchive := writeTestTarGz(t, filepath.Join(assetDir, "rg-search.tar.gz"), []testArchiveFile{
		{Path: "TOOL.md", Body: "---\nname: rg-search\ntags: system\n---\n", Mode: 0o644},
		{Path: "bin/rg", Body: "#!/bin/sh\n", Mode: 0o755},
	})

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	catalog := Catalog{
		Version:     "2026.04.11",
		GeneratedAt: "2026-04-11T00:00:00Z",
		Items: []CatalogItem{
			buildTestCatalogItem(KindAgent, "gbrain", gbrainArchive, server.URL),
			buildTestCatalogItem(KindAgent, "completion", completionArchive, server.URL),
			buildTestCatalogItem(KindAgent, "coder", coderArchive, server.URL),
			buildTestCatalogItem(KindAgent, "researcher", researcherArchive, server.URL),
			buildTestCatalogItem(KindSkill, "plan", planArchive, server.URL),
			buildTestCatalogItem(KindSkill, "execute-plan", executePlanArchive, server.URL),
			buildTestCatalogItem(KindSkill, "skill-creator", skillCreatorArchive, server.URL),
			buildTestCatalogItem(KindSkill, "agent-browser-search", agentBrowserSearchArchive, server.URL),
			buildTestCatalogItem(KindTool, "rg-search", rgSearchArchive, server.URL),
		},
	}
	mux.HandleFunc("/index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(catalog)
	})
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(assetDir, filepath.Base(r.URL.Path)))
	})

	service := NewService(baseDir, Options{CatalogURL: server.URL + "/index.json"})
	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile(): %v", err)
	}

	for _, item := range catalog.Items {
		installPath := InstallPath(baseDir, item.Kind, item.ID)
		if !installRootReady(installPath, item.Kind) {
			t.Fatalf("managed install root not ready: %s/%s", item.Kind, item.ID)
		}
	}
	if _, err := os.Stat(filepath.Join(baseDir, "bin", "rg")); err != nil {
		t.Fatalf("projected rg binary missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "agents", "gbrain", ".agent", "skills", "query", "SKILL.md")); err != nil {
		t.Fatalf("gbrain query skill missing after marketplace reconcile: %v", err)
	}

	state, err := service.loadState()
	if err != nil {
		t.Fatalf("loadState(): %v", err)
	}
	if len(state.Items) != len(catalog.Items) {
		t.Fatalf("managed state items = %d, want %d", len(state.Items), len(catalog.Items))
	}
}

func TestServiceListItemsHidesRuntimeOnlySystoolRecord(t *testing.T) {
	baseDir := t.TempDir()
	service := NewService(baseDir, Options{})
	state := &StateFile{
		Version: stateVersion,
		Items: []ManagedItemRecord{{
			ID:               "systool",
			Kind:             KindTool,
			Managed:          true,
			Builtin:          true,
			InstallPath:      InstallPath(baseDir, KindTool, "systool"),
			InstalledVersion: "1",
		}},
	}
	catalog := &Catalog{Version: "1", Items: []CatalogItem{{
		ID:          "systool",
		Kind:        KindTool,
		Name:        "Systool",
		Description: "runtime-only",
		Builtin:     true,
		Version:     "1",
	}}}
	result := service.buildListResult(state, catalog, UsageReport{}, nil)
	if len(result.Items) != 0 {
		t.Fatalf("items length = %d, want 0", len(result.Items))
	}
}

func TestServiceInstallRejectsRuntimeOnlySystool(t *testing.T) {
	baseDir := t.TempDir()
	service := NewService(baseDir, Options{})
	result, err := service.InstallItem(context.Background(), KindTool, "systool", UsageReport{})
	if err != nil {
		t.Fatalf("InstallItem(): %v", err)
	}
	if result.Success {
		t.Fatalf("InstallItem(systool) success = true, want false")
	}
}

func TestServiceListItemsMarksInUseFromUsageReport(t *testing.T) {
	baseDir := t.TempDir()
	service := NewService(baseDir, Options{})
	state := &StateFile{
		Version: stateVersion,
		Items: []ManagedItemRecord{{
			ID:               "coder",
			Kind:             KindAgent,
			Managed:          true,
			Builtin:          true,
			InstallPath:      InstallPath(baseDir, KindAgent, "coder"),
			InstalledVersion: "1",
		}},
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "agents", "coder", ".agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "agents", "coder", ".agent", "AGENT.md"), []byte("---\nname: coder\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	catalog := &Catalog{Version: "1", Items: []CatalogItem{{
		ID: "coder", Kind: KindAgent, Name: "Coder", Description: "test", Builtin: true, Version: "1",
		Assets: map[string]Asset{"any": {URL: "https://example.com/coder.tar.gz", SHA256: "abc"}},
	}}}
	result := service.buildListResult(state, catalog, UsageReport{Agents: []string{"coder"}}, nil)
	if len(result.Items) != 1 {
		t.Fatalf("items length = %d, want 1", len(result.Items))
	}
	if !result.Items[0].InUse {
		t.Fatalf("InUse = false, want true")
	}
}

func TestServiceInstallsOrgItemIntoFlatDirectoryAndIndex(t *testing.T) {
	baseDir := t.TempDir()
	assetDir := t.TempDir()
	orgArchive := writeTestTarGz(t, filepath.Join(assetDir, "review.tar.gz"), []testArchiveFile{
		{Path: "SKILL.md", Body: "---\nname: Review\ndescription: Internal review\n---\n", Mode: 0o644},
	})

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	authDir := filepath.Join(baseDir, "configs", "user")
	if err := os.MkdirAll(authDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authDir, "auth.json"), []byte(`{"version":2,"gateway":"`+server.URL+`","token":"tok","uid":"user-test","deploymentID":"dep-test","orgID":"org-acme","identityID":"idn-test","connectionID":"conn-test","authMethod":"email","authTime":"2026-07-23T00:00:00Z","expiresAt":"2026-07-24T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog := Catalog{
		Version: "org-acme-1",
		Items: []CatalogItem{{
			ID:          "review",
			Kind:        KindSkill,
			Name:        "Review",
			Description: "Internal review",
			Version:     "1.0.0",
			Scope:       "org",
			OrgID:       "org-acme",
			OrgName:     "Acme",
			Assets: map[string]Asset{
				"any": {
					URL:    server.URL + "/assets/" + filepath.Base(orgArchive),
					SHA256: testSHA256Hex(orgArchive),
				},
			},
		}},
	}
	mux.HandleFunc("/api/v1/user/orgs/org-acme/marketplace/catalog", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Fatalf("Authorization = %q, want Bearer tok", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(catalog)
	})
	mux.HandleFunc("/assets/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, filepath.Join(assetDir, filepath.Base(r.URL.Path)))
	})

	service := NewService(baseDir, Options{})
	result, err := service.InstallOrgItem(context.Background(), "org-acme", KindSkill, "review", UsageReport{})
	if err != nil {
		t.Fatalf("InstallOrgItem err = %v", err)
	}
	if !result.Success {
		t.Fatalf("InstallOrgItem success=false error=%s", result.Error)
	}
	installPath := filepath.Join(baseDir, "skills", "review")
	if _, err := os.Stat(filepath.Join(installPath, "SKILL.md")); err != nil {
		t.Fatalf("org skill not installed at flat path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(baseDir, "skills", "@org-acme", "review", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("org namespace path should not be used for new installs, stat err = %v", err)
	}
	if _, err := os.Stat(StatePath(baseDir)); err != nil {
		t.Fatalf("marketplace package index missing: %v", err)
	}
	if result.Item == nil || result.Item.Scope != "org" || result.Item.OrgID != "org-acme" || result.Item.LocalName != "review" {
		t.Fatalf("result item = %#v", result.Item)
	}
	state, err := service.loadState()
	if err != nil {
		t.Fatalf("loadState(): %v", err)
	}
	if len(state.Items) != 1 || state.Items[0].LocalName != "review" || state.Items[0].InstallPath != installPath {
		t.Fatalf("state items = %#v", state.Items)
	}
}

func testSHA256Hex(path string) string {
	sha := sha256.Sum256(mustReadFile(path))
	return hex.EncodeToString(sha[:])
}

func buildTestCatalogItem(kind Kind, id string, archivePath string, serverURL string) CatalogItem {
	sha := sha256.Sum256(mustReadFile(archivePath))
	return CatalogItem{
		ID:          id,
		Kind:        kind,
		Name:        id,
		Description: id + " test package",
		Builtin:     true,
		Version:     "2026.04.11",
		Assets: map[string]Asset{
			"any": {
				URL:    serverURL + "/assets/" + filepath.Base(archivePath),
				SHA256: hex.EncodeToString(sha[:]),
			},
		},
	}
}

func writeTestTarGz(t *testing.T, archivePath string, files []testArchiveFile) string {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("Create(%s): %v", archivePath, err)
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tarWriter := tar.NewWriter(gz)
	defer tarWriter.Close()
	for _, entry := range files {
		header := &tar.Header{Name: entry.Path, Mode: entry.Mode, Size: int64(len(entry.Body))}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("WriteHeader(%s): %v", entry.Path, err)
		}
		if _, err := tarWriter.Write([]byte(entry.Body)); err != nil {
			t.Fatalf("Write(%s): %v", entry.Path, err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("tarWriter.Close(): %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz.Close(): %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("file.Close(): %v", err)
	}
	return archivePath
}

func mustReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return data
}
