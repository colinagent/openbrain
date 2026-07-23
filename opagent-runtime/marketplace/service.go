package marketplace

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
	"net/http"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
	"time"

	"github.com/colinagent/openbrain/opagent-runtime/packages/common"
)

const (
	DefaultCatalogURL = "https://download.op-agent.com/marketplace/latest/index.json"
	stateVersion      = 1
)

type Kind string

const (
	KindAgent Kind = "agent"
	KindSkill Kind = "skill"
	KindTool  Kind = "tool"
)

type BuiltinItem struct {
	Kind     Kind
	ID       string
	Required bool
}

var BuiltinManagedItems = []BuiltinItem{}

var runtimeOnlyBuiltinItems = []struct {
	Kind Kind
	ID   string
}{
	{Kind: KindTool, ID: "systool"},
}

type Asset struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type CatalogItem struct {
	ID          string           `json:"id"`
	Kind        Kind             `json:"kind"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Builtin     bool             `json:"builtin"`
	Version     string           `json:"version"`
	Assets      map[string]Asset `json:"assets"`
	Scope       string           `json:"scope,omitempty"`
	OrgID       string           `json:"orgID,omitempty"`
	OrgName     string           `json:"orgName,omitempty"`
}

type Catalog struct {
	Version     string        `json:"version"`
	GeneratedAt string        `json:"generatedAt,omitempty"`
	Items       []CatalogItem `json:"items"`
}

type ManagedItemRecord struct {
	ID               string   `json:"id"`
	Kind             Kind     `json:"kind"`
	Scope            string   `json:"scope,omitempty"`
	OrgID            string   `json:"orgID,omitempty"`
	OrgName          string   `json:"orgName,omitempty"`
	LocalName        string   `json:"localName,omitempty"`
	InstalledVersion string   `json:"installedVersion,omitempty"`
	SourceURL        string   `json:"sourceUrl,omitempty"`
	Managed          bool     `json:"managed"`
	Builtin          bool     `json:"builtin"`
	InstallPath      string   `json:"installPath"`
	ProjectedPaths   []string `json:"projectedPaths,omitempty"`
	LastCheckedAt    int64    `json:"lastCheckedAt"`
	UpdateAvailable  bool     `json:"updateAvailable"`
}

type StateFile struct {
	Version            int                 `json:"version"`
	LastCatalogVersion string              `json:"lastCatalogVersion,omitempty"`
	LastCatalogAt      string              `json:"lastCatalogAt,omitempty"`
	Items              []ManagedItemRecord `json:"items"`
}

type ItemStatus string

const (
	StatusNotInstalled ItemStatus = "not_installed"
	StatusInstalled    ItemStatus = "installed"
	StatusUpdateAvail  ItemStatus = "update_available"
)

type ListItem struct {
	ID                 string     `json:"id"`
	Kind               Kind       `json:"kind"`
	Scope              string     `json:"scope,omitempty"`
	OrgID              string     `json:"orgID,omitempty"`
	OrgName            string     `json:"orgName,omitempty"`
	LocalName          string     `json:"localName,omitempty"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	Builtin            bool       `json:"builtin"`
	Version            string     `json:"version,omitempty"`
	InstalledVersion   string     `json:"installedVersion,omitempty"`
	InstallPath        string     `json:"installPath"`
	SourceURL          string     `json:"sourceUrl,omitempty"`
	Managed            bool       `json:"managed"`
	UpdateAvailable    bool       `json:"updateAvailable"`
	InUse              bool       `json:"inUse"`
	Status             ItemStatus `json:"status"`
	MissingFromCatalog bool       `json:"missingFromCatalog"`
}

type ListResult struct {
	Items          []ListItem `json:"items"`
	CatalogVersion string     `json:"catalogVersion,omitempty"`
	GeneratedAt    string     `json:"generatedAt,omitempty"`
	Error          string     `json:"error,omitempty"`
}

type ActionResult struct {
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
	Item    *ListItem `json:"item,omitempty"`
}

type StateResult struct {
	State          *StateFile `json:"state"`
	CatalogVersion string     `json:"catalogVersion,omitempty"`
	GeneratedAt    string     `json:"generatedAt,omitempty"`
}

type UsageReport struct {
	Agents []string `json:"agents,omitempty"`
	Skills []string `json:"skills,omitempty"`
	Tools  []string `json:"tools,omitempty"`
}

type OrgView struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Role string `json:"role,omitempty"`
}

type OrgListResult struct {
	Orgs  []OrgView `json:"orgs"`
	Error string    `json:"error,omitempty"`
}

type Options struct {
	CatalogURL string
	HTTPClient *http.Client
}

type Service struct {
	baseDir    string
	catalogURL string
	platform   string
	httpClient *http.Client
}

type authConfig struct {
	Version      int    `json:"version"`
	Gateway      string `json:"gateway"`
	Token        string `json:"token"`
	UID          string `json:"uid"`
	DeploymentID string `json:"deploymentID"`
	OrgID        string `json:"orgID"`
	IdentityID   string `json:"identityID"`
	ConnectionID string `json:"connectionID"`
	AuthMethod   string `json:"authMethod"`
	AuthTime     string `json:"authTime"`
	ExpiresAt    string `json:"expiresAt"`
}

type marketplaceScope struct {
	OrgID string
}

