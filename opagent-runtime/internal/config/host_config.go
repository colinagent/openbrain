package config

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type HostInfoConfig struct {
	HostID   string
	HostName string
	Ips      []string
}

// GetHostInfo returns host info using sysDir for host_id persistence.
// Use this when building system config before SetSystem has been called.
func GetHostInfo(sysDir string) *HostInfoConfig {
	hostID, err := ensureHostID(sysDir)
	if err != nil {
		hostID = ""
	}
	return &HostInfoConfig{
		HostID:   hostID,
		HostName: GetHostName(),
		Ips:      GetIps(),
	}
}

func GetIps() []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}

	ips := make([]string, 0, len(addrs))
	seen := make(map[string]struct{}, len(addrs))

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP == nil {
			continue
		}
		ip := ipNet.IP
		if ip.IsLoopback() {
			continue
		}
		if v4 := ip.To4(); v4 != nil {
			key := v4.String()
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			ips = append(ips, key)
			continue
		}
		if ip.To16() != nil {
			key := ip.String()
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			ips = append(ips, key)
		}
	}

	sort.Strings(ips)
	return ips
}

func GetHostName() string {
	hostName, err := os.Hostname()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(hostName)
}

func ensureHostID(sysDir string) (string, error) {
	if strings.TrimSpace(sysDir) == "" {
		return "", errors.New("sysDir is required")
	}

	path := filepath.Join(sysDir, "host_id")

	if raw, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(raw))
		if id != "" {
			return id, nil
		}
	}

	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		return "", err
	}

	hostName := normalizeHostName(GetHostName())
	suffix := readLegacyInstanceSuffix(sysDir)
	if suffix == "" {
		var err error
		suffix, err = randomSuffix(4)
		if err != nil {
			return "", err
		}
	}
	id := hostName + "-" + suffix

	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

func normalizeHostName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	if idx := strings.Index(name, "."); idx > 0 {
		name = name[:idx]
	}
	if name == "" {
		return "host"
	}

	var b strings.Builder
	prevDash := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			prevDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == '-':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		default:
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}

	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "host"
	}
	if len(out) > 12 {
		out = strings.Trim(out[:12], "-")
		if out == "" {
			out = "host"
		}
	}
	return out
}

func readLegacyInstanceSuffix(sysDir string) string {
	legacyPath := filepath.Join(sysDir, "instance_id")
	raw, err := os.ReadFile(legacyPath)
	if err != nil {
		return ""
	}
	legacy := strings.TrimSpace(string(raw))
	if legacy == "" {
		return ""
	}
	sum := sha1.Sum([]byte(legacy))
	encoded := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
	if len(encoded) < 4 {
		return encoded
	}
	return encoded[:4]
}

func randomSuffix(length int) (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	if length <= 0 {
		return "", errors.New("suffix length must be > 0")
	}

	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}

	out := make([]byte, length)
	for i := range buf {
		out[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(out), nil
}
