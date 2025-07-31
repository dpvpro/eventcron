// Package main implements the incron table manipulator (incrontab) in Go
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/user"

	"strings"
	"syscall"

	"github.com/dpvpro/eventcrone/pkg/incron"
)

const (
	defaultEditor = "vi"
	tempFilePrefix = "incrontab"
)

// Operation represents the type of operation to perform
type Operation int

const (
	OpList Operation = iota
	OpEdit
	OpRemove
	OpReplace
	OpHelp
	OpVersion
)

func main() {
	var (
		listFlag    = flag.Bool("l", false, "List current incron table")
		editFlag    = flag.Bool("e", false, "Edit current incron table")
		removeFlag  = flag.Bool("r", false, "Remove current incron table")
		replaceFlag = flag.Bool("", false, "Replace incron table with file from stdin")
		userFlag    = flag.String("u", "", "Specify user (root only)")
		versionFlag = flag.Bool("V", false, "Show version and exit")
		helpFlag    = flag.Bool("h", false, "Show help and exit")
	)
	flag.Parse()

	if *helpFlag {
		showHelp()
		os.Exit(0)
	}

	if *versionFlag {
		fmt.Printf("incrontab %s\n", incron.Version)
		os.Exit(0)
	}

	// Determine operation
	op := OpList // default
	if *listFlag {
		op = OpList
	} else if *editFlag {
		op = OpEdit
	} else if *removeFlag {
		op = OpRemove
	} else if *replaceFlag {
		op = OpReplace
	} else if flag.NArg() > 0 {
		// File specified as argument means replace
		op = OpReplace
	}

	// Get target user
	targetUser, err := getTargetUser(*userFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Check permissions
	if err := checkPermissions(targetUser); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Execute operation
	if err := executeOperation(op, targetUser); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// showHelp displays usage information
func showHelp() {
	fmt.Printf("Usage: %s [options] [file]\n", os.Args[0])
	fmt.Println("\nOptions:")
	fmt.Println("  -l        List current incron table")
	fmt.Println("  -e        Edit current incron table")
	fmt.Println("  -r        Remove current incron table")
	fmt.Println("  -u user   Specify user (root only)")
	fmt.Println("  -V        Show version and exit")
	fmt.Println("  -h        Show help and exit")
	fmt.Println()
	fmt.Println("If no options are specified, the table is listed.")
	fmt.Println("If a file is specified as an argument, the table is replaced with the file contents.")
	fmt.Println()
	fmt.Println("Table format:")
	fmt.Println("  <path> <mask> <command>")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  /tmp IN_CREATE,IN_MODIFY echo File changed: $@/$#")
	fmt.Println()
	fmt.Printf("incrontab %s\n", incron.Version)
}

// getTargetUser determines which user's table to operate on
func getTargetUser(userFlag string) (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %v", err)
	}

	// If no user specified, use current user
	if userFlag == "" {
		return currentUser.Username, nil
	}

	// Only root can specify other users
	if currentUser.Uid != "0" {
		return "", fmt.Errorf("only root can specify other users")
	}

	// Verify the specified user exists
	_, err = user.Lookup(userFlag)
	if err != nil {
		return "", fmt.Errorf("user %s not found: %v", userFlag, err)
	}

	return userFlag, nil
}

// checkPermissions checks if the user has permission to use incron
func checkPermissions(username string) error {
	// Check if user has permission to use incron
	allowed, err := incron.CheckUserPermission(username)
	if err != nil {
		return fmt.Errorf("failed to check user permissions: %v", err)
	}

	if !allowed {
		return fmt.Errorf("user %s is not allowed to use incron", username)
	}

	return nil
}

// executeOperation executes the specified operation
func executeOperation(op Operation, username string) error {
	switch op {
	case OpList:
		return listTable(username)
	case OpEdit:
		return editTable(username)
	case OpRemove:
		return removeTable(username)
	case OpReplace:
		return replaceTable(username)
	default:
		return fmt.Errorf("unknown operation")
	}
}

// listTable lists the current incron table for the user
func listTable(username string) error {
	if !incron.UserTableExists(username) {
		// No table exists, just exit silently
		return nil
	}

	table, err := incron.LoadUserTable(username)
	if err != nil {
		return fmt.Errorf("failed to load table: %v", err)
	}

	if table.IsEmpty() {
		return nil
	}

	fmt.Print(table.String())
	return nil
}

// editTable opens the user's incron table in an editor
func editTable(username string) error {
	// Get editor
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = defaultEditor
	}

	// Create temporary file
	tempFile, err := os.CreateTemp("", tempFilePrefix+"_"+username+"_*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	// Load existing table if it exists
	var table *incron.IncronTable
	if incron.UserTableExists(username) {
		table, err = incron.LoadUserTable(username)
		if err != nil {
			return fmt.Errorf("failed to load existing table: %v", err)
		}
	} else {
		table = &incron.IncronTable{Username: username}
	}

	// Write current table to temp file
	if !table.IsEmpty() {
		if _, err := tempFile.WriteString(table.String() + "\n"); err != nil {
			tempFile.Close()
			return fmt.Errorf("failed to write to temporary file: %v", err)
		}
	}

	// Add helpful comments for new users
	if table.IsEmpty() {
		helpText := `# Edit this file to configure incron table for user ` + username + `
# Format: <path> <mask> <command>
# 
# Example:
# /tmp IN_CREATE,IN_MODIFY echo "File $# was $% in $@"
#
# Available masks:
# IN_ACCESS, IN_MODIFY, IN_ATTRIB, IN_CLOSE_WRITE, IN_CLOSE_NOWRITE,
# IN_OPEN, IN_MOVED_FROM, IN_MOVED_TO, IN_CREATE, IN_DELETE,
# IN_DELETE_SELF, IN_MOVE_SELF, IN_ALL_EVENTS
#
# Additional options:
# recursive=true/false   - watch subdirectories
# loopable=true/false    - allow events during command execution  
# dotdirs=true/false     - include hidden directories
#
# Wildcards in commands:
# $$  - literal $ character
# $@  - watched directory path
# $#  - filename that triggered the event
# $%  - event name (textual)
# $&  - event flags (numeric)
#

`
		if _, err := tempFile.WriteString(helpText); err != nil {
			tempFile.Close()
			return fmt.Errorf("failed to write help text: %v", err)
		}
	}

	tempFile.Close()

	// Get file modification time before editing
	statBefore, err := os.Stat(tempPath)
	if err != nil {
		return fmt.Errorf("failed to stat temp file: %v", err)
	}

	// Open editor
	cmd := exec.Command(editor, tempPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %v", err)
	}

	// Check if file was modified
	statAfter, err := os.Stat(tempPath)
	if err != nil {
		return fmt.Errorf("failed to stat temp file after editing: %v", err)
	}

	if statBefore.ModTime().Equal(statAfter.ModTime()) {
		fmt.Println("No changes made")
		return nil
	}

	// Parse the edited file
	newTable, err := incron.LoadTable(tempPath)
	if err != nil {
		return fmt.Errorf("failed to parse edited table: %v", err)
	}

	// Set username
	newTable.Username = username

	// Validate the new table
	if errors := incron.ValidateTable(newTable); len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Validation errors found:\n")
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  %v\n", err)
		}
		
		// Ask user if they want to re-edit
		fmt.Print("Re-edit the table? (y/n): ")
		var response string
		fmt.Scanln(&response)
		if strings.ToLower(response) == "y" || strings.ToLower(response) == "yes" {
			// Copy the edited content back and re-edit
			editedContent, err := os.ReadFile(tempPath)
			if err != nil {
				return fmt.Errorf("failed to read edited file: %v", err)
			}
			
			newTempFile, err := os.CreateTemp("", tempFilePrefix+"_"+username+"_*")
			if err != nil {
				return fmt.Errorf("failed to create new temporary file: %v", err)
			}
			newTempPath := newTempFile.Name()
			defer os.Remove(newTempPath)
			
			if _, err := newTempFile.Write(editedContent); err != nil {
				newTempFile.Close()
				return fmt.Errorf("failed to write to new temporary file: %v", err)
			}
			newTempFile.Close()
			
			// Recursively call editTable with the preserved content
			return editTableWithContent(username, newTempPath)
		}
		return fmt.Errorf("table not saved due to validation errors")
	}

	// Save the new table
	tablePath := incron.GetUserTablePath(username)
	if err := incron.SaveTable(newTable, tablePath); err != nil {
		return fmt.Errorf("failed to save table: %v", err)
	}

	// Set proper permissions
	if err := os.Chmod(tablePath, 0600); err != nil {
		return fmt.Errorf("failed to set table permissions: %v", err)
	}

	// Send SIGHUP to incrond to reload tables
	if err := reloadDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to reload daemon: %v\n", err)
	}

	fmt.Printf("Table for user %s installed\n", username)
	return nil
}