func NewService(baseDir string, options Options) *Service {
	catalogURL := strings.TrimSpace(options.CatalogURL)
	if catalogURL == "" {
		catalogURL = strings.TrimSpace(os.Getenv("OPAGENT_MARKETPLACE_INDEX_URL"))
	}
	if catalogURL == "" {
		catalogURL = DefaultCatalogURL
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &Service{
		baseDir:    strings.TrimSpace(baseDir),
		catalogURL: catalogURL,
		platform:   detectPlatform(),
		httpClient: httpClient,
	}
}

func (s *Service) Reconcile(ctx context.Context) error {
	state, err := s.loadState()
	if err != nil {
		return err
	}
	catalog, err := s.fetchCatalog(ctx)
	if err != nil {
		missing := s.missingRequiredBuiltins()
		if len(missing) == 0 {
			return nil
		}
		return fmt.Errorf("marketplace catalog unavailable and required nodes are missing: %s: %w", strings.Join(missing, ", "), err)
	}
	state.LastCatalogVersion = strings.TrimSpace(catalog.Version)
	state.LastCatalogAt = strings.TrimSpace(catalog.GeneratedAt)

	catalogByKey := make(map[string]CatalogItem, len(catalog.Items))
	for _, item := range catalog.Items {
		if !isValidKind(item.Kind) {
			continue
		}
		if isRuntimeOnlyBuiltinItem(item.Kind, item.ID) {
			continue
		}
		catalogByKey[itemKey(item.Kind, item.ID)] = item
	}

	records := make(map[string]ManagedItemRecord, len(state.Items))
	for _, item := range state.Items {
		if !isPublicScope(scopeFromRecord(item)) {
			continue
		}
		if !isValidKind(item.Kind) {
			continue
		}
		if isRuntimeOnlyBuiltinItem(item.Kind, item.ID) {
			continue
		}
		item.InstallPath = InstallPath(s.baseDir, item.Kind, item.ID)
		records[itemKey(item.Kind, item.ID)] = item
	}

	targets := make([]BuiltinItem, 0, len(catalogByKey)+len(BuiltinManagedItems)+len(records))
	seenTargets := make(map[string]struct{})
	addTarget := func(item BuiltinItem) {
		key := itemKey(item.Kind, item.ID)
		if _, exists := seenTargets[key]; exists {
			return
		}
		seenTargets[key] = struct{}{}
		targets = append(targets, item)
	}
	for _, item := range catalogByKey {
		if !item.Builtin || !isPublicScope(scopeFromCatalogItem(item)) {
			continue
		}
		addTarget(BuiltinItem{Kind: item.Kind, ID: item.ID})
	}
	for _, item := range BuiltinManagedItems {
		addTarget(item)
	}
	for _, record := range records {
		if !record.Managed {
			continue
		}
		addTarget(BuiltinItem{Kind: record.Kind, ID: record.ID})
	}

	for _, target := range targets {
		key := itemKey(target.Kind, target.ID)
		record := records[key]
		record.Kind = target.Kind
		record.ID = target.ID
		record.Managed = true
		record.Builtin = record.Builtin || isBuiltinManagedItem(target.Kind, target.ID)
		record.InstallPath = InstallPath(s.baseDir, target.Kind, target.ID)

		catalogItem, ok := catalogByKey[key]
		if !ok {
			if installRootReady(record.InstallPath, target.Kind) {
				record.LastCheckedAt = time.Now().UnixMilli()
				record.UpdateAvailable = false
				records[key] = record
				continue
			}
			if target.Required {
				return fmt.Errorf("required marketplace item missing from catalog: %s/%s", target.Kind, target.ID)
			}
			continue
		}

		updatedRecord, _, reconcileErr := s.reconcileCatalogItem(ctx, catalogItem, record)
		if reconcileErr != nil {
			if installRootReady(record.InstallPath, target.Kind) {
				record.LastCheckedAt = time.Now().UnixMilli()
				record.SourceURL = assetURLForCatalogItem(catalogItem, s.platform)
				record.UpdateAvailable = true
				records[key] = record
				continue
			}
			if target.Required {
				return fmt.Errorf("failed to install required marketplace item %s/%s: %w", target.Kind, target.ID, reconcileErr)
			}
			continue
		}
		records[key] = updatedRecord
	}

	previousItems := state.Items
	state.Items = make([]ManagedItemRecord, 0, len(previousItems)+len(records))
	for _, record := range previousItems {
		if !isPublicScope(scopeFromRecord(record)) {
			state.Items = append(state.Items, record)
		}
	}
	for _, record := range records {
		state.Items = append(state.Items, record)
	}
	sort.Slice(state.Items, func(i, j int) bool {
		return scopedItemKey(scopeFromRecord(state.Items[i]), state.Items[i].Kind, state.Items[i].ID) < scopedItemKey(scopeFromRecord(state.Items[j]), state.Items[j].Kind, state.Items[j].ID)
	})
	return s.saveState(state)
}

func (s *Service) ListItems(ctx context.Context, force bool, usage UsageReport) (*ListResult, error) {
	return s.ListItemsForOrg(ctx, "", force, usage)
}

func (s *Service) ListItemsForOrg(ctx context.Context, orgID string, force bool, usage UsageReport) (*ListResult, error) {
	state, err := s.loadState()
	if err != nil {
		return nil, err
	}
	scope := marketplaceScope{OrgID: strings.TrimSpace(orgID)}
	catalog, fetchErr := s.fetchCatalogForScope(ctx, scope)
	if fetchErr == nil && isPublicScope(scope) {
		state.LastCatalogVersion = strings.TrimSpace(catalog.Version)
		state.LastCatalogAt = strings.TrimSpace(catalog.GeneratedAt)
		if err := s.saveState(state); err != nil {
			return nil, err
		}
	}
	return s.buildListResultForScope(state, catalog, scope, usage, fetchErr), nil
}

func (s *Service) Refresh(ctx context.Context, usage UsageReport) (*ListResult, error) {
	return s.ListItems(ctx, true, usage)
}

func (s *Service) RefreshForOrg(ctx context.Context, orgID string, usage UsageReport) (*ListResult, error) {
	return s.ListItemsForOrg(ctx, orgID, true, usage)
}

func (s *Service) ListOrgs(ctx context.Context) (*OrgListResult, error) {
	auth, err := s.loadAuthConfig()
	if err != nil {
		return &OrgListResult{Orgs: []OrgView{}, Error: err.Error()}, nil
	}
	url := strings.TrimRight(auth.Gateway, "/") + "/api/v1/user/orgs"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.Token)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return &OrgListResult{Orgs: []OrgView{}, Error: err.Error()}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &OrgListResult{Orgs: []OrgView{}, Error: fmt.Sprintf("org list request failed: %s", resp.Status)}, nil
	}
	var result OrgListResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Orgs == nil {
		result.Orgs = []OrgView{}
	}
	return &result, nil
}

