package node

// type sysToolCacheEntry struct {
// 	NodeID string
// 	Spec   op.ToolSpec
// }

// var (
// 	sysToolCache   map[string]sysToolCacheEntry // tool name → entry
// 	sysToolCacheMu sync.RWMutex
// )

// // setSysToolsFromNode writes or updates the systool cache for one system tools node.
// func setSysToolsFromNode(nodeID string, specs []op.ToolSpec) {
// 	if nodeID == "" || len(specs) == 0 {
// 		return
// 	}
// 	sysToolCacheMu.Lock()
// 	defer sysToolCacheMu.Unlock()
// 	if sysToolCache == nil {
// 		sysToolCache = make(map[string]sysToolCacheEntry)
// 	}
// 	for _, s := range specs {
// 		name := strings.TrimSpace(s.Name)
// 		if name == "" {
// 			continue
// 		}
// 		sysToolCache[name] = sysToolCacheEntry{NodeID: nodeID, Spec: s}
// 	}
// }

// // getSysToolCache returns a copy of the current systool cache.
// func getSysToolCache() map[string]sysToolCacheEntry {
// 	sysToolCacheMu.RLock()
// 	defer sysToolCacheMu.RUnlock()
// 	if sysToolCache == nil {
// 		return nil
// 	}
// 	cp := make(map[string]sysToolCacheEntry, len(sysToolCache))
// 	for k, v := range sysToolCache {
// 		cp[k] = v
// 	}
// 	return cp
// }