// editTableWithContent is a helper for re-editing with preserved content
func editTableWithContent(username, tempPath string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = defaultEditor
	}

	// Open editor
	cmd := exec.Command(editor, tempPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %v", err)
	}

	// Parse the edited file again
	newTable, err := incron.LoadTable(tempPath)
	if err != nil {
		return fmt.Errorf("failed to parse edited table: %v", err)
	}

	newTable.Username = username

	// Validate again
	if errors := incron.ValidateTable(newTable); len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Validation errors still present:\n")
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  %v\n", err)
		}
		return fmt.Errorf("table not saved due to validation errors")
	}

	// Save the table
	tablePath := incron.GetUserTablePath(username)
	if err := incron.SaveTable(newTable, tablePath); err != nil {
		return fmt.Errorf("failed to save table: %v", err)
	}

	if err := os.Chmod(tablePath, 0600); err != nil {
		return fmt.Errorf("failed to set table permissions: %v", err)
	}

	if err := reloadDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to reload daemon: %v\n", err)
	}

	fmt.Printf("Table for user %s installed\n", username)
	return nil
}

// removeTable removes the user's incron table
func removeTable(username string) error {
	if !incron.UserTableExists(username) {
		fmt.Printf("No table for user %s\n", username)
		return nil
	}

	if err := incron.RemoveUserTable(username); err != nil {
		return fmt.Errorf("failed to remove table: %v", err)
	}

	// Send SIGHUP to incrond to reload tables
	if err := reloadDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to reload daemon: %v\n", err)
	}

	fmt.Printf("Table for user %s removed\n", username)
	return nil
}

