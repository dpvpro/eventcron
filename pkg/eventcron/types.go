// Package eventcron provides core types and functionality for the Go implementation of eventcron
package eventcron

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"syscall"
)

// Version information
const (
	Version = "1.0.0"
	Name    = "eventcron"
)

// Default paths and configuration
const (
	DefaultConfigFile    = "/etc/eventcron.conf"
	DefaultUserTableDir  = "/var/spool/eventcron"
	DefaultSystemTableDir = "/etc/eventcron.d"
	DefaultAllowFile     = "/etc/eventcron.allow"
	DefaultDenyFile      = "/etc/eventcron.deny"
)

// Inotify event masks - mapping from original C++ constants
const (
	InAccess        = syscall.IN_ACCESS
	InModify        = syscall.IN_MODIFY
	InAttrib        = syscall.IN_ATTRIB
	InCloseWrite    = syscall.IN_CLOSE_WRITE
	InCloseNowrite  = syscall.IN_CLOSE_NOWRITE
	InOpen          = syscall.IN_OPEN
	InMovedFrom     = syscall.IN_MOVED_FROM
	InMovedTo       = syscall.IN_MOVED_TO
	InCreate        = syscall.IN_CREATE
	InDelete        = syscall.IN_DELETE
	InDeleteSelf    = syscall.IN_DELETE_SELF
	InMoveSelf      = syscall.IN_MOVE_SELF
	InUnmount       = syscall.IN_UNMOUNT
	InQOverflow     = syscall.IN_Q_OVERFLOW
	InIgnored       = syscall.IN_IGNORED
	InOnlydir       = syscall.IN_ONLYDIR
	InDontFollow    = syscall.IN_DONT_FOLLOW
	InExclUnlink    = syscall.IN_EXCL_UNLINK
	InMaskAdd       = syscall.IN_MASK_ADD
	InIsdir         = syscall.IN_ISDIR
	InOneshot       = syscall.IN_ONESHOT
	InAllEvents     = syscall.IN_ALL_EVENTS
	InMove          = InMovedFrom | InMovedTo
	InClose         = InCloseWrite | InCloseNowrite
)

// EventMaskMap maps string representations to syscall constants
var EventMaskMap = map[string]uint32{
	"IN_ACCESS":        InAccess,
	"IN_MODIFY":        InModify,
	"IN_ATTRIB":        InAttrib,
	"IN_CLOSE_WRITE":   InCloseWrite,
	"IN_CLOSE_NOWRITE": InCloseNowrite,
	"IN_OPEN":          InOpen,
	"IN_MOVED_FROM":    InMovedFrom,
	"IN_MOVED_TO":      InMovedTo,
	"IN_CREATE":        InCreate,
	"IN_DELETE":        InDelete,
	"IN_DELETE_SELF":   InDeleteSelf,
	"IN_MOVE_SELF":     InMoveSelf,
	"IN_UNMOUNT":       InUnmount,
	"IN_Q_OVERFLOW":    InQOverflow,
	"IN_IGNORED":       InIgnored,
	"IN_ONLYDIR":       InOnlydir,
	"IN_DONT_FOLLOW":   InDontFollow,
	"IN_EXCL_UNLINK":   InExclUnlink,
	"IN_MASK_ADD":      InMaskAdd,
	"IN_ISDIR":         InIsdir,
	"IN_ONESHOT":       InOneshot,
	"IN_ALL_EVENTS":    InAllEvents,
	"IN_MOVE":          InMove,
	"IN_CLOSE":         InClose,
}

// ReverseEventMaskMap maps syscall constants to string representations
var ReverseEventMaskMap = map[uint32]string{
	InAccess:        "IN_ACCESS",
	InModify:        "IN_MODIFY",
	InAttrib:        "IN_ATTRIB",
	InCloseWrite:    "IN_CLOSE_WRITE",
	InCloseNowrite:  "IN_CLOSE_NOWRITE",
	InOpen:          "IN_OPEN",
	InMovedFrom:     "IN_MOVED_FROM",
	InMovedTo:       "IN_MOVED_TO",
	InCreate:        "IN_CREATE",
	InDelete:        "IN_DELETE",
	InDeleteSelf:    "IN_DELETE_SELF",
	InMoveSelf:      "IN_MOVE_SELF",
	InUnmount:       "IN_UNMOUNT",
	InQOverflow:     "IN_Q_OVERFLOW",
	InIgnored:       "IN_IGNORED",
	InOnlydir:       "IN_ONLYDIR",
	InDontFollow:    "IN_DONT_FOLLOW",
	InExclUnlink:    "IN_EXCL_UNLINK",
	InMaskAdd:       "IN_MASK_ADD",
	InIsdir:         "IN_ISDIR",
	InOneshot:       "IN_ONESHOT",
	InAllEvents:     "IN_ALL_EVENTS",
}

