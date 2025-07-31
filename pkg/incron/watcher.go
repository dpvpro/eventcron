// Package incron provides inotify watcher functionality
package incron

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// InotifyEvent represents an inotify event
type InotifyEvent struct {
	Path     string // Full path where the event occurred
	Name     string // Name of the file/directory that triggered the event
	Mask     uint32 // Event mask
	Cookie   uint32 // Unique cookie for related events
	WatchDir string // The directory being watched
}

// String returns a string representation of the event
func (e *InotifyEvent) String() string {
	return fmt.Sprintf("InotifyEvent{Path: %s, Name: %s, Mask: %s, Cookie: %d, WatchDir: %s}",
		e.Path, e.Name, maskToString(e.Mask), e.Cookie, e.WatchDir)
}

// Watcher manages inotify watches for incron entries
type Watcher struct {
	fd          int                    // Inotify file descriptor
	watches     map[int]*WatchInfo     // Watch descriptor to watch info mapping
	pathWatches map[string]int         // Path to watch descriptor mapping
	events      chan *InotifyEvent     // Event channel
	errors      chan error             // Error channel
	done        chan struct{}          // Done channel for shutdown
	mu          sync.RWMutex           // Mutex for thread safety
	running     bool                   // Whether the watcher is running
}

// WatchInfo contains information about a watched path
type WatchInfo struct {
	Path      string      // Watched path
	Mask      uint32      // Watch mask
	Entry     *IncronEntry // Associated incron entry
	Recursive bool        // Whether to watch recursively
	DotDirs   bool        // Whether to include dot directories
}

// NewWatcher creates a new inotify watcher
func NewWatcher() (*Watcher, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize inotify: %v", err)
	}

	w := &Watcher{
		fd:          fd,
		watches:     make(map[int]*WatchInfo),
		pathWatches: make(map[string]int),
		events:      make(chan *InotifyEvent, 100),
		errors:      make(chan error, 10),
		done:        make(chan struct{}),
	}

	return w, nil
}

// Start starts the watcher goroutine
func (w *Watcher) Start() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return fmt.Errorf("watcher is already running")
	}

	w.running = true
	go w.readEvents()
	return nil
}

// Stop stops the watcher and closes all resources
func (w *Watcher) Stop() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.running {
		return nil
	}

	w.running = false
	close(w.done)

	// Remove all watches
	for wd := range w.watches {
		_, _ = unix.InotifyRmWatch(w.fd, uint32(wd))
	}

	// Close file descriptor
	if err := unix.Close(w.fd); err != nil {
		return fmt.Errorf("failed to close inotify fd: %v", err)
	}

	close(w.events)
	close(w.errors)

	return nil
}

// AddWatch adds a watch for the given incron entry
func (w *Watcher) AddWatch(entry *IncronEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	path := entry.Path

	// Check if we're already watching this path
	if _, exists := w.pathWatches[path]; exists {
		return fmt.Errorf("path %s is already being watched", path)
	}

	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot stat path %s: %v", path, err)
	}

	watchInfo := &WatchInfo{
		Path:      path,
		Mask:      entry.Mask,
		Entry:     entry,
		Recursive: entry.Options.Recursive,
		DotDirs:   entry.Options.DotDirs,
	}

	// Add watch for the main path
	wd, err := w.addSingleWatch(path, entry.Mask)
	if err != nil {
		return err
	}

	w.watches[wd] = watchInfo
	w.pathWatches[path] = wd

	// If it's a directory and recursive is enabled, add watches for subdirectories
	if info.IsDir() && entry.Options.Recursive {
		if err := w.addRecursiveWatches(path, entry.Mask, entry.Options.DotDirs); err != nil {
			// Clean up the main watch if recursive setup fails
			w.removeWatch(wd)
			return fmt.Errorf("failed to setup recursive watches: %v", err)
		}
	}

	return nil
}

// RemoveWatch removes a watch for the given path
func (w *Watcher) RemoveWatch(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	wd, exists := w.pathWatches[path]
	if !exists {
		return fmt.Errorf("path %s is not being watched", path)
	}

	return w.removeWatch(wd)
}

// removeWatch removes a watch by watch descriptor (internal, assumes lock held)
func (w *Watcher) removeWatch(wd int) error {
	watchInfo, exists := w.watches[wd]
	if !exists {
		return fmt.Errorf("watch descriptor %d not found", wd)
	}

	// Remove from inotify
	if _, err := unix.InotifyRmWatch(w.fd, uint32(wd)); err != nil {
		return fmt.Errorf("failed to remove inotify watch: %v", err)
	}

	// Remove from our maps
	delete(w.watches, wd)
	delete(w.pathWatches, watchInfo.Path)

	return nil
}

// addSingleWatch adds a single inotify watch
func (w *Watcher) addSingleWatch(path string, mask uint32) (int, error) {
	wd, err := unix.InotifyAddWatch(w.fd, path, mask)
	if err != nil {
		return -1, fmt.Errorf("failed to add inotify watch for %s: %v", path, err)
	}
	return wd, nil
}