func (s *Service) GetState(ctx context.Context) (*StateResult, error) {
	state, err := s.loadState()
	if err != nil {
		return nil, err
	}
	return &StateResult{
		State:          state,
		CatalogVersion: state.LastCatalogVersion,
		GeneratedAt:    state.LastCatalogAt,
	}, nil
}

func (s *Service) InstallItem(ctx context.Context, kind Kind, id string, usage UsageReport) (*ActionResult, error) {
	return s.InstallOrgItem(ctx, "", kind, id, usage)
}

func (s *Service) UpdateItem(ctx context.Context, kind Kind, id string, usage UsageReport) (*ActionResult, error) {
	return s.UpdateOrgItem(ctx, "", kind, id, usage)
}

func (s *Service) InstallOrgItem(ctx context.Context, orgID string, kind Kind, id string, usage UsageReport) (*ActionResult, error) {
	return s.installOrUpdateItem(ctx, marketplaceScope{OrgID: strings.TrimSpace(orgID)}, kind, id, usage)
}

func (s *Service) UpdateOrgItem(ctx context.Context, orgID string, kind Kind, id string, usage UsageReport) (*ActionResult, error) {
	return s.installOrUpdateItem(ctx, marketplaceScope{OrgID: strings.TrimSpace(orgID)}, kind, id, usage)
}

func (s *Service) installOrUpdateItem(ctx context.Context, scope marketplaceScope, kind Kind, id string, usage UsageReport) (*ActionResult, error) {
	if !isValidKind(kind) || strings.TrimSpace(id) == "" {
		return &ActionResult{Success: false, Error: "Marketplace kind and id are required"}, nil
	}
	if isPublicScope(scope) && isRuntimeOnlyBuiltinItem(kind, id) {
		return &ActionResult{Success: false, Error: "Runtime-only built-in item is managed by the runtime bundle"}, nil
	}
	catalog, err := s.fetchCatalogForScope(ctx, scope)
	if err != nil {
		return &ActionResult{Success: false, Error: err.Error()}, nil
	}
	state, err := s.loadState()
	if err != nil {
		return nil, err
	}
	if isPublicScope(scope) {
		state.LastCatalogVersion = strings.TrimSpace(catalog.Version)
		state.LastCatalogAt = strings.TrimSpace(catalog.GeneratedAt)
	}

	var catalogItem *CatalogItem
	for i := range catalog.Items {
		item := &catalog.Items[i]
		if item.Kind == kind && strings.TrimSpace(item.ID) == strings.TrimSpace(id) {
			catalogItem = item
			break
		}
	}
	if catalogItem == nil {
		return &ActionResult{Success: false, Error: fmt.Sprintf("Marketplace item not found: %s/%s", kind, id)}, nil
	}

	record := findStateRecordForScope(state, scope, kind, id)
	record.Kind = kind
	record.ID = strings.TrimSpace(id)
	record.Scope = scopeName(scope)
	record.OrgID = strings.TrimSpace(scope.OrgID)
	record.OrgName = strings.TrimSpace(catalogItem.OrgName)
	record.LocalName = allocateMarketplaceLocalName(state, record)
	record.Managed = true
	record.Builtin = isPublicScope(scope) && (catalogItem.Builtin || isBuiltinManagedItem(kind, id))
	record.InstallPath = installPathForLocalName(s.baseDir, kind, record.LocalName)

	updatedRecord, err := s.installCatalogItem(ctx, *catalogItem, record)
	if err != nil {
		return &ActionResult{Success: false, Error: err.Error()}, nil
	}
	state = upsertStateRecord(state, updatedRecord)
	if err := s.saveState(state); err != nil {
		return nil, err
	}
	item := s.buildItemResultForScope(state, catalog, scope, updatedRecord.Kind, updatedRecord.ID, usage)
	return &ActionResult{Success: true, Item: item}, nil
}

func InstallPath(baseDir string, kind Kind, id string) string {
	return InstallPathForScope(baseDir, kind, id, "")
}

func InstallPathForScope(baseDir string, kind Kind, id string, orgID string) string {
	return installPathForLocalName(baseDir, kind, marketplaceLocalName(id))
}

func installPathForLocalName(baseDir string, kind Kind, localName string) string {
	trimmedBaseDir := strings.TrimSpace(baseDir)
	trimmedLocalName := marketplaceLocalName(localName)
	if trimmedLocalName == "" {
		trimmedLocalName = "item"
	}
	switch kind {
	case KindAgent:
		return filepath.Join(trimmedBaseDir, "agents", trimmedLocalName)
	case KindSkill:
		return filepath.Join(trimmedBaseDir, "skills", trimmedLocalName)
	case KindTool:
		return filepath.Join(trimmedBaseDir, "tools", trimmedLocalName)
	default:
		return filepath.Join(trimmedBaseDir, trimmedLocalName)
	}
}

