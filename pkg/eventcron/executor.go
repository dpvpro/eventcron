// Package incron provides command execution functionality
package eventcron

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// CommandExecutor executes commands for incron entries
type CommandExecutor struct {
	runningCommands map[string]*RunningCommand // Key: command ID
	mu              sync.RWMutex               // Mutex for thread safety
	maxConcurrent   int                        // Maximum concurrent commands
	currentCount    int                        // Current running command count
	timeout         time.Duration              // Command timeout
}

// RunningCommand represents a currently executing command
type RunningCommand struct {
	ID        string          // Unique identifier
	Entry     *IncronEntry    // Associated incron entry
	Event     *InotifyEvent   // Event that triggered the command
	Cmd       *exec.Cmd       // The actual command
	Username  string          // User to run the command as
	StartTime time.Time       // When the command started
	Context   context.Context // Context for cancellation
	Cancel    context.CancelFunc
}

// ExecutionResult represents the result of command execution
type ExecutionResult struct {
	ID       string
	Success  bool
	ExitCode int
	Output   []byte
	Error    error
	Duration time.Duration
}

// NewCommandExecutor creates a new command executor
func NewCommandExecutor(maxConcurrent int, timeout time.Duration) *CommandExecutor {
	return &CommandExecutor{
		runningCommands: make(map[string]*RunningCommand),
		maxConcurrent:   maxConcurrent,
		timeout:         timeout,
	}
}

// Execute executes a command for the given entry and event
func (ce *CommandExecutor) Execute(entry *IncronEntry, event *InotifyEvent, username string) (*ExecutionResult, error) {
	ce.mu.Lock()
	defer ce.mu.Unlock()

	// Check if we've reached the maximum concurrent commands
	if ce.currentCount >= ce.maxConcurrent {
		return nil, fmt.Errorf("maximum concurrent commands (%d) reached", ce.maxConcurrent)
	}

	// Generate unique ID for this command
	id := generateCommandID(entry, event)

	// Check if we should avoid loops
	if entry.Options.NoLoop {
		// Check if a command is already running for this path
		for _, runningCmd := range ce.runningCommands {
			if runningCmd.Entry.Path == entry.Path && runningCmd.Username == username {
				return nil, fmt.Errorf("command already running for path %s (loop prevention)", entry.Path)
			}
		}
	}

	// Expand the command with wildcards
	expandedCmd := entry.ExpandCommand(event.WatchDir, event.Name, event.Mask)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), ce.timeout)

	// Parse command and arguments
	cmdParts := parseCommand(expandedCmd)
	if len(cmdParts) == 0 {
		cancel()
		return nil, fmt.Errorf("empty command")
	}

	// Create the command
	cmd := exec.CommandContext(ctx, cmdParts[0], cmdParts[1:]...)

	// Set environment variables
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf("INCRON_PATH=%s", event.Path))
	cmd.Env = append(cmd.Env, fmt.Sprintf("INCRON_NAME=%s", event.Name))
	cmd.Env = append(cmd.Env, fmt.Sprintf("INCRON_EVENT=%s", maskToString(event.Mask)))

	// Create running command info
	runningCmd := &RunningCommand{
		ID:        id,
		Entry:     entry,
		Event:     event,
		Cmd:       cmd,
		Username:  username,
		StartTime: time.Now(),
		Context:   ctx,
		Cancel:    cancel,
	}

	// Set up credential for running as specific user
	if username != "root" && username != "" {
		if err := ce.setupUserCredentials(cmd, username); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to setup user credentials: %v", err)
		}
	}

	// Store the running command
	ce.runningCommands[id] = runningCmd
	ce.currentCount++

	// Start the command in a goroutine
	resultChan := make(chan *ExecutionResult, 1)
	go ce.runCommand(runningCmd, resultChan)

	// Wait for result or return immediately based on configuration
	// For now, we'll wait for the result
	result := <-resultChan

	// Clean up
	ce.mu.Lock()
	delete(ce.runningCommands, id)
	ce.currentCount--
	ce.mu.Unlock()

	return result, nil
}