// addRecursiveWatches adds watches for all subdirectories
func (w *Watcher) addRecursiveWatches(rootPath string, mask uint32, includeDotDirs bool) error {
	return filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip non-directories
		if !info.IsDir() {
			return nil
		}

		// Skip the root path (already added)
		if path == rootPath {
			return nil
		}

		// Skip dot directories if not enabled
		if !includeDotDirs && strings.HasPrefix(filepath.Base(path), ".") {
			return filepath.SkipDir
		}

		// Add watch for this directory
		wd, err := w.addSingleWatch(path, mask)
		if err != nil {
			// Log error but continue with other directories
			fmt.Fprintf(os.Stderr, "Warning: failed to add watch for %s: %v\n", path, err)
			return nil
		}

		// Create watch info for this subdirectory
		watchInfo := &WatchInfo{
			Path:      path,
			Mask:      mask,
			Entry:     nil, // Subdirectory watches don't have their own entries
			Recursive: true,
			DotDirs:   includeDotDirs,
		}

		w.watches[wd] = watchInfo
		w.pathWatches[path] = wd

		return nil
	})
}

// Events returns the event channel
func (w *Watcher) Events() <-chan *InotifyEvent {
	return w.events
}

// Errors returns the error channel
func (w *Watcher) Errors() <-chan error {
	return w.errors
}

// readEvents reads events from the inotify file descriptor
func (w *Watcher) readEvents() {
	buffer := make([]byte, 4096)

	for {
		select {
		case <-w.done:
			return
		default:
			n, err := unix.Read(w.fd, buffer)
			if err != nil {
				if err == syscall.EINTR {
					continue
				}
				select {
				case w.errors <- fmt.Errorf("error reading inotify events: %v", err):
				case <-w.done:
				}
				return
			}

			if n == 0 {
				continue
			}

			w.parseEvents(buffer[:n])
		}
	}
}

// parseEvents parses raw inotify events from buffer
func (w *Watcher) parseEvents(buffer []byte) {
	offset := 0

	for offset < len(buffer) {
		if offset+16 > len(buffer) {
			break
		}

		// Parse inotify_event structure
		wd := int(*(*int32)(unsafe.Pointer(&buffer[offset])))
		mask := *(*uint32)(unsafe.Pointer(&buffer[offset+4]))
		cookie := *(*uint32)(unsafe.Pointer(&buffer[offset+8]))
		nameLen := *(*uint32)(unsafe.Pointer(&buffer[offset+12]))

		offset += 16

		var name string
		if nameLen > 0 {
			if offset+int(nameLen) > len(buffer) {
				break
			}
			// Remove null terminator
			nameBytes := buffer[offset : offset+int(nameLen)]
			if len(nameBytes) > 0 && nameBytes[len(nameBytes)-1] == 0 {
				nameBytes = nameBytes[:len(nameBytes)-1]
			}
			name = string(nameBytes)
			offset += int(nameLen)
		}

		// Create event
		event := w.createEvent(wd, mask, cookie, name)
		if event != nil {
			select {
			case w.events <- event:
			case <-w.done:
				return
			default:
				// Channel is full, drop event
				fmt.Fprintf(os.Stderr, "Warning: event channel full, dropping event: %v\n", event)
			}
		}

		// Handle directory creation for recursive watches
		if mask&unix.IN_CREATE != 0 && mask&unix.IN_ISDIR != 0 {
			w.handleDirCreate(wd, name)
		}
	}
}

// createEvent creates an InotifyEvent from raw data
func (w *Watcher) createEvent(wd int, mask, cookie uint32, name string) *InotifyEvent {
	w.mu.RLock()
	watchInfo, exists := w.watches[wd]
	w.mu.RUnlock()

	if !exists {
		return nil
	}

	path := watchInfo.Path
	if name != "" {
		path = filepath.Join(path, name)
	}

	return &InotifyEvent{
		Path:     path,
		Name:     name,
		Mask:     mask,
		Cookie:   cookie,
		WatchDir: watchInfo.Path,
	}
}

// handleDirCreate handles directory creation for recursive watches
func (w *Watcher) handleDirCreate(wd int, name string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	watchInfo, exists := w.watches[wd]
	if !exists || !watchInfo.Recursive {
		return
	}

	// Skip dot directories if not enabled
	if !watchInfo.DotDirs && strings.HasPrefix(name, ".") {
		return
	}

	newPath := filepath.Join(watchInfo.Path, name)

	// Check if the new path is a directory
	info, err := os.Stat(newPath)
	if err != nil || !info.IsDir() {
		return
	}

	// Add watch for the new directory
	newWd, err := w.addSingleWatch(newPath, watchInfo.Mask)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to add watch for new directory %s: %v\n", newPath, err)
		return
	}

	// Create watch info for the new directory
	newWatchInfo := &WatchInfo{
		Path:      newPath,
		Mask:      watchInfo.Mask,
		Entry:     nil, // Subdirectory watches don't have their own entries
		Recursive: true,
		DotDirs:   watchInfo.DotDirs,
	}

	w.watches[newWd] = newWatchInfo
	w.pathWatches[newPath] = newWd
}

// GetWatchedPaths returns a list of all watched paths
func (w *Watcher) GetWatchedPaths() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	paths := make([]string, 0, len(w.pathWatches))
	for path := range w.pathWatches {
		paths = append(paths, path)
	}
	return paths
}

// GetWatchCount returns the number of active watches
func (w *Watcher) GetWatchCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.watches)
}

// maskToString converts an event mask to string representation
func maskToString(mask uint32) string {
	var parts []string

	for flag, name := range ReverseEventMaskMap {
		if mask&flag != 0 {
			parts = append(parts, name)
		}
	}

	if len(parts) == 0 {
		return fmt.Sprintf("0x%x", mask)
	}

	return strings.Join(parts, "|")
}