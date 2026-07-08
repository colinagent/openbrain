package core

import "github.com/colinagent/openbrain/opagent-runtime/internal/memory/objectstore"

var defaultObjectStore objectstore.Store

func SetDefaultObjectStore(store objectstore.Store) {
	defaultObjectStore = store
}

func GetObjectStore() objectstore.Store {
	return defaultObjectStore
}
