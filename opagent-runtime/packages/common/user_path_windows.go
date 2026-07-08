//go:build windows

package common

import (
	"golang.org/x/sys/windows/registry"
)

func EnsureUserPathContains(dir string) error {
	entry := userPathEntryForDir(dir)
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	current, _, err := key.GetStringValue("Path")
	if err != nil && err != registry.ErrNotExist {
		return err
	}
	next := appendUniquePathValue(current, entry, samePathEntry)
	if next == current {
		return nil
	}
	return key.SetExpandStringValue("Path", next)
}

func pathEntriesCaseInsensitive() bool {
	return true
}

func homeVariable() string {
	return `%USERPROFILE%`
}