func StatePath(baseDir string) string {
	return filepath.Join(strings.TrimSpace(baseDir), "index", "marketplace-packages.json")
}

func LegacyStatePath(baseDir string) string {
	return filepath.Join(strings.TrimSpace(baseDir), "configs", "system", "marketplace.json")
}

func (s *Service) reconcileCatalogItem(ctx context.Context, item CatalogItem, record ManagedItemRecord) (ManagedItemRecord, bool, error) {
	asset, ok := resolveCatalogAsset(item, s.platform)
	if !ok {
		return record, false, fmt.Errorf("no marketplace asset for %s on platform %q", itemKey(item.Kind, item.ID), s.platform)
	}
	itemScope := scopeFromCatalogItem(item)
	record.Kind = item.Kind
	record.ID = item.ID
	record.Scope = scopeName(itemScope)
	record.OrgID = strings.TrimSpace(item.OrgID)
	record.OrgName = strings.TrimSpace(item.OrgName)
	record.LocalName = marketplaceLocalName(firstNonEmpty(record.LocalName, item.ID))
	record.Managed = true
	record.Builtin = isPublicScope(itemScope) && (item.Builtin || isBuiltinManagedItem(item.Kind, item.ID))
	record.InstallPath = installPathForLocalName(s.baseDir, item.Kind, record.LocalName)
	record.SourceURL = strings.TrimSpace(asset.URL)

	ready := installRootReady(record.InstallPath, item.Kind)
	needsInstall := !ready || strings.TrimSpace(record.InstalledVersion) != strings.TrimSpace(item.Version)
	if !needsInstall && item.Kind == KindTool {
		nextProjected, err := syncToolBinProjection(s.baseDir, record.InstallPath, record.ProjectedPaths)
		if err == nil {
			record.ProjectedPaths = nextProjected
		} else {
			needsInstall = true
		}
	}
	if !needsInstall {
		record.InstalledVersion = strings.TrimSpace(item.Version)
		record.LastCheckedAt = time.Now().UnixMilli()
		record.UpdateAvailable = false
		return record, false, nil
	}

	nextRecord, err := s.installCatalogItem(ctx, item, record)
	if err != nil {
		return record, false, err
	}
	return nextRecord, true, nil
}

func (s *Service) installCatalogItem(ctx context.Context, item CatalogItem, record ManagedItemRecord) (ManagedItemRecord, error) {
	asset, ok := resolveCatalogAsset(item, s.platform)
	if !ok {
		return record, fmt.Errorf("no marketplace asset for %s on platform %q", itemKey(item.Kind, item.ID), s.platform)
	}
	archivePath, err := s.downloadCatalogAsset(ctx, asset)
	if err != nil {
		return record, err
	}
	defer os.Remove(archivePath)

	extractDir, err := os.MkdirTemp("", "opagent-marketplace-extract-*")
	if err != nil {
		return record, err
	}
	defer os.RemoveAll(extractDir)

	if err := extractTarGz(archivePath, extractDir); err != nil {
		return record, err
	}
	packageRoot, err := resolvePackageRoot(extractDir, item.Kind)
	if err != nil {
		return record, err
	}
	record.LocalName = marketplaceLocalName(firstNonEmpty(record.LocalName, item.ID))
	installPath := installPathForLocalName(s.baseDir, item.Kind, record.LocalName)
	if err := replaceInstallDir(packageRoot, installPath); err != nil {
		return record, err
	}
	projectedPaths := record.ProjectedPaths
	itemScope := scopeFromCatalogItem(item)
	if item.Kind == KindTool && isPublicScope(itemScope) {
		projectedPaths, err = syncToolBinProjection(s.baseDir, installPath, record.ProjectedPaths)
		if err != nil {
			return record, err
		}
	} else {
		if err := common.RemoveProjectedPaths(record.ProjectedPaths); err != nil {
			return record, err
		}
		projectedPaths = nil
	}

	record.Kind = item.Kind
	record.ID = item.ID
	record.Scope = scopeName(itemScope)
	record.OrgID = strings.TrimSpace(item.OrgID)
	record.OrgName = strings.TrimSpace(item.OrgName)
	record.LocalName = marketplaceLocalName(firstNonEmpty(record.LocalName, item.ID))
	record.Managed = true
	record.Builtin = isPublicScope(itemScope) && (item.Builtin || isBuiltinManagedItem(item.Kind, item.ID))
	record.InstalledVersion = strings.TrimSpace(item.Version)
	record.SourceURL = strings.TrimSpace(asset.URL)
	record.InstallPath = installPath
	record.ProjectedPaths = projectedPaths
	record.LastCheckedAt = time.Now().UnixMilli()
	record.UpdateAvailable = false
	return record, nil
}

func (s *Service) buildListResult(state *StateFile, catalog *Catalog, usage UsageReport, fetchErr error) *ListResult {
	return s.buildListResultForScope(state, catalog, marketplaceScope{}, usage, fetchErr)
}