// replaceTable replaces the user's incron table with content from stdin or file
func replaceTable(username string) error {
	var input *os.File
	var err error

	// Determine input source
	if flag.NArg() > 0 {
		// Read from file
		filename := flag.Arg(0)
		input, err = os.Open(filename)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %v", filename, err)
		}
		defer input.Close()
	} else {
		// Read from stdin
		input = os.Stdin
	}

	// Create temporary file to store input
	tempFile, err := os.CreateTemp("", tempFilePrefix+"_"+username+"_*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %v", err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	// Copy input to temp file
	scanner := bufio.NewScanner(input)
	for scanner.Scan() {
		if _, err := tempFile.WriteString(scanner.Text() + "\n"); err != nil {
			tempFile.Close()
			return fmt.Errorf("failed to write to temporary file: %v", err)
		}
	}
	tempFile.Close()

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read input: %v", err)
	}

	// Parse the input
	table, err := incron.LoadTable(tempPath)
	if err != nil {
		return fmt.Errorf("failed to parse input: %v", err)
	}

	table.Username = username

	// Validate the table
	if errors := incron.ValidateTable(table); len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Validation errors found:\n")
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  %v\n", err)
		}
		return fmt.Errorf("table not saved due to validation errors")
	}

	// Save the table
	tablePath := incron.GetUserTablePath(username)
	if err := incron.SaveTable(table, tablePath); err != nil {
		return fmt.Errorf("failed to save table: %v", err)
	}

	if err := os.Chmod(tablePath, 0600); err != nil {
		return fmt.Errorf("failed to set table permissions: %v", err)
	}

	// Send SIGHUP to incrond to reload tables
	if err := reloadDaemon(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to reload daemon: %v\n", err)
	}

	fmt.Printf("Table for user %s installed\n", username)
	return nil
}

// reloadDaemon sends SIGHUP to incrond to reload tables
func reloadDaemon() error {
	// Read PID from file
	pidFile := "/var/run/incrond.pid"
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("incrond does not appear to be running")
		}
		return fmt.Errorf("failed to read PID file: %v", err)
	}

	var pid int
	if _, err := fmt.Sscanf(string(pidBytes), "%d", &pid); err != nil {
		return fmt.Errorf("invalid PID in file: %v", err)
	}

	// Send SIGHUP signal
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process %d: %v", pid, err)
	}

	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send SIGHUP to process %d: %v", pid, err)
	}

	return nil
}