// runCommand runs the command and sends the result to the channel
func (ce *CommandExecutor) runCommand(runningCmd *RunningCommand, resultChan chan<- *ExecutionResult) {
	defer runningCmd.Cancel()

	startTime := time.Now()
	
	// Start the command
	output, err := runningCmd.Cmd.CombinedOutput()
	duration := time.Since(startTime)

	result := &ExecutionResult{
		ID:       runningCmd.ID,
		Duration: duration,
		Output:   output,
	}

	if err != nil {
		result.Success = false
		result.Error = err

		// Try to get exit code
		if exitError, ok := err.(*exec.ExitError); ok {
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				result.ExitCode = status.ExitStatus()
			}
		}
	} else {
		result.Success = true
		result.ExitCode = 0
	}

	// Send result
	select {
	case resultChan <- result:
	case <-runningCmd.Context.Done():
		// Context was cancelled
		result.Success = false
		result.Error = fmt.Errorf("command cancelled: %v", runningCmd.Context.Err())
		select {
		case resultChan <- result:
		default:
		}
	}
}

// setupUserCredentials sets up the command to run as the specified user
func (ce *CommandExecutor) setupUserCredentials(cmd *exec.Cmd, username string) error {
	userInfo, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("failed to lookup user %s: %v", username, err)
	}

	uid, err := strconv.Atoi(userInfo.Uid)
	if err != nil {
		return fmt.Errorf("invalid UID for user %s: %v", username, err)
	}

	gid, err := strconv.Atoi(userInfo.Gid)
	if err != nil {
		return fmt.Errorf("invalid GID for user %s: %v", username, err)
	}

	// Set credentials
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		},
	}

	// Set working directory to user's home directory
	cmd.Dir = userInfo.HomeDir

	// Set USER and HOME environment variables
	cmd.Env = append(cmd.Env, fmt.Sprintf("USER=%s", username))
	cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=%s", userInfo.HomeDir))

	return nil
}

// parseCommand parses a command string into command and arguments
func parseCommand(cmdStr string) []string {
	cmdStr = strings.TrimSpace(cmdStr)
	if cmdStr == "" {
		return nil
	}

	// Simple parsing - split by spaces
	// TODO: Implement proper shell-like parsing with quotes
	parts := strings.Fields(cmdStr)
	return parts
}

// generateCommandID generates a unique ID for a command
func generateCommandID(entry *IncronEntry, event *InotifyEvent) string {
	return fmt.Sprintf("%s_%s_%d_%d", 
		strings.ReplaceAll(entry.Path, "/", "_"),
		event.Name,
		event.Mask,
		time.Now().UnixNano())
}

// GetRunningCommands returns information about currently running commands
func (ce *CommandExecutor) GetRunningCommands() map[string]*RunningCommand {
	ce.mu.RLock()
	defer ce.mu.RUnlock()

	result := make(map[string]*RunningCommand)
	for id, cmd := range ce.runningCommands {
		result[id] = cmd
	}
	return result
}

// GetRunningCount returns the number of currently running commands
func (ce *CommandExecutor) GetRunningCount() int {
	ce.mu.RLock()
	defer ce.mu.RUnlock()
	return ce.currentCount
}

// KillCommand kills a running command by ID
func (ce *CommandExecutor) KillCommand(id string) error {
	ce.mu.RLock()
	runningCmd, exists := ce.runningCommands[id]
	ce.mu.RUnlock()

	if !exists {
		return fmt.Errorf("command with ID %s not found", id)
	}

	// Cancel the context
	runningCmd.Cancel()

	// Try to kill the process
	if runningCmd.Cmd.Process != nil {
		return runningCmd.Cmd.Process.Kill()
	}

	return nil
}

// KillAllCommands kills all running commands
func (ce *CommandExecutor) KillAllCommands() error {
	ce.mu.RLock()
	ids := make([]string, 0, len(ce.runningCommands))
	for id := range ce.runningCommands {
		ids = append(ids, id)
	}
	ce.mu.RUnlock()

	var lastErr error
	for _, id := range ids {
		if err := ce.KillCommand(id); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// WaitForAllCommands waits for all running commands to complete or timeout
func (ce *CommandExecutor) WaitForAllCommands(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		ce.mu.RLock()
		count := ce.currentCount
		ce.mu.RUnlock()

		if count == 0 {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for commands to complete")
}

// SetMaxConcurrent sets the maximum number of concurrent commands
func (ce *CommandExecutor) SetMaxConcurrent(max int) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.maxConcurrent = max
}

// SetTimeout sets the command execution timeout
func (ce *CommandExecutor) SetTimeout(timeout time.Duration) {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.timeout = timeout
}