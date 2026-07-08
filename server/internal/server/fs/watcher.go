package fs

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchSubscription represents a watch subscription
type WatchSubscription struct {
	ID        string
	Path      string
	Recursive bool
	Excludes  []string
	Client    interface{} // Reference to the client (opaque to avoid circular imports)
}

// Watcher manages file system watching with event coalescing
// Reference: VS Code's ParcelWatcher and NodeJSFileWatcherLibrary
type Watcher struct {
	watcher       *fsnotify.Watcher
	subscriptions map[string]*WatchSubscription
	pathToSubs    map[string][]string // path -> subscription IDs
	subWatchPaths map[string][]string
	coalescer     *EventCoalescer
	worker        *ThrottledWorker
	mu            sync.RWMutex
	verbose       bool

	// Callback for notifying clients
	onFileChange func(subID string, changes []FileChange)

	// Aggregation timer
	aggregateTimer *time.Timer
	pendingEvents  []fsnotify.Event
	pendingMu      sync.Mutex

	stopChan chan struct{}
}

// NewWatcher creates a new file watcher
func NewWatcher(verbose bool, onFileChange func(subID string, changes []FileChange)) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	w := &Watcher{
		watcher:       fsWatcher,
		subscriptions: make(map[string]*WatchSubscription),
		pathToSubs:    make(map[string][]string),
		subWatchPaths: make(map[string][]string),
		coalescer:     NewEventCoalescer(),
		verbose:       verbose,
		onFileChange:  onFileChange,
		pendingEvents: make([]fsnotify.Event, 0),
		stopChan:      make(chan struct{}),
	}

	// Create throttled worker for processing coalesced events
	w.worker = NewThrottledWorker(w.handleCoalescedEvents)

	// Start event processing goroutine
	go w.eventLoop()

	return w, nil
}

// Watch adds a new watch subscription
func (w *Watcher) Watch(sub *WatchSubscription) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Normalize path
	absPath, err := filepath.Abs(sub.Path)
	if err != nil {
		return err
	}
	sub.Path = absPath

	// Store subscription
	w.subscriptions[sub.ID] = sub

	paths, err := w.watchPaths(sub)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if _, ok := w.pathToSubs[path]; !ok {
			w.pathToSubs[path] = make([]string, 0)
			if err := w.watcher.Add(path); err != nil {
				for _, added := range paths[:indexOfPath(paths, path)] {
					w.removePathSubscription(added, sub.ID)
				}
				delete(w.subscriptions, sub.ID)
				return err
			}
		}
		w.pathToSubs[path] = append(w.pathToSubs[path], sub.ID)
	}
	w.subWatchPaths[sub.ID] = paths

	if w.verbose {
		log.Printf("Watch added: %s (id=%s)", absPath, sub.ID)
	}

	return nil
}

// Unwatch removes a watch subscription
func (w *Watcher) Unwatch(subID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	sub, ok := w.subscriptions[subID]
	if !ok {
		return nil // Already removed
	}

	// Remove from subscriptions
	delete(w.subscriptions, subID)

	for _, path := range w.subWatchPaths[subID] {
		w.removePathSubscription(path, subID)
	}
	delete(w.subWatchPaths, subID)

	if w.verbose {
		log.Printf("Watch removed: %s (id=%s)", sub.Path, subID)
	}

	return nil
}

func (w *Watcher) watchPaths(sub *WatchSubscription) ([]string, error) {
	if !sub.Recursive {
		return []string{sub.Path}, nil
	}
	paths := make([]string, 0, 64)
	err := filepath.WalkDir(sub.Path, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		if path != sub.Path && isExcludedWatchPath(path, sub.Path, sub.Excludes) {
			return filepath.SkipDir
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return []string{sub.Path}, nil
	}
	return paths, nil
}

func (w *Watcher) removePathSubscription(path string, subID string) {
	subs, ok := w.pathToSubs[path]
	if !ok {
		return
	}
	next := make([]string, 0, len(subs)-1)
	for _, id := range subs {
		if id != subID {
			next = append(next, id)
		}
	}
	if len(next) == 0 {
		delete(w.pathToSubs, path)
		_ = w.watcher.Remove(path)
		return
	}
	w.pathToSubs[path] = next
}

func indexOfPath(paths []string, target string) int {
	for index, path := range paths {
		if path == target {
			return index
		}
	}
	return len(paths)
}

// UnwatchClient removes all watches for a client
func (w *Watcher) UnwatchClient(client interface{}) {
	w.mu.Lock()
	toRemove := make([]string, 0)
	for id, sub := range w.subscriptions {
		if sub.Client == client {
			toRemove = append(toRemove, id)
		}
	}
	w.mu.Unlock()

	for _, id := range toRemove {
		w.Unwatch(id)
	}
}

// eventLoop processes fsnotify events
func (w *Watcher) eventLoop() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			if w.verbose {
				log.Printf("Watcher error: %v", err)
			}

		case <-w.stopChan:
			return
		}
	}
}