func (s *Service) buildListResultForScope(state *StateFile, catalog *Catalog, scope marketplaceScope, usage UsageReport, fetchErr error) *ListResult {
	items := make([]ListItem, 0)
	catalogItems := []CatalogItem{}
	if catalog != nil {
		catalogItems = catalog.Items
	}
	seen := make(map[string]struct{})
	inUseAgents := makeStringSet(usage.Agents)
	inUseSkills := makeStringSet(usage.Skills)
	inUseTools := makeStringSet(usage.Tools)

	for _, item := range catalogItems {
		itemScope := scopeFromCatalogItem(item)
		if !sameScope(itemScope, scope) {
			continue
		}
		if isPublicScope(itemScope) && isRuntimeOnlyBuiltinItem(item.Kind, item.ID) {
			continue
		}
		record := findStateRecordForScope(state, itemScope, item.Kind, item.ID)
		installPath := record.InstallPath
		if strings.TrimSpace(installPath) == "" {
			installPath = InstallPathForScope(s.baseDir, item.Kind, item.ID, itemScope.OrgID)
		}
		installed := installRootReady(installPath, item.Kind)
		updateAvailable := installed && recordManaged(record) && (record.UpdateAvailable ||
			(strings.TrimSpace(record.InstalledVersion) != "" &&
				strings.TrimSpace(item.Version) != "" &&
				strings.TrimSpace(record.InstalledVersion) != strings.TrimSpace(item.Version)))
		items = append(items, ListItem{
			ID:                 item.ID,
			Kind:               item.Kind,
			Scope:              scopeName(itemScope),
			OrgID:              strings.TrimSpace(item.OrgID),
			OrgName:            strings.TrimSpace(item.OrgName),
			LocalName:          record.LocalName,
			Name:               item.Name,
			Description:        item.Description,
			Builtin:            isPublicScope(itemScope) && (item.Builtin || record.Builtin),
			Version:            strings.TrimSpace(item.Version),
			InstalledVersion:   nullableInstalledVersion(record.InstalledVersion, installed),
			InstallPath:        installPath,
			SourceURL:          firstNonEmpty(record.SourceURL, assetURLForCatalogItem(item, s.platform)),
			Managed:            recordManaged(record),
			UpdateAvailable:    updateAvailable,
			InUse:              inUseForKind(item.Kind, item.ID, inUseAgents, inUseSkills, inUseTools),
			Status:             listStatus(installed, updateAvailable),
			MissingFromCatalog: false,
		})
		seen[scopedItemKey(itemScope, item.Kind, item.ID)] = struct{}{}
	}

	for _, record := range state.Items {
		recordScope := scopeFromRecord(record)
		if !sameScope(recordScope, scope) {
			continue
		}
		if isPublicScope(recordScope) && isRuntimeOnlyBuiltinItem(record.Kind, record.ID) {
			continue
		}
		key := scopedItemKey(recordScope, record.Kind, record.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		installed := installRootReady(record.InstallPath, record.Kind)
		items = append(items, ListItem{
			ID:                 record.ID,
			Kind:               record.Kind,
			Scope:              scopeName(recordScope),
			OrgID:              strings.TrimSpace(record.OrgID),
			OrgName:            strings.TrimSpace(record.OrgName),
			LocalName:          record.LocalName,
			Name:               toDisplayName(record.ID),
			Description:        "Installed marketplace item is missing from the catalog.",
			Builtin:            isPublicScope(recordScope) && record.Builtin,
			Version:            "",
			InstalledVersion:   nullableInstalledVersion(record.InstalledVersion, installed),
			InstallPath:        record.InstallPath,
			SourceURL:          record.SourceURL,
			Managed:            recordManaged(record),
			UpdateAvailable:    false,
			InUse:              inUseForKind(record.Kind, record.ID, inUseAgents, inUseSkills, inUseTools),
			Status:             listStatus(installed, false),
			MissingFromCatalog: true,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Builtin != items[j].Builtin {
			return items[i].Builtin
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	catalogVersion := strings.TrimSpace(state.LastCatalogVersion)
	generatedAt := strings.TrimSpace(state.LastCatalogAt)
	if !isPublicScope(scope) {
		catalogVersion = ""
		generatedAt = ""
	}
	result := &ListResult{Items: items, CatalogVersion: catalogVersion, GeneratedAt: generatedAt}
	if catalog != nil {
		result.CatalogVersion = strings.TrimSpace(catalog.Version)
		if strings.TrimSpace(catalog.GeneratedAt) != "" {
			result.GeneratedAt = strings.TrimSpace(catalog.GeneratedAt)
		}
	}
	if fetchErr != nil {
		result.Error = fetchErr.Error()
	}
	return result
}

func (s *Service) buildItemResult(state *StateFile, catalog *Catalog, kind Kind, id string, usage UsageReport) *ListItem {
	return s.buildItemResultForScope(state, catalog, marketplaceScope{}, kind, id, usage)
}

func (s *Service) buildItemResultForScope(state *StateFile, catalog *Catalog, scope marketplaceScope, kind Kind, id string, usage UsageReport) *ListItem {
	list := s.buildListResultForScope(state, catalog, scope, usage, nil)
	for i := range list.Items {
		item := &list.Items[i]
		if item.Kind == kind && item.ID == strings.TrimSpace(id) && sameScope(marketplaceScope{OrgID: item.OrgID}, scope) {
			copy := *item
			return &copy
		}
	}
	return nil
}

func (s *Service) fetchCatalog(ctx context.Context) (*Catalog, error) {
	return s.fetchCatalogForScope(ctx, marketplaceScope{})
}

func (s *Service) fetchCatalogForScope(ctx context.Context, scope marketplaceScope) (*Catalog, error) {
	if !isPublicScope(scope) {
		return s.fetchOrgCatalog(ctx, scope.OrgID)
	}
	if strings.TrimSpace(s.catalogURL) == "" {
		return nil, fmt.Errorf("marketplace catalog URL is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.catalogURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("marketplace catalog request failed: %s", resp.Status)
	}
	var catalog Catalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, err
	}
	if strings.TrimSpace(catalog.Version) == "" {
		return nil, fmt.Errorf("marketplace catalog missing version")
	}
	return &catalog, nil
}

func (s *Service) fetchOrgCatalog(ctx context.Context, orgID string) (*Catalog, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, fmt.Errorf("orgID is required")
	}
	auth, err := s.loadAuthConfig()
	if err != nil {
		return nil, err
	}
	if orgID != strings.TrimSpace(auth.OrgID) {
		return nil, fmt.Errorf("orgID must match the token-bound organization")
	}
	url := strings.TrimRight(auth.Gateway, "/") + "/api/v1/user/orgs/" + orgID + "/marketplace/catalog"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+auth.Token)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("org marketplace catalog request failed: %s", resp.Status)
	}
	var catalog Catalog
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		return nil, err
	}
	if strings.TrimSpace(catalog.Version) == "" {
		return nil, fmt.Errorf("org marketplace catalog missing version")
	}
	for i := range catalog.Items {
		catalog.Items[i].Scope = "org"
		catalog.Items[i].Builtin = false
		if strings.TrimSpace(catalog.Items[i].OrgID) == "" {
			catalog.Items[i].OrgID = orgID
		}
	}
	return &catalog, nil
}

func (s *Service) loadAuthConfig() (*authConfig, error) {
	path := filepath.Join(strings.TrimSpace(s.baseDir), "configs", "user", "auth.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read auth config: %w", err)
	}
	var cfg authConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse auth config: %w", err)
	}
	cfg.Gateway = strings.TrimSpace(cfg.Gateway)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.UID = strings.TrimSpace(cfg.UID)
	cfg.DeploymentID = strings.TrimSpace(cfg.DeploymentID)
	cfg.OrgID = strings.TrimSpace(cfg.OrgID)
	cfg.IdentityID = strings.TrimSpace(cfg.IdentityID)
	cfg.ConnectionID = strings.TrimSpace(cfg.ConnectionID)
	cfg.AuthMethod = strings.TrimSpace(cfg.AuthMethod)
	cfg.AuthTime = strings.TrimSpace(cfg.AuthTime)
	cfg.ExpiresAt = strings.TrimSpace(cfg.ExpiresAt)
	if cfg.Version != 2 || cfg.Gateway == "" || cfg.Token == "" || cfg.UID == "" ||
		cfg.DeploymentID == "" || cfg.OrgID == "" || cfg.IdentityID == "" ||
		cfg.ConnectionID == "" || cfg.AuthMethod == "" || cfg.AuthTime == "" ||
		cfg.ExpiresAt == "" {
		return nil, fmt.Errorf("tenant-bound auth config version 2 is required")
	}
	return &cfg, nil
}

