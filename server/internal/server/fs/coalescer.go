package fs

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// Reference: VS Code watcher.ts EventCoalescer

// FileChangeType represents the type of file change
type FileChangeType int

const (
	FileChangeCreated FileChangeType = iota
	FileChangeChanged
	FileChangeDeleted
)

func (t FileChangeType) String() string {
	switch t {
	case FileChangeCreated:
		return "created"
	case FileChangeChanged:
		return "changed"
	case FileChangeDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// FileChange represents a single file change event
type FileChange struct {
	Type FileChangeType
	Path string
}

// Timing constants (reference: VS Code nodejsWatcherLib.ts)
const (
	// FileChangesHandlerDelay is the delay for collecting file changes before coalescing
	// Same as VS Code: 75ms
	FileChangesHandlerDelay = 75 * time.Millisecond

	// FileDeleteHandlerDelay is extra delay for DELETE events to handle atomic save
	// Same as VS Code: 100ms
	FileDeleteHandlerDelay = 100 * time.Millisecond
)

// EventCoalescer coalesces file change events
// Reference: VS Code watcher.ts EventCoalescer class
type EventCoalescer struct {
	events  map[string]*FileChange
	mu      sync.Mutex
	isLinux bool
}

// NewEventCoalescer creates a new event coalescer
func NewEventCoalescer() *EventCoalescer {
	return &EventCoalescer{
		events:  make(map[string]*FileChange),
		isLinux: runtime.GOOS == "linux",
	}
}

// toKey returns the map key for an event (case-insensitive on Windows/macOS)
func (c *EventCoalescer) toKey(path string) string {
	if c.isLinux {
		return path
	}
	return strings.ToLower(path)
}

// ProcessEvent processes a single file change event
// Implements VS Code's coalescing logic:
// - CREATE + DELETE → ignore (nothing changed)
// - DELETE + CREATE → CHANGE (atomic save scenario)
// - CREATE + CHANGE → CREATE
// - CHANGE + CHANGE → CHANGE
func (c *EventCoalescer) ProcessEvent(event FileChange) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.toKey(event.Path)
	existing, exists := c.events[key]

	if !exists {
		// New event, just store it
		c.events[key] = &FileChange{
			Type: event.Type,
			Path: event.Path,
		}
		return
	}

	// Event path already exists - apply coalescing rules
	currentType := existing.Type
	newType := event.Type

	// Track case renames on case-insensitive file systems
	// by keeping both CREATE and DELETE events if paths differ in case
	if existing.Path != event.Path &&
		(event.Type == FileChangeDeleted || event.Type == FileChangeCreated) {
		// Different case - keep both events
		c.events[key] = &FileChange{
			Type: event.Type,
			Path: event.Path,
		}
		return
	}

	// CREATE + DELETE → ignore (nothing changed)
	if currentType == FileChangeCreated && newType == FileChangeDeleted {
		delete(c.events, key)
		return
	}

	// DELETE + CREATE → CHANGE (atomic save scenario)
	if currentType == FileChangeDeleted && newType == FileChangeCreated {
		existing.Type = FileChangeChanged
		return
	}

	// CREATE + CHANGE → CREATE (keep create)
	if currentType == FileChangeCreated && newType == FileChangeChanged {
		// Do nothing, keep CREATE
		return
	}

	// Otherwise, apply the new change type
	existing.Type = newType
}

// Coalesce returns the coalesced events and removes redundant DELETE events
// Reference: VS Code's algorithm to remove child DELETEs when parent is deleted
func (c *EventCoalescer) Coalesce() []FileChange {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Split events into add/change and delete
	addOrChange := make([]FileChange, 0)
	deletes := make([]FileChange, 0)

	for _, event := range c.events {
		if event.Type == FileChangeDeleted {
			deletes = append(deletes, *event)
		} else {
			addOrChange = append(addOrChange, *event)
		}
	}

	// Sort deletes by path length (shortest first)
	sort.Slice(deletes, func(i, j int) bool {
		return len(deletes[i].Path) < len(deletes[j].Path)
	})

	// Filter out child deletes when parent is deleted
	deletedPaths := make([]string, 0)
	filteredDeletes := make([]FileChange, 0)

	for _, event := range deletes {
		isChildOfDeleted := false
		for _, deletedPath := range deletedPaths {
			if isParent(event.Path, deletedPath, !c.isLinux) {
				isChildOfDeleted = true
				break
			}
		}

		if !isChildOfDeleted {
			filteredDeletes = append(filteredDeletes, event)
			deletedPaths = append(deletedPaths, event.Path)
		}
	}

	// Clear the events map
	c.events = make(map[string]*FileChange)

	// Combine and return
	return append(filteredDeletes, addOrChange...)
}

// Reset clears all pending events
func (c *EventCoalescer) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = make(map[string]*FileChange)
}

// isParent checks if path is a child of parentPath
func isParent(path, parentPath string, ignoreCase bool) bool {
	if ignoreCase {
		path = strings.ToLower(path)
		parentPath = strings.ToLower(parentPath)
	}

	// Ensure parent path ends with separator
	if !strings.HasSuffix(parentPath, string(filepath.Separator)) {
		parentPath += string(filepath.Separator)
	}

	return strings.HasPrefix(path, parentPath)
}

// ThrottledWorker batches work items and processes them with throttling
// Reference: VS Code's ThrottledWorker in nodejsWatcherLib.ts
type ThrottledWorker struct {
	maxWorkChunkSize int
	throttleDelay    time.Duration
	maxBufferedWork  int

	buffer    []FileChange
	mu        sync.Mutex
	handler   func([]FileChange)
	timer     *time.Timer
	isRunning bool
}

// NewThrottledWorker creates a new throttled worker
func NewThrottledWorker(handler func([]FileChange)) *ThrottledWorker {
	return &ThrottledWorker{
		maxWorkChunkSize: 100,                    // Process up to 100 events at once
		throttleDelay:    200 * time.Millisecond, // Rest 200ms between chunks
		maxBufferedWork:  10000,                  // Max 10000 events in buffer
		buffer:           make([]FileChange, 0),
		handler:          handler,
	}
}

// Add adds work items to the buffer
func (w *ThrottledWorker) Add(events []FileChange) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Append events, respecting max buffer size
	remaining := w.maxBufferedWork - len(w.buffer)
	if remaining <= 0 {
		// Buffer full, drop new events
		return
	}

	if len(events) > remaining {
		events = events[:remaining]
	}

	w.buffer = append(w.buffer, events...)

	// Start processing if not already running
	if !w.isRunning {
		w.isRunning = true
		w.scheduleWork()
	}
}

// scheduleWork schedules the next batch processing
func (w *ThrottledWorker) scheduleWork() {
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.throttleDelay, w.processWork)
}

// processWork processes a batch of work items
func (w *ThrottledWorker) processWork() {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.isRunning = false
		w.mu.Unlock()
		return
	}

	// Take a chunk
	chunkSize := w.maxWorkChunkSize
	if len(w.buffer) < chunkSize {
		chunkSize = len(w.buffer)
	}

	chunk := make([]FileChange, chunkSize)
	copy(chunk, w.buffer[:chunkSize])

	// Remove from buffer
	w.buffer = w.buffer[chunkSize:]

	// Schedule next if buffer not empty
	if len(w.buffer) > 0 {
		w.scheduleWork()
	} else {
		w.isRunning = false
	}
	w.mu.Unlock()

	// Process chunk
	if w.handler != nil {
		w.handler(chunk)
	}
}
