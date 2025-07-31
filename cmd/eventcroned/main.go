// Package main implements the eventcrone daemon (eventcroned) in Go
package main

import (
	"flag"
	"fmt"
	"log"
	"log/syslog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/dpvpro/eventcrone/pkg/eventcrone"
)

const (
	defaultConfigFile    = "/etc/eventcrone.conf"
	defaultPidFile       = "/var/run/eventcroned.pid"
	defaultMaxConcurrent = 32
	defaultTimeout       = 300 // 5 minutes
)

// Config holds daemon configuration
type Config struct {
	MaxConcurrentCommands int
	CommandTimeout        time.Duration
	LogToSyslog          bool
	LogLevel             string
	PidFile              string
	UserTableDir         string
	SystemTableDir       string
}

// Daemon represents the eventcrone daemon
type Daemon struct {
	config       *Config
	watcher      *eventcrone.Watcher
	executor     *eventcrone.CommandExecutor
	userTables   map[string]*eventcrone.IncronTable
	systemTables map[string]*eventcrone.IncronTable
	logger       *log.Logger
	mu           sync.RWMutex
	shutdown     chan struct{}
	done         chan struct{}
}

func main() {
	var (
		configFile = flag.String("f", defaultConfigFile, "Configuration file path")
		foreground = flag.Bool("n", false, "Run in foreground (don't daemonize)")
		pidFile    = flag.String("p", defaultPidFile, "PID file path")
		version    = flag.Bool("V", false, "Show version and exit")
		help       = flag.Bool("h", false, "Show help and exit")
	)
	flag.Parse()

	if *help {
		fmt.Printf("Usage: %s [options]\n", os.Args[0])
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		fmt.Println("\neventcrone daemon (eventcroned) version", eventcrone.Version)
		os.Exit(0)
	}

	if *version {
		fmt.Printf("eventcroned %s\n", eventcrone.Version)
		os.Exit(0)
	}

	// Check root privileges
	if err := eventcrone.CheckRootPrivileges(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Load configuration
	config := loadConfig(*configFile)
	config.PidFile = *pidFile

	// Setup logging
	logger, err := setupLogging(config.LogToSyslog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to setup logging: %v\n", err)
		os.Exit(1)
	}

	// Setup directories and permissions
	if err := eventcrone.SetupPermissions(); err != nil {
		logger.Printf("Failed to setup permissions: %v", err)
		os.Exit(1)
	}

	// Create daemon
	daemon := &Daemon{
		config:       config,
		userTables:   make(map[string]*eventcrone.IncronTable),
		systemTables: make(map[string]*eventcrone.IncronTable),
		logger:       logger,
		shutdown:     make(chan struct{}),
		done:         make(chan struct{}),
	}

	// Daemonize if not running in foreground
	if !*foreground {
		if err := daemonize(); err != nil {
			logger.Printf("Failed to daemonize: %v", err)
			os.Exit(1)
		}
	}

	// Write PID file
	if err := writePidFile(config.PidFile); err != nil {
		logger.Printf("Failed to write PID file: %v", err)
		os.Exit(1)
	}
	defer removePidFile(config.PidFile)

	// Initialize daemon
	if err := daemon.Initialize(); err != nil {
		logger.Printf("Failed to initialize daemon: %v", err)
		os.Exit(1)
	}

	// Setup signal handling
	go daemon.handleSignals()

	// Start daemon
	logger.Printf("eventcroned %s starting up", eventcrone.Version)
	if err := daemon.Run(); err != nil {
		logger.Printf("Daemon error: %v", err)
		os.Exit(1)
	}

	logger.Printf("eventcroned %s shutting down", eventcrone.Version)
}

// loadConfig loads configuration from file or returns defaults
func loadConfig(configFile string) *Config {
	config := &Config{
		MaxConcurrentCommands: defaultMaxConcurrent,
		CommandTimeout:        time.Duration(defaultTimeout) * time.Second,
		LogToSyslog:          true,
		LogLevel:             "info",
		UserTableDir:         eventcrone.DefaultUserTableDir,
		SystemTableDir:       eventcrone.DefaultSystemTableDir,
	}

	// TODO: Implement actual config file parsing
	// For now, return defaults
	return config
}

// setupLogging sets up logging to syslog or stderr
func setupLogging(useSyslog bool) (*log.Logger, error) {
	if useSyslog {
		syslogWriter, err := syslog.New(syslog.LOG_DAEMON|syslog.LOG_INFO, "eventcroned")
		if err != nil {
			return nil, fmt.Errorf("failed to connect to syslog: %v", err)
		}
		return log.New(syslogWriter, "", 0), nil
	}
	return log.New(os.Stderr, "eventcroned: ", log.LstdFlags), nil
}

// daemonize turns the process into a daemon
func daemonize() error {
	// Fork the process
	pid, err := syscall.ForkExec(os.Args[0], os.Args, &syscall.ProcAttr{
		Env:   os.Environ(),
		Files: []uintptr{0, 1, 2}, // stdin, stdout, stderr
		Sys: &syscall.SysProcAttr{
			Setsid: true,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to fork: %v", err)
	}

	if pid > 0 {
		// Parent process exits
		os.Exit(0)
	}

	// Child process continues
	// Change working directory to root
	if err := os.Chdir("/"); err != nil {
		return fmt.Errorf("failed to change directory: %v", err)
	}

	// Close standard file descriptors
	syscall.Close(0)
	syscall.Close(1)
	syscall.Close(2)

	// Redirect to /dev/null
	devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open /dev/null: %v", err)
	}
	defer devNull.Close()

	syscall.Dup2(int(devNull.Fd()), 0)
	syscall.Dup2(int(devNull.Fd()), 1)
	syscall.Dup2(int(devNull.Fd()), 2)

	return nil
}

// writePidFile writes the current PID to the specified file
func writePidFile(pidFile string) error {
	pid := os.Getpid()
	return os.WriteFile(pidFile, []byte(fmt.Sprintf("%d\n", pid)), 0644)
}

// removePidFile removes the PID file
func removePidFile(pidFile string) {
	os.Remove(pidFile)
}

// Initialize initializes the daemon
func (d *Daemon) Initialize() error {
	// Create inotify watcher
	watcher, err := eventcrone.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create watcher: %v", err)
	}
	d.watcher = watcher

	// Create command executor
	d.executor = eventcrone.NewCommandExecutor(
		d.config.MaxConcurrentCommands,
		d.config.CommandTimeout,
	)

	// Load tables
	if err := d.LoadTables(); err != nil {
		return fmt.Errorf("failed to load tables: %v", err)
	}

	// Start watcher
	if err := d.watcher.Start(); err != nil {
		return fmt.Errorf("failed to start watcher: %v", err)
	}

	return nil
}

// LoadTables loads all user and system tables
func (d *Daemon) LoadTables() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Clear existing tables
	d.userTables = make(map[string]*eventcrone.IncronTable)
	d.systemTables = make(map[string]*eventcrone.IncronTable)

	// Load user tables
	userTables, err := eventcrone.LoadAllUserTables()
	if err != nil {
		d.logger.Printf("Warning: failed to load user tables: %v", err)
	} else {
		d.userTables = userTables
	}

	// Load system tables
	systemTables, err := eventcrone.LoadAllSystemTables()
	if err != nil {
		d.logger.Printf("Warning: failed to load system tables: %v", err)
	} else {
		d.systemTables = systemTables
	}

	// Setup watches for all tables
	totalEntries := 0
	for username, table := range d.userTables {
		for _, entry := range table.Entries {
			if err := d.watcher.AddWatch(&entry); err != nil {
				d.logger.Printf("Warning: failed to add watch for user %s, path %s: %v",
					username, entry.Path, err)
			} else {
				totalEntries++
			}
		}
	}

	for tableName, table := range d.systemTables {
		for _, entry := range table.Entries {
			if err := d.watcher.AddWatch(&entry); err != nil {
				d.logger.Printf("Warning: failed to add watch for system table %s, path %s: %v",
					tableName, entry.Path, err)
			} else {
				totalEntries++
			}
		}
	}

	d.logger.Printf("Loaded %d user tables, %d system tables, %d total entries",
		len(d.userTables), len(d.systemTables), totalEntries)

	return nil
}

