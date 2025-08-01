// Package eventcron provides table loading and management functionality
package eventcron

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadTable loads an eventcron table from a file
func LoadTable(filePath string) (*IncronTable, error) {
	table := &IncronTable{
		FilePath: filePath,
	}

	// Extract username from file path if it's a user table
	if strings.Contains(filePath, DefaultUserTableDir) {
		base := filepath.Base(filePath)
		table.Username = base
	}

	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open table file %s: %v", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		entry, err := ParseEntry(line, lineNumber)
		if err != nil {
			return nil, fmt.Errorf("error in file %s: %v", filePath, err)
		}

		// Skip nil entries (empty lines, comments)
		if entry != nil {
			table.Add(*entry)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file %s: %v", filePath, err)
	}

	return table, nil
}

// SaveTable saves an eventcron table to a file
func SaveTable(table *IncronTable, filePath string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %v", dir, err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %v", filePath, err)
	}
	defer file.Close()

	// Write header comment
	fmt.Fprintf(file, "# Eventcron table for user %s\n", table.Username)
	fmt.Fprintf(file, "# Format: <path> <mask> <command>\n")
	fmt.Fprintf(file, "# Generated by eventcron %s\n\n", Version)

	// Write entries
	for _, entry := range table.Entries {
		fmt.Fprintln(file, entry.String())
	}

	return nil
}

// LoadUserTable loads a user's eventcron table
func LoadUserTable(username string) (*IncronTable, error) {
	tablePath := GetUserTablePath(username)
	return LoadTable(tablePath)
}

// LoadSystemTable loads a system eventcron table
func LoadSystemTable(tableName string) (*IncronTable, error) {
	tablePath := GetSystemTablePath(tableName)
	return LoadTable(tablePath)
}

// GetUserTablePath returns the path to a user's eventcron table
func GetUserTablePath(username string) string {
	return filepath.Join(DefaultUserTableDir, username)
}

// GetSystemTablePath returns the path to a system eventcron table
func GetSystemTablePath(tableName string) string {
	return filepath.Join(DefaultSystemTableDir, tableName)
}

// LoadAllUserTables loads all user tables from the user table directory
func LoadAllUserTables() (map[string]*IncronTable, error) {
	tables := make(map[string]*IncronTable)

	entries, err := os.ReadDir(DefaultUserTableDir)
	if err != nil {
		if os.IsNotExist(err) {
			return tables, nil // Return empty map if directory doesn't exist
		}
		return nil, fmt.Errorf("failed to read user table directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		username := entry.Name()
		table, err := LoadUserTable(username)
		if err != nil {
			// Log error but continue with other tables
			fmt.Fprintf(os.Stderr, "Warning: failed to load user table for %s: %v\n", username, err)
			continue
		}

		if !table.IsEmpty() {
			tables[username] = table
		}
	}

	return tables, nil
}

// LoadAllSystemTables loads all system tables from the system table directory
func LoadAllSystemTables() (map[string]*IncronTable, error) {
	tables := make(map[string]*IncronTable)

	entries, err := os.ReadDir(DefaultSystemTableDir)
	if err != nil {
		if os.IsNotExist(err) {
			return tables, nil // Return empty map if directory doesn't exist
		}
		return nil, fmt.Errorf("failed to read system table directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		tableName := entry.Name()
		table, err := LoadSystemTable(tableName)
		if err != nil {
			// Log error but continue with other tables
			fmt.Fprintf(os.Stderr, "Warning: failed to load system table %s: %v\n", tableName, err)
			continue
		}

		if !table.IsEmpty() {
			tables[tableName] = table
		}
	}

	return tables, nil
}

// TableExists checks if a table file exists
func TableExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

// UserTableExists checks if a user table exists
func UserTableExists(username string) bool {
	return TableExists(GetUserTablePath(username))
}

// SystemTableExists checks if a system table exists
func SystemTableExists(tableName string) bool {
	return TableExists(GetSystemTablePath(tableName))
}

// RemoveUserTable removes a user's eventcron table
func RemoveUserTable(username string) error {
	tablePath := GetUserTablePath(username)
	err := os.Remove(tablePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove user table for %s: %v", username, err)
	}
	return nil
}

// RemoveSystemTable removes a system eventcron table
func RemoveSystemTable(tableName string) error {
	tablePath := GetSystemTablePath(tableName)
	err := os.Remove(tablePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove system table %s: %v", tableName, err)
	}
	return nil
}

// ValidateTable validates all entries in a table
func ValidateTable(table *IncronTable) []error {
	var errors []error

	for i, entry := range table.Entries {
		if err := ValidateEntry(&entry); err != nil {
			errors = append(errors, fmt.Errorf("entry %d: %v", i+1, err))
		}
	}

	return errors
}

// ValidateEntry validates a single eventcron entry
func ValidateEntry(entry *IncronEntry) error {
	// Check if path is absolute
	if !filepath.IsAbs(entry.Path) {
		return fmt.Errorf("path must be absolute: %s", entry.Path)
	}

	// Check if command is not empty
	if strings.TrimSpace(entry.Command) == "" {
		return fmt.Errorf("command cannot be empty")
	}

	// Check if mask is valid
	if entry.Mask == 0 {
		return fmt.Errorf("event mask cannot be zero")
	}

	return nil
}