// EntryOptions holds additional options for eventcron entries
type EntryOptions struct {
	NoLoop     bool // loopable=false - disable events during command execution
	Recursive  bool // recursive=true/false - watch subdirectories
	DotDirs    bool // dotdirs=true - include hidden directories and files
}

// eventcronEntry represents a single entry in an eventcron table
type IncronEntry struct {
	Path      string       // Watched filesystem path
	Mask      uint32       // Event mask (combination of IN_* constants)
	Command   string       // Command to execute
	Options   EntryOptions // Additional options
	LineNumber int         // Line number in the source file (for error reporting)
}

// String returns the string representation of an eventcronEntry suitable for writing to a file
func (e *IncronEntry) String() string {
	maskStr := e.MaskToString()

	// Add options to mask if they differ from defaults
	var opts []string
	if !e.Options.NoLoop {
		opts = append(opts, "loopable=true")
	}
	if !e.Options.Recursive {
		opts = append(opts, "recursive=false")
	}
	if e.Options.DotDirs {
		opts = append(opts, "dotdirs=true")
	}

	if len(opts) > 0 {
		maskStr = maskStr + "," + strings.Join(opts, ",")
	}

	return fmt.Sprintf("%s %s %s", e.Path, maskStr, e.Command)
}

// MaskToString converts the numeric mask to string representation
func (e *IncronEntry) MaskToString() string {
	if e.Mask == InAllEvents {
		return "IN_ALL_EVENTS"
	}

	var parts []string
	mask := e.Mask

	// Check each flag in order of preference
	flags := []uint32{
		InAccess, InModify, InAttrib, InCloseWrite, InCloseNowrite,
		InOpen, InMovedFrom, InMovedTo, InCreate, InDelete,
		InDeleteSelf, InMoveSelf, InUnmount, InQOverflow, InIgnored,
		InOnlydir, InDontFollow, InExclUnlink, InMaskAdd, InIsdir, InOneshot,
	}

	for _, flag := range flags {
		if mask&flag != 0 {
			if name, ok := ReverseEventMaskMap[flag]; ok {
				parts = append(parts, name)
				mask &^= flag // Remove this flag from mask
			}
		}
	}

	// If there are remaining bits, add them as numeric
	if mask != 0 {
		parts = append(parts, fmt.Sprintf("0x%x", mask))
	}

	if len(parts) == 0 {
		return "0"
	}

	return strings.Join(parts, ",")
}

// ParseEntry parses a string line into an IncronEntry
func ParseEntry(line string, lineNumber int) (*IncronEntry, error) {
	line = strings.TrimSpace(line)

	// Skip empty lines and comments
	if line == "" || strings.HasPrefix(line, "#") {
		return nil, nil
	}

	// Split into at most 3 parts: path, mask, command
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("line %d: invalid format, expected: <path> <mask> <command>", lineNumber)
	}

	entry := &IncronEntry{
		Path:       parts[0],
		LineNumber: lineNumber,
		Options: EntryOptions{
			NoLoop:    true,  // Default: loopable=false
			Recursive: true,  // Default: recursive=true
			DotDirs:   false, // Default: dotdirs=false
		},
	}

	// Parse mask and options
	mask, err := parseMask(parts[1], &entry.Options)
	if err != nil {
		return nil, fmt.Errorf("line %d: %v", lineNumber, err)
	}
	entry.Mask = mask

	// Command is everything after the second space
	entry.Command = parts[2]

	return entry, nil
}