func (s *Service) downloadCatalogAsset(ctx context.Context, asset Asset) (string, error) {
	url := strings.TrimSpace(asset.URL)
	expected := strings.ToLower(strings.TrimSpace(asset.SHA256))
	if url == "" || expected == "" {
		return "", fmt.Errorf("marketplace asset URL and sha256 are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("marketplace asset download failed: %s", resp.Status)
	}
	file, err := os.CreateTemp("", "opagent-marketplace-*.tar.gz")
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	writer := io.MultiWriter(file, hasher)
	if _, err := io.Copy(writer, resp.Body); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", err
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != expected {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("marketplace asset sha256 mismatch")
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func (s *Service) loadState() (*StateFile, error) {
	path := StatePath(s.baseDir)
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		legacyData, legacyErr := os.ReadFile(LegacyStatePath(s.baseDir))
		if legacyErr != nil {
			if errors.Is(legacyErr, os.ErrNotExist) {
				return &StateFile{Version: stateVersion, Items: []ManagedItemRecord{}}, nil
			}
			return nil, legacyErr
		}
		data = legacyData
		err = nil
	}
	if err != nil {
		return nil, err
	}
	var state StateFile
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	if state.Version == 0 {
		state.Version = stateVersion
	}
	for i := range state.Items {
		state.Items[i].Scope = scopeName(scopeFromRecord(state.Items[i]))
		state.Items[i].LocalName = marketplaceLocalName(firstNonEmpty(state.Items[i].LocalName, state.Items[i].ID))
		state.Items[i].InstallPath = installPathForLocalName(s.baseDir, state.Items[i].Kind, state.Items[i].LocalName)
	}
	return &state, nil
}

func (s *Service) saveState(state *StateFile) error {
	state.Version = stateVersion
	return writeJSONAtomic(StatePath(s.baseDir), state)
}

func (s *Service) missingRequiredBuiltins() []string {
	missing := make([]string, 0)
	for _, item := range BuiltinManagedItems {
		if !item.Required {
			continue
		}
		installPath := InstallPath(s.baseDir, item.Kind, item.ID)
		if installRootReady(installPath, item.Kind) {
			continue
		}
		missing = append(missing, fmt.Sprintf("%s/%s", item.Kind, item.ID))
	}
	return missing
}

func detectPlatform() string {
	switch goruntime.GOOS + "/" + goruntime.GOARCH {
	case "darwin/amd64":
		return "darwin-amd64"
	case "darwin/arm64":
		return "darwin-arm64"
	case "linux/amd64":
		return "linux-amd64"
	case "linux/arm64":
		return "linux-arm64"
	case "windows/amd64":
		return "windows-amd64"
	case "windows/arm64":
		return "windows-arm64"
	default:
		return ""
	}
}

func RequiredMarker(kind Kind) string {
	switch kind {
	case KindAgent:
		return filepath.Join(".agent", "AGENT.md")
	case KindSkill:
		return "SKILL.md"
	case KindTool:
		return common.ToolManifestName
	default:
		return ""
	}
}

func itemKey(kind Kind, id string) string {
	return string(kind) + ":" + strings.TrimSpace(id)
}

func isValidKind(kind Kind) bool {
	switch kind {
	case KindAgent, KindSkill, KindTool:
		return true
	default:
		return false
	}
}

func isBuiltinManagedItem(kind Kind, id string) bool {
	for _, item := range BuiltinManagedItems {
		if item.Kind == kind && strings.TrimSpace(item.ID) == strings.TrimSpace(id) {
			return true
		}
	}
	return false
}

func isRuntimeOnlyBuiltinItem(kind Kind, id string) bool {
	for _, item := range runtimeOnlyBuiltinItems {
		if item.Kind == kind && strings.TrimSpace(item.ID) == strings.TrimSpace(id) {
			return true
		}
	}
	return false
}

func resolveCatalogAsset(item CatalogItem, platform string) (Asset, bool) {
	if asset, ok := item.Assets[strings.TrimSpace(platform)]; ok {
		if strings.TrimSpace(asset.URL) != "" && strings.TrimSpace(asset.SHA256) != "" {
			return asset, true
		}
	}
	if asset, ok := item.Assets["any"]; ok {
		if strings.TrimSpace(asset.URL) != "" && strings.TrimSpace(asset.SHA256) != "" {
			return asset, true
		}
	}
	return Asset{}, false
}

func assetURLForCatalogItem(item CatalogItem, platform string) string {
	asset, ok := resolveCatalogAsset(item, platform)
	if !ok {
		return ""
	}
	return strings.TrimSpace(asset.URL)
}

func installRootReady(installPath string, kind Kind) bool {
	marker := RequiredMarker(kind)
	if marker == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(strings.TrimSpace(installPath), marker))
	return err == nil
}

func extractTarGz(archivePath, extractDir string) error {
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
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := strings.TrimSpace(header.Name)
		if name == "" {
			continue
		}
		targetPath := filepath.Join(extractDir, filepath.Clean(name))
		if !pathWithinRoot(extractDir, targetPath) {
			return fmt.Errorf("tar entry escapes extraction root: %s", name)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(file, reader); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, targetPath); err != nil {
				return err
			}
		}
	}
}

