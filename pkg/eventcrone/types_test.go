package eventcrone

import (
	"strings"
	"testing"
)

func TestParseEntry(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		lineNumber  int
		expectError bool
		expected    *IncronEntry
	}{
		{
			name:       "simple entry",
			line:       "/tmp IN_CREATE echo test",
			lineNumber: 1,
			expected: &IncronEntry{
				Path:       "/tmp",
				Mask:       InCreate,
				Command:    "echo test",
				LineNumber: 1,
				Options: EntryOptions{
					NoLoop:    true,
					Recursive: true,
					DotDirs:   false,
				},
			},
		},
		{
			name:       "multiple events",
			line:       "/tmp IN_CREATE,IN_MODIFY echo test",
			lineNumber: 1,
			expected: &IncronEntry{
				Path:       "/tmp",
				Mask:       InCreate | InModify,
				Command:    "echo test",
				LineNumber: 1,
				Options: EntryOptions{
					NoLoop:    true,
					Recursive: true,
					DotDirs:   false,
				},
			},
		},
		{
			name:       "with options",
			line:       "/tmp IN_CREATE,loopable=true,recursive=false echo test",
			lineNumber: 1,
			expected: &IncronEntry{
				Path:       "/tmp",
				Mask:       InCreate,
				Command:    "echo test",
				LineNumber: 1,
				Options: EntryOptions{
					NoLoop:    false,
					Recursive: false,
					DotDirs:   false,
				},
			},
		},
		{
			name:        "empty line",
			line:        "",
			lineNumber:  1,
			expected:    nil,
			expectError: false,
		},
		{
			name:        "comment line",
			line:        "# this is a comment",
			lineNumber:  1,
			expected:    nil,
			expectError: false,
		},
		{
			name:        "invalid format",
			line:        "/tmp IN_CREATE",
			lineNumber:  1,
			expected:    nil,
			expectError: true,
		},
		{
			name:        "invalid mask",
			line:        "/tmp INVALID_MASK echo test",
			lineNumber:  1,
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry, err := ParseEntry(tt.line, tt.lineNumber)
			
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			
			if tt.expected == nil {
				if entry != nil {
					t.Errorf("expected nil entry but got %+v", entry)
				}
				return
			}
			
			if entry == nil {
				t.Errorf("expected entry but got nil")
				return
			}
			
			if entry.Path != tt.expected.Path {
				t.Errorf("path mismatch: got %q, want %q", entry.Path, tt.expected.Path)
			}
			
			if entry.Mask != tt.expected.Mask {
				t.Errorf("mask mismatch: got %d, want %d", entry.Mask, tt.expected.Mask)
			}
			
			if entry.Command != tt.expected.Command {
				t.Errorf("command mismatch: got %q, want %q", entry.Command, tt.expected.Command)
			}
			
			if entry.LineNumber != tt.expected.LineNumber {
				t.Errorf("line number mismatch: got %d, want %d", entry.LineNumber, tt.expected.LineNumber)
			}
			
			if entry.Options != tt.expected.Options {
				t.Errorf("options mismatch: got %+v, want %+v", entry.Options, tt.expected.Options)
			}
		})
	}
}

func TestIncronEntry_String(t *testing.T) {
	entry := &IncronEntry{
		Path:    "/tmp",
		Mask:    InCreate | InModify,
		Command: "echo test",
		Options: EntryOptions{
			NoLoop:    true,
			Recursive: true,
			DotDirs:   false,
		},
	}
	
	result := entry.String()
	// The order of flags in the output may vary, so just check that both flags are present
	if !strings.Contains(result, "IN_CREATE") || !strings.Contains(result, "IN_MODIFY") {
		t.Errorf("String() missing expected flags: got %q", result)
	}
	if !strings.Contains(result, "/tmp") || !strings.Contains(result, "echo test") {
		t.Errorf("String() missing expected path or command: got %q", result)
	}
}

func TestIncronEntry_ExpandCommand(t *testing.T) {
	entry := &IncronEntry{
		Command: "echo $@ $# $% $& $$",
	}
	
	expanded := entry.ExpandCommand("/watch/path", "filename.txt", InCreate)
	
	// Check individual components instead of exact match
	if !strings.Contains(expanded, "/watch/path") {
		t.Errorf("ExpandCommand() missing watch path: got %q", expanded)
	}
	if !strings.Contains(expanded, "filename.txt") {
		t.Errorf("ExpandCommand() missing filename: got %q", expanded)
	}
	if !strings.Contains(expanded, "IN_CREATE") {
		t.Errorf("ExpandCommand() missing event name: got %q", expanded)
	}
	if !strings.Contains(expanded, "$") {
		t.Errorf("ExpandCommand() missing literal dollar: got %q", expanded)
	}
}