// parseMask parses the mask string and extracts options
func parseMask(maskStr string, opts *EntryOptions) (uint32, error) {
	var mask uint32

	// Split by comma to handle options
	parts := strings.Split(maskStr, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)

		// Check if it's an option
		if strings.Contains(part, "=") {
			if err := parseOption(part, opts); err != nil {
				return 0, err
			}
			continue
		}

		// Parse as event mask
		if eventMask, ok := EventMaskMap[part]; ok {
			mask |= eventMask
		} else if num, err := parseNumericMask(part); err == nil {
			mask |= num
		} else {
			return 0, fmt.Errorf("unknown event mask: %s", part)
		}
	}

	if mask == 0 {
		return 0, fmt.Errorf("no valid event mask specified")
	}

	return mask, nil
}

// parseOption parses a single option like "loopable=false"
func parseOption(optStr string, opts *EntryOptions) error {
	parts := strings.SplitN(optStr, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid option format: %s", optStr)
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	switch key {
	case "loopable":
		if value == "true" {
			opts.NoLoop = false
		} else if value == "false" {
			opts.NoLoop = true
		} else {
			return fmt.Errorf("invalid value for loopable: %s (expected true/false)", value)
		}
	case "recursive":
		if value == "true" {
			opts.Recursive = true
		} else if value == "false" {
			opts.Recursive = false
		} else {
			return fmt.Errorf("invalid value for recursive: %s (expected true/false)", value)
		}
	case "dotdirs":
		if value == "true" {
			opts.DotDirs = true
		} else if value == "false" {
			opts.DotDirs = false
		} else {
			return fmt.Errorf("invalid value for dotdirs: %s (expected true/false)", value)
		}
	default:
		return fmt.Errorf("unknown option: %s", key)
	}

	return nil
}

// parseNumericMask parses numeric mask (hex or decimal)
func parseNumericMask(s string) (uint32, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		val, err := strconv.ParseUint(s[2:], 16, 32)
		return uint32(val), err
	}
	val, err := strconv.ParseUint(s, 10, 32)
	return uint32(val), err
}

// ExpandCommand expands wildcards in the command string
func (e *IncronEntry) ExpandCommand(watchPath, filename string, eventMask uint32) string {
	cmd := e.Command

	// Replace wildcards
	cmd = strings.ReplaceAll(cmd, "$$", "$")
	cmd = strings.ReplaceAll(cmd, "$@", watchPath)
	cmd = strings.ReplaceAll(cmd, "$#", filename)
	cmd = strings.ReplaceAll(cmd, "$%", e.eventMaskToText(eventMask))
	cmd = strings.ReplaceAll(cmd, "$&", fmt.Sprintf("%d", eventMask))

	return cmd
}

// eventMaskToText converts event mask to human-readable text
func (e *IncronEntry) eventMaskToText(mask uint32) string {
	var parts []string

	for flag, name := range ReverseEventMaskMap {
		if mask&flag != 0 {
			parts = append(parts, name)
		}
	}

	if len(parts) == 0 {
		return fmt.Sprintf("0x%x", mask)
	}

	return strings.Join(parts, ",")
}

// MatchesPath checks if the given path matches this entry's path pattern
func (e *IncronEntry) MatchesPath(path string) bool {
	// For now, implement simple glob-style matching
	// TODO: Implement full glob pattern matching
	if strings.Contains(e.Path, "*") {
		pattern := strings.ReplaceAll(e.Path, "*", ".*")
		matched, _ := regexp.MatchString("^"+pattern+"$", path)
		return matched
	}

	return e.Path == path
}

// IncronTable represents a collection of incron entries
type IncronTable struct {
	Entries  []IncronEntry
	Username string // Empty for system tables
	FilePath string // Path to the source file
}

// Add adds an entry to the table
func (t *IncronTable) Add(entry IncronEntry) {
	t.Entries = append(t.Entries, entry)
}

// Clear removes all entries from the table
func (t *IncronTable) Clear() {
	t.Entries = t.Entries[:0]
}

// IsEmpty returns true if the table has no entries
func (t *IncronTable) IsEmpty() bool {
	return len(t.Entries) == 0
}

// Count returns the number of entries in the table
func (t *IncronTable) Count() int {
	return len(t.Entries)
}

// String returns the string representation of the entire table
func (t *IncronTable) String() string {
	var lines []string
	for _, entry := range t.Entries {
		lines = append(lines, entry.String())
	}
	return strings.Join(lines, "\n")
}