// Run starts the main daemon loop
func (d *Daemon) Run() error {
	d.logger.Printf("Starting main event loop")

	for {
		select {
		case event := <-d.watcher.Events():
			go d.handleEvent(event)

		case err := <-d.watcher.Errors():
			d.logger.Printf("Watcher error: %v", err)

		case <-d.shutdown:
			d.logger.Printf("Shutdown signal received")
			return d.Stop()

		}
	}
}

// handleEvent processes an inotify event
func (d *Daemon) handleEvent(event *eventcrone.InotifyEvent) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Find matching entries in user tables
	for username, table := range d.userTables {
		for _, entry := range table.Entries {
			if d.eventMatches(&entry, event) {
				// Check user permissions
				allowed, err := eventcrone.CheckUserPermission(username)
				if err != nil {
					d.logger.Printf("Error checking permissions for user %s: %v", username, err)
					continue
				}
				if !allowed {
					d.logger.Printf("User %s not allowed to use eventcrone", username)
					continue
				}

				// Execute command
				go d.executeCommand(&entry, event, username)
			}
		}
	}

	// Find matching entries in system tables
	for _, table := range d.systemTables {
		for _, entry := range table.Entries {
			if d.eventMatches(&entry, event) {
				// System commands run as root
				go d.executeCommand(&entry, event, "root")
			}
		}
	}
}

