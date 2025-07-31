# eventcrone

A modern Go implementation of the inotify cron system (incron).

## Overview

eventcrone is a complete rewrite of the original incron C++ project in Go. It provides a daemon (`eventcroned`) that monitors filesystem events using Linux inotify and executes commands when specified events occur, plus a table management utility (`eventcronetab`) similar to crontab.

Unlike traditional cron which runs commands based on time, incron runs commands based on filesystem events like file creation, modification, or deletion.

## Features

- **Modern Go implementation** - Better performance, memory safety, and easier maintenance
- **Full compatibility** - Compatible with original incron table format and wildcards
- **Recursive directory watching** - Monitor entire directory trees
- **Flexible event filtering** - Support for all inotify event types
- **User permissions** - Support for allow/deny files like original incron
- **System tables** - Support for system-wide incron tables in `/etc/incron.d/`
- **Signal handling** - Graceful shutdown and table reloading with SIGHUP
- **Concurrent execution** - Efficient handling of multiple simultaneous events
- **Loop prevention** - Optional protection against infinite event loops

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/dpvpro/eventcrone.git
cd eventcrone

# Build the project
make build

# Install (requires root)
sudo make install
```

### Requirements

- Go 1.21 or later
- Linux with inotify support (kernel 2.6.13+)
- Root privileges for installation and daemon operation

## Usage

### Starting the Daemon

```bash
# Start daemon in foreground (for testing)
sudo incrond -n

# Start daemon in background
sudo incrond

# Check status
systemctl status incrond  # if using systemd
```

### Managing User Tables

The `incrontab` command manages incron tables for users:

```bash
# List current user's table
incrontab -l

# Edit current user's table
incrontab -e

# Remove current user's table
incrontab -r

# Install table from file
incrontab /path/to/table/file

# Edit another user's table (root only)
sudo incrontab -u username -e
```

### Table Format

Each line in an incron table has the format:
```
<path> <mask> <command>
```

**Examples:**

```bash
# Monitor file creation in /tmp
/tmp IN_CREATE echo "File created: $@/$#"

# Monitor file modifications with multiple events
/home/user/documents IN_MODIFY,IN_CLOSE_WRITE backup-script $@/$#

# Recursive monitoring with options
/var/log IN_CREATE,recursive=true,dotdirs=false logger "Log file created: $#"

# Monitor with loop prevention
/data IN_MODIFY,loopable=false process-data $@/$#
```

### Event Masks

Available event masks:

- `IN_ACCESS` - File was accessed
- `IN_MODIFY` - File was modified
- `IN_ATTRIB` - Metadata changed
- `IN_CLOSE_WRITE` - File opened for writing was closed
- `IN_CLOSE_NOWRITE` - File not opened for writing was closed
- `IN_OPEN` - File was opened
- `IN_MOVED_FROM` - File moved from watched directory
- `IN_MOVED_TO` - File moved to watched directory
- `IN_CREATE` - File/directory created
- `IN_DELETE` - File/directory deleted
- `IN_DELETE_SELF` - Watched file/directory deleted
- `IN_MOVE_SELF` - Watched file/directory moved
- `IN_ALL_EVENTS` - All events

### Options

- `recursive=true/false` - Watch subdirectories (default: true)
- `loopable=true/false` - Allow events during command execution (default: false)
- `dotdirs=true/false` - Include hidden directories and files (default: false)

### Command Wildcards

Commands can use these wildcards:

- `$$` - Literal $ character
- `$@` - Watched directory path
- `$#` - Filename that triggered the event
- `$%` - Event name (textual representation)
- `$&` - Event flags (numeric representation)

## Configuration

### Daemon Configuration

The daemon looks for configuration in `/etc/incron.conf` (currently placeholder).

### User Permissions

User access is controlled by:

- `/etc/incron.allow` - If exists, only listed users can use incron
- `/etc/incron.deny` - If exists, listed users cannot use incron
- If neither exists, all users can use incron

### System Tables

System-wide tables can be placed in `/etc/incron.d/`. These run with root privileges and are managed directly (not via incrontab).

## Directories

- `/var/spool/incron/` - User incron tables
- `/etc/incron.d/` - System incron tables
- `/etc/incron.conf` - Configuration file
- `/var/run/incrond.pid` - Daemon PID file

## Systemd Integration

A systemd service file is included:

```bash
# Enable and start the service
sudo systemctl enable incrond
sudo systemctl start incrond

# Reload tables without restart
sudo systemctl reload incrond
```

## Development

### Building

```bash
# Development build with race detector
make dev

# Debug build
make debug

# Run tests
make test

# Run tests with coverage
make test-coverage

# Format and lint code
make check
```

### Cross-compilation

```bash
# Build for multiple platforms
make build-all

# Build for specific platform
GOOS=linux GOARCH=arm64 make build
```

## Differences from Original C++ Version

### Improvements

- **Better performance** - Go's goroutines provide more efficient concurrency than fork/exec
- **Memory safety** - Automatic garbage collection eliminates memory leaks
- **Easier deployment** - Single static binary with no dependencies
- **Better error handling** - More descriptive error messages and logging
- **Modern codebase** - Clean, maintainable Go code

### Compatibility

- **Table format** - 100% compatible with original incron tables
- **Command wildcards** - All wildcards work exactly the same
- **File permissions** - Same permission model using allow/deny files
- **Signal handling** - Same signals (SIGHUP for reload, SIGTERM for shutdown)

### New Features

- **Enhanced logging** - Better structured logging with different levels
- **Improved validation** - Better error messages for invalid table entries
- **Graceful shutdown** - Clean shutdown with command completion waiting
- **Resource limits** - Configurable limits on concurrent commands

## Troubleshooting

### Common Issues

1. **Permission denied**
   - Ensure incrontab has setuid bit: `chmod u+s /usr/local/bin/incrontab`
   - Check user permissions in allow/deny files

2. **Daemon won't start**
   - Check that you're running as root
   - Verify inotify support: `ls /proc/sys/fs/inotify/`
   - Check system logs for error messages

3. **Events not triggering**
   - Verify path exists and is accessible
   - Check event mask matches the events you expect
   - Test with `IN_ALL_EVENTS` to see what events are generated

4. **Commands not executing**
   - Check command syntax and permissions
   - Verify user has permission to execute the command
   - Check system logs for execution errors

### Debugging

```bash
# Run daemon in foreground with verbose logging
sudo incrond -n

# Check what events are being generated
incrontab -e
# Add: /path/to/test IN_ALL_EVENTS logger "Event: $% File: $#"

# Monitor system logs
journalctl -u incrond -f
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests if applicable
5. Run `make check` to verify code quality
6. Submit a pull request

## License

This project is licensed under the same terms as the original incron-next project:
- GNU General Public License v3.0

## Authors

- **Go Implementation**: Based on incron-next by dpvpro
- **Original incron**: Lukas Jelinek (2006-2012)
- **incron-next**: Andreas Altair Redmer (2014-2017), dpvpro (2023+)

## Links

- [Original incron-next](https://github.com/dpvpro/incron-next)
- [Go incron Documentation](https://github.com/dpvpro/incron-next/tree/main/eventcrone)