// handleEvent processes a single fsnotify event with aggregation
func (w *Watcher) handleEvent(event fsnotify.Event) {
	if event.Op&fsnotify.Create != 0 {
		w.addRecursiveWatchForCreatedDir(event.Name)
	}

	w.pendingMu.Lock()
	defer w.pendingMu.Unlock()

	w.pendingEvents = append(w.pendingEvents, event)

	// Reset or start aggregation timer
	if w.aggregateTimer != nil {
		w.aggregateTimer.Stop()
	}

	// Use longer delay for delete events (atomic save handling)
	delay := FileChangesHandlerDelay
	if event.Op&fsnotify.Remove != 0 {
		delay = FileDeleteHandlerDelay
	}

	w.aggregateTimer = time.AfterFunc(delay, w.flushPendingEvents)
}

func (w *Watcher) addRecursiveWatchForCreatedDir(path string) {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, sub := range w.subscriptions {
		if !sub.Recursive || !isParentOrSame(absPath, sub.Path) || isExcludedWatchPath(absPath, sub.Path, sub.Excludes) {
			continue
		}
		if w.pathToSubs[absPath] == nil {
			if err := w.watcher.Add(absPath); err != nil {
				continue
			}
		}
		if !containsString(w.pathToSubs[absPath], sub.ID) {
			w.pathToSubs[absPath] = append(w.pathToSubs[absPath], sub.ID)
			w.subWatchPaths[sub.ID] = append(w.subWatchPaths[sub.ID], absPath)
		}
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// flushPendingEvents processes all pending events through coalescer
func (w *Watcher) flushPendingEvents() {
	w.pendingMu.Lock()
	events := w.pendingEvents
	w.pendingEvents = make([]fsnotify.Event, 0)
	w.pendingMu.Unlock()

	if len(events) == 0 {
		return
	}

	// Convert fsnotify events to FileChange and process through coalescer
	for _, event := range events {
		change := w.toFileChange(event)
		if change != nil {
			w.coalescer.ProcessEvent(*change)
		}
	}

	// Get coalesced events
	coalesced := w.coalescer.Coalesce()

	if len(coalesced) > 0 {
		// Send to throttled worker
		w.worker.Add(coalesced)
	}
}

// toFileChange converts fsnotify.Event to FileChange
func (w *Watcher) toFileChange(event fsnotify.Event) *FileChange {
	var changeType FileChangeType

	switch {
	case event.Op&fsnotify.Create != 0:
		changeType = FileChangeCreated
	case event.Op&fsnotify.Write != 0:
		changeType = FileChangeChanged
	case event.Op&fsnotify.Remove != 0:
		changeType = FileChangeDeleted
	case event.Op&fsnotify.Rename != 0:
		changeType = FileChangeDeleted // Rename source is treated as delete
	case event.Op&fsnotify.Chmod != 0:
		changeType = FileChangeChanged
	default:
		return nil
	}

	return &FileChange{
		Type: changeType,
		Path: event.Name,
	}
}

// handleCoalescedEvents is called by ThrottledWorker with batched events
func (w *Watcher) handleCoalescedEvents(changes []FileChange) {
	if w.onFileChange == nil {
		return
	}

	type pendingNotification struct {
		subID   string
		changes []FileChange
	}

	// Only snapshot watcher state while holding w.mu. The callback may unregister
	// the client and re-enter Watcher, so invoking it under w.mu would invert the
	// Watcher -> Server lock order and can deadlock the whole WS server.
	pending := make([]pendingNotification, 0)
	w.mu.RLock()

	// Build map of subscription to relevant changes
	for subID, sub := range w.subscriptions {
		relevant := make([]FileChange, 0)
		for _, change := range changes {
			if isParentOrSame(change.Path, sub.Path) && !isExcludedWatchPath(change.Path, sub.Path, sub.Excludes) {
				relevant = append(relevant, change)
			}
		}
		if len(relevant) > 0 {
			pending = append(pending, pendingNotification{subID: subID, changes: relevant})
		}
	}
	w.mu.RUnlock()

	for _, notification := range pending {
		w.onFileChange(notification.subID, notification.changes)
	}
}

func isExcludedWatchPath(path string, root string, excludes []string) bool {
	if len(excludes) == 0 {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	rel = filepath.ToSlash(rel)
	base := filepath.Base(path)
	for _, exclude := range excludes {
		pattern := strings.TrimSpace(filepath.ToSlash(exclude))
		if pattern == "" {
			continue
		}
		if pattern == base || pattern == rel || strings.HasPrefix(rel, strings.TrimSuffix(pattern, "/")+"/") {
			return true
		}
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
	}
	return false
}

func isParentOrSame(path, parent string) bool {
	if path == parent {
		return true
	}
	if !strings.HasSuffix(parent, string(filepath.Separator)) {
		parent += string(filepath.Separator)
	}
	return strings.HasPrefix(path, parent)
}

// Close stops the watcher
func (w *Watcher) Close() error {
	close(w.stopChan)
	return w.watcher.Close()
}