// eventMatches checks if an event matches an eventcrone entry
func (d *Daemon) eventMatches(entry *eventcrone.IncronEntry, event *eventcrone.InotifyEvent) bool {
	// Check if the path matches
	if !entry.MatchesPath(event.WatchDir) && !entry.MatchesPath(event.Path) {
		return false
	}

	// Check if the event mask matches
	if entry.Mask&event.Mask == 0 {
		return false
	}

	return true
}

// executeCommand executes a command for an eventcrone entry
func (d *Daemon) executeCommand(entry *eventcrone.IncronEntry, event *eventcrone.InotifyEvent, username string) {
	result, err := d.executor.Execute(entry, event, username)
	if err != nil {
		d.logger.Printf("Failed to execute command for user %s: %v", username, err)
		return
	}

	if !result.Success {
		d.logger.Printf("Command failed for user %s (exit code %d): %v",
			username, result.ExitCode, result.Error)
	} else {
		d.logger.Printf("Command executed successfully for user %s (duration: %v)",
			username, result.Duration)
	}
}

// handleSignals sets up signal handling
func (d *Daemon) handleSignals() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	for sig := range sigChan {
		switch sig {
		case syscall.SIGTERM, syscall.SIGINT:
			d.logger.Printf("Received %v signal, shutting down", sig)
			close(d.shutdown)
			return

		case syscall.SIGHUP:
			d.logger.Printf("Received SIGHUP signal, reloading tables")
			if err := d.LoadTables(); err != nil {
				d.logger.Printf("Failed to reload tables: %v", err)
			} else {
				d.logger.Printf("Tables reloaded successfully")
			}
		}
	}
}

// Stop stops the daemon gracefully
func (d *Daemon) Stop() error {
	d.logger.Printf("Stopping daemon...")

	// Stop accepting new events
	if err := d.watcher.Stop(); err != nil {
		d.logger.Printf("Error stopping watcher: %v", err)
	}

	// Wait for running commands to complete (with timeout)
	if err := d.executor.WaitForAllCommands(30 * time.Second); err != nil {
		d.logger.Printf("Timeout waiting for commands, killing remaining: %v", err)
		d.executor.KillAllCommands()
	}

	close(d.done)
	return nil
}