func resolvePackageRoot(extractDir string, kind Kind) (string, error) {
	marker := RequiredMarker(kind)
	if marker == "" {
		return "", fmt.Errorf("unsupported marketplace kind: %s", kind)
	}
	if installRootReady(extractDir, kind) {
		return extractDir, nil
	}
	entries, err := os.ReadDir(extractDir)
	if err != nil {
		return "", err
	}
	dirEntries := make([]os.DirEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirEntries = append(dirEntries, entry)
		}
	}
	if len(entries) == 1 && len(dirEntries) == 1 {
		candidate := filepath.Join(extractDir, dirEntries[0].Name())
		if installRootReady(candidate, kind) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("marketplace package is missing %s", marker)
}

func replaceInstallDir(sourceDir, destinationDir string) error {
	parentDir := filepath.Dir(destinationDir)
	entryName := filepath.Base(destinationDir)
	stagingDir := filepath.Join(parentDir, "."+entryName+".marketplace-staging-"+fmt.Sprintf("%d", time.Now().UnixNano()))
	backupDir := filepath.Join(parentDir, "."+entryName+".marketplace-backup-"+fmt.Sprintf("%d", time.Now().UnixNano()))
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return err
	}
	if err := copyTree(sourceDir, stagingDir); err != nil {
		return err
	}
	if _, err := os.Stat(destinationDir); errors.Is(err, os.ErrNotExist) {
		return os.Rename(stagingDir, destinationDir)
	}
	if err := os.Rename(destinationDir, backupDir); err != nil {
		_ = os.RemoveAll(stagingDir)
		return err
	}
	if err := os.Rename(stagingDir, destinationDir); err != nil {
		_ = os.Rename(backupDir, destinationDir)
		_ = os.RemoveAll(stagingDir)
		return err
	}
	return os.RemoveAll(backupDir)
}

func copyTree(sourceDir, destinationDir string) error {
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		targetPath := destinationDir
		if rel != "." {
			targetPath = filepath.Join(destinationDir, rel)
		}
		mode := info.Mode()
		switch {
		case info.IsDir():
			return os.MkdirAll(targetPath, mode.Perm())
		case mode&os.ModeSymlink != 0:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return err
			}
			linkTarget, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, targetPath)
		default:
			return copyFileWithMode(path, targetPath, mode.Perm())
		}
	})
}

func copyFileWithMode(sourcePath, destinationPath string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	tempPath := destinationPath + fmt.Sprintf(".%d.tmp", time.Now().UnixNano())
	target, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(target, source); err != nil {
		_ = target.Close()
		_ = os.Remove(tempPath)
		return err
	}
	if err := target.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return os.Rename(tempPath, destinationPath)
}

func syncToolBinProjection(baseDir, installPath string, previousPaths []string) ([]string, error) {
	return common.SyncToolBinProjection(baseDir, installPath, previousPaths)
}

func writeJSONAtomic(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tempPath := path + fmt.Sprintf(".%d.tmp", time.Now().UnixNano())
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tempPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tempPath, path)
}

func pathWithinRoot(root, target string) bool {
	resolvedRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return false
	}
	resolvedTarget, err := filepath.Abs(strings.TrimSpace(target))
	if err != nil {
		return false
	}
	if resolvedRoot == resolvedTarget {
		return true
	}
	return strings.HasPrefix(resolvedTarget, resolvedRoot+string(os.PathSeparator))
}

func makeStringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{})
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	return set
}