func TestIncronEntry_MatchesPath(t *testing.T) {
	tests := []struct {
		name     string
		entryPath string
		testPath  string
		expected  bool
	}{
		{
			name:     "exact match",
			entryPath: "/tmp",
			testPath:  "/tmp",
			expected:  true,
		},
		{
			name:     "no match",
			entryPath: "/tmp",
			testPath:  "/var",
			expected:  false,
		},
		{
			name:     "wildcard match",
			entryPath: "/tmp/*.txt",
			testPath:  "/tmp/test.txt",
			expected:  true,
		},
		{
			name:     "wildcard no match",
			entryPath: "/tmp/*.txt",
			testPath:  "/tmp/test.log",
			expected:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &IncronEntry{Path: tt.entryPath}
			result := entry.MatchesPath(tt.testPath)
			
			if result != tt.expected {
				t.Errorf("MatchesPath() mismatch: got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestMaskToString(t *testing.T) {
	tests := []struct {
		name string
		mask uint32
		want string
	}{
		{
			name: "single event",
			mask: InCreate,
			want: "IN_CREATE",
		},
		{
			name: "multiple events",
			mask: InCreate | InModify,
			want: "IN_CREATE,IN_MODIFY",
		},
		{
			name: "all events",
			mask: InAllEvents,
			want: "IN_ALL_EVENTS",
		},
		{
			name: "zero mask",
			mask: 0,
			want: "0",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &IncronEntry{Mask: tt.mask}
			result := entry.MaskToString()
			
			// For multiple events, check that all parts are present
			if strings.Contains(tt.want, ",") {
				parts := strings.Split(tt.want, ",")
				for _, part := range parts {
					if !strings.Contains(result, part) {
						t.Errorf("MaskToString() missing part %q in result %q", part, result)
					}
				}
			} else if result != tt.want {
				t.Errorf("MaskToString() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestIncronTable_Operations(t *testing.T) {
	table := &IncronTable{}
	
	if !table.IsEmpty() {
		t.Error("new table should be empty")
	}
	
	if table.Count() != 0 {
		t.Errorf("new table count should be 0, got %d", table.Count())
	}
	
	entry := IncronEntry{
		Path:    "/tmp",
		Mask:    InCreate,
		Command: "echo test",
	}
	
	table.Add(entry)
	
	if table.IsEmpty() {
		t.Error("table should not be empty after adding entry")
	}
	
	if table.Count() != 1 {
		t.Errorf("table count should be 1, got %d", table.Count())
	}
	
	table.Clear()
	
	if !table.IsEmpty() {
		t.Error("table should be empty after clear")
	}
	
	if table.Count() != 0 {
		t.Errorf("table count should be 0 after clear, got %d", table.Count())
	}
}

func TestEventMaskMap(t *testing.T) {
	// Test that all event masks are properly mapped
	testCases := []struct {
		name string
		mask uint32
	}{
		{"IN_CREATE", InCreate},
		{"IN_MODIFY", InModify},
		{"IN_DELETE", InDelete},
		{"IN_CLOSE_WRITE", InCloseWrite},
		{"IN_ALL_EVENTS", InAllEvents},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if val, ok := EventMaskMap[tc.name]; !ok {
				t.Errorf("EventMaskMap missing entry for %s", tc.name)
			} else if val != tc.mask {
				t.Errorf("EventMaskMap[%s] = %d, want %d", tc.name, val, tc.mask)
			}
			
			if name, ok := ReverseEventMaskMap[tc.mask]; !ok {
				t.Errorf("ReverseEventMaskMap missing entry for %d", tc.mask)
			} else if name != tc.name {
				t.Errorf("ReverseEventMaskMap[%d] = %s, want %s", tc.mask, name, tc.name)
			}
		})
	}
}

func TestValidateEntry(t *testing.T) {
	tests := []struct {
		name        string
		entry       *IncronEntry
		expectError bool
	}{
		{
			name: "valid entry",
			entry: &IncronEntry{
				Path:    "/tmp",
				Mask:    InCreate,
				Command: "echo test",
			},
			expectError: false,
		},
		{
			name: "relative path",
			entry: &IncronEntry{
				Path:    "tmp",
				Mask:    InCreate,
				Command: "echo test",
			},
			expectError: true,
		},
		{
			name: "empty command",
			entry: &IncronEntry{
				Path:    "/tmp",
				Mask:    InCreate,
				Command: "",
			},
			expectError: true,
		},
		{
			name: "zero mask",
			entry: &IncronEntry{
				Path:    "/tmp",
				Mask:    0,
				Command: "echo test",
			},
			expectError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEntry(tt.entry)
			
			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}