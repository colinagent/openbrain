package protocol

const (
	MethodGitBranches = "git/branches"
	MethodGitCheckout = "git/checkout"
)

type GitBranchesParams struct {
	Path string `json:"path"`
}

type GitBranch struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

type GitDirtySummary struct {
	ChangedFiles int  `json:"changedFiles"`
	AddedLines   int  `json:"addedLines"`
	DeletedLines int  `json:"deletedLines"`
	HasChanges   bool `json:"hasChanges"`
}

type GitBranchesResult struct {
	IsRepo        bool             `json:"isRepo"`
	RepoRoot      string           `json:"repoRoot,omitempty"`
	CurrentBranch string           `json:"currentBranch,omitempty"`
	Detached      bool             `json:"detached,omitempty"`
	DetachedLabel string           `json:"detachedLabel,omitempty"`
	Branches      []GitBranch      `json:"branches,omitempty"`
	Dirty         *GitDirtySummary `json:"dirty,omitempty"`
}

type GitCheckoutParams struct {
	Path   string `json:"path"`
	Branch string `json:"branch"`
	Create bool   `json:"create,omitempty"`
}

type GitCheckoutResult struct {
	RepoRoot      string `json:"repoRoot"`
	CurrentBranch string `json:"currentBranch"`
}