func inUseForKind(kind Kind, id string, agents, skills, tools map[string]struct{}) bool {
	id = strings.TrimSpace(id)
	switch kind {
	case KindAgent:
		_, ok := agents[id]
		return ok
	case KindSkill:
		_, ok := skills[id]
		return ok
	case KindTool:
		_, ok := tools[id]
		return ok
	default:
		return false
	}
}

func listStatus(installed bool, updateAvailable bool) ItemStatus {
	if !installed {
		return StatusNotInstalled
	}
	if updateAvailable {
		return StatusUpdateAvail
	}
	return StatusInstalled
}

func nullableInstalledVersion(version string, installed bool) string {
	if !installed {
		return ""
	}
	return strings.TrimSpace(version)
}

func toDisplayName(id string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(id), func(r rune) bool {
		return r == '-' || r == '_'
	})
	if len(parts) == 0 {
		return strings.TrimSpace(id)
	}
	next := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		next = append(next, strings.ToUpper(part[:1])+part[1:])
	}
	return strings.Join(next, " ")
}

func findStateRecord(state *StateFile, kind Kind, id string) ManagedItemRecord {
	return findStateRecordForScope(state, marketplaceScope{}, kind, id)
}

func findStateRecordForScope(state *StateFile, scope marketplaceScope, kind Kind, id string) ManagedItemRecord {
	for _, item := range state.Items {
		if item.Kind == kind && strings.TrimSpace(item.ID) == strings.TrimSpace(id) && sameScope(scopeFromRecord(item), scope) {
			return item
		}
	}
	return ManagedItemRecord{Kind: kind, ID: strings.TrimSpace(id), Scope: scopeName(scope), OrgID: strings.TrimSpace(scope.OrgID), Managed: true}
}

func allocateMarketplaceLocalName(state *StateFile, record ManagedItemRecord) string {
	baseName := marketplaceLocalName(firstNonEmpty(record.LocalName, record.ID))
	if baseName == "" {
		baseName = "item"
	}
	if marketplaceLocalNameAvailable(state, record, baseName) {
		return baseName
	}
	if strings.TrimSpace(record.OrgID) != "" {
		orgName := marketplaceLocalName(strings.TrimSpace(record.OrgID) + "-" + baseName)
		if marketplaceLocalNameAvailable(state, record, orgName) {
			return orgName
		}
	}
	for i := 2; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", baseName, i)
		if marketplaceLocalNameAvailable(state, record, candidate) {
			return candidate
		}
	}
	return fmt.Sprintf("%s-%d", baseName, time.Now().UnixNano())
}

func marketplaceLocalNameAvailable(state *StateFile, record ManagedItemRecord, localName string) bool {
	localName = marketplaceLocalName(localName)
	if localName == "" {
		return false
	}
	if state == nil {
		return true
	}
	for _, item := range state.Items {
		if item.Kind != record.Kind {
			continue
		}
		if strings.TrimSpace(item.ID) == strings.TrimSpace(record.ID) && sameScope(scopeFromRecord(item), scopeFromRecord(record)) {
			continue
		}
		existing := marketplaceLocalName(firstNonEmpty(item.LocalName, item.ID))
		if existing == localName {
			return false
		}
	}
	return true
}

func marketplaceLocalName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-' || r == '_' || r == '.' || r == ' ' || r == '/':
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func upsertStateRecord(state *StateFile, record ManagedItemRecord) *StateFile {
	nextItems := make([]ManagedItemRecord, 0, len(state.Items)+1)
	for _, item := range state.Items {
		if item.Kind == record.Kind && item.ID == record.ID && sameScope(scopeFromRecord(item), scopeFromRecord(record)) {
			continue
		}
		nextItems = append(nextItems, item)
	}
	nextItems = append(nextItems, record)
	sort.Slice(nextItems, func(i, j int) bool {
		leftScope := scopeName(scopeFromRecord(nextItems[i]))
		rightScope := scopeName(scopeFromRecord(nextItems[j]))
		if leftScope != rightScope {
			return leftScope < rightScope
		}
		if nextItems[i].OrgID != nextItems[j].OrgID {
			return nextItems[i].OrgID < nextItems[j].OrgID
		}
		if nextItems[i].Kind != nextItems[j].Kind {
			return nextItems[i].Kind < nextItems[j].Kind
		}
		return nextItems[i].ID < nextItems[j].ID
	})
	state.Items = nextItems
	return state
}

func recordManaged(record ManagedItemRecord) bool {
	return record.Managed != false
}

func scopeFromCatalogItem(item CatalogItem) marketplaceScope {
	if strings.EqualFold(strings.TrimSpace(item.Scope), "org") || strings.TrimSpace(item.OrgID) != "" {
		return marketplaceScope{OrgID: strings.TrimSpace(item.OrgID)}
	}
	return marketplaceScope{}
}

func scopeFromRecord(record ManagedItemRecord) marketplaceScope {
	if strings.EqualFold(strings.TrimSpace(record.Scope), "org") || strings.TrimSpace(record.OrgID) != "" {
		return marketplaceScope{OrgID: strings.TrimSpace(record.OrgID)}
	}
	return marketplaceScope{}
}

func scopeName(scope marketplaceScope) string {
	if strings.TrimSpace(scope.OrgID) != "" {
		return "org"
	}
	return "public"
}

func isPublicScope(scope marketplaceScope) bool {
	return strings.TrimSpace(scope.OrgID) == ""
}

func sameScope(a, b marketplaceScope) bool {
	return strings.TrimSpace(a.OrgID) == strings.TrimSpace(b.OrgID)
}

func scopedItemKey(scope marketplaceScope, kind Kind, id string) string {
	return scopeName(scope) + ":" + strings.TrimSpace(scope.OrgID) + ":" + itemKey(kind, id)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
