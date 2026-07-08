package config

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestNormalizeHostName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "My-Host.EXAMPLE.com", want: "my-host"},
		{in: "host___name", want: "host-name"},
		{in: "----", want: "host"},
		{in: "", want: "host"},
	}

	for _, tc := range cases {
		got := normalizeHostName(tc.in)
		if got != tc.want {
			t.Fatalf("normalizeHostName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestEnsureHostIDPersists(t *testing.T) {
	sysDir := t.TempDir()

	id1, err := ensureHostID(sysDir)
	if err != nil {
		t.Fatalf("ensureHostID(first) error: %v", err)
	}
	if id1 == "" {
		t.Fatal("ensureHostID(first) returned empty id")
	}

	id2, err := ensureHostID(sysDir)
	if err != nil {
		t.Fatalf("ensureHostID(second) error: %v", err)
	}
	if id2 != id1 {
		t.Fatalf("host id should persist, first=%q second=%q", id1, id2)
	}

	pattern := regexp.MustCompile(`^[a-z0-9-]+-[a-z0-9]{4}$`)
	if !pattern.MatchString(id1) {
		t.Fatalf("host id %q does not match expected format", id1)
	}
}

func TestEnsureHostIDMigratesLegacyInstanceID(t *testing.T) {
	sysDir := t.TempDir()
	legacy := "legacy-instance-id-1234"
	if err := os.WriteFile(filepath.Join(sysDir, "instance_id"), []byte(legacy+"\n"), 0o600); err != nil {
		t.Fatalf("write legacy instance_id: %v", err)
	}

	id, err := ensureHostID(sysDir)
	if err != nil {
		t.Fatalf("ensureHostID() error: %v", err)
	}

	expectedSuffix := readLegacyInstanceSuffix(sysDir)
	if expectedSuffix == "" {
		t.Fatal("expected non-empty suffix from legacy id")
	}
	if len(id) < len(expectedSuffix)+2 {
		t.Fatalf("host id too short: %q", id)
	}
	gotSuffix := id[len(id)-len(expectedSuffix):]
	if gotSuffix != expectedSuffix {
		t.Fatalf("legacy-derived suffix mismatch: got %q want %q (id=%q)", gotSuffix, expectedSuffix, id)
	}
}
