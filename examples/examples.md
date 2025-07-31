# eventcrone Usage Examples

This document provides practical examples of using eventcrone for various monitoring scenarios.

## Basic Examples

### 1. Monitor File Creation in a Directory

Monitor when new files are created in `/tmp` and log them:

```bash
# Add to incron table
incrontab -e

# Add this line:
/tmp IN_CREATE logger "New file created: $@/$#"
```

### 2. Backup Files When Modified

Automatically backup files when they are modified:

```bash
# Monitor documents directory
/home/user/documents IN_CLOSE_WRITE cp "$@/$#" /backup/$(date +%Y%m%d-%H%M%S)-$#
```

### 3. Process Uploaded Files

Monitor an upload directory and process new files:

```bash
/var/www/uploads IN_MOVED_TO /usr/local/bin/process-upload.sh "$@/$#"
```

## Advanced Examples

### 4. Recursive Directory Monitoring

Monitor all subdirectories for changes (useful for development):

```bash
# Monitor source code changes
/home/user/projects IN_MODIFY,recursive=true,dotdirs=false /usr/local/bin/auto-build.sh "$@" "$#"
```

### 5. Configuration File Monitoring with Service Restart

Monitor configuration files and restart services safely:

```bash
# Monitor nginx config
/etc/nginx/nginx.conf IN_MODIFY,loopable=false nginx -t && systemctl reload nginx

# Monitor application config  
/etc/myapp/config.yml IN_MODIFY,loopable=false systemctl restart myapp
```

### 6. Log File Rotation Trigger

Automatically set up log rotation for new log files:

```bash
/var/log/applications IN_CREATE /usr/local/bin/setup-logrotate.sh "$@/$#"
```

### 7. Media File Processing

Process media files when they are uploaded:

```bash
# Convert images to thumbnails
/media/photos IN_MOVED_TO convert "$@/$#" -resize 200x200 "/media/thumbnails/thumb_$#"

# Process videos
/media/videos IN_CLOSE_WRITE ffmpeg -i "$@/$#" -vcodec libx264 "/media/processed/processed_$#"
```

### 8. Database Import Automation

Monitor for new database dump files and import them:

```bash
/data/imports IN_CREATE,IN_MOVED_TO /usr/local/bin/import-data.sh "$@/$#"
```

### 9. Security Monitoring

Monitor system directories for unauthorized changes:

```bash
# Monitor /etc for changes
/etc IN_MODIFY,IN_CREATE,IN_DELETE,recursive=false logger -p security.warning "System file changed: $@/$# (event: $%)"

# Monitor user home directories
/home IN_CREATE,IN_DELETE,recursive=true,dotdirs=true logger -p security.info "User file activity: $@/$# ($%)"
```

### 10. Development Workflow Automation

Automate development tasks based on file changes:

```bash
# Auto-run tests when source files change
/project/src IN_MODIFY,recursive=true make test

# Auto-generate documentation
/project/docs/source IN_MODIFY,recursive=true make docs

# Auto-deploy when build completes
/project/build IN_CREATE deploy.sh "$@/$#"
```

## System Administration Examples

### 11. Log Monitoring and Alerting

Monitor log files for specific patterns:

```bash
# Create a script that monitors log content
/var/log/application.log IN_MODIFY /usr/local/bin/check-log-errors.sh "$@/$#"
```

Where `check-log-errors.sh` might look like:
```bash
#!/bin/bash
if tail -n 10 "$1" | grep -i "error\|critical\|fatal"; then
    mail -s "Application Error Detected" admin@company.com < /dev/null
fi
```

### 12. Backup Automation

Trigger backups when important files change:

```bash
# Backup database when it's modified
/var/lib/mysql IN_CLOSE_WRITE,recursive=false /usr/local/bin/backup-mysql.sh

# Backup configuration when changed
/etc IN_MODIFY,recursive=true rsync -av /etc/ backup@server:/backups/etc/$(date +%Y%m%d-%H%M%S)/
```

### 13. File Organization

Automatically organize files by type:

```bash
# Organize downloads by file extension
/home/user/Downloads IN_MOVED_TO /usr/local/bin/organize-by-extension.sh "$@/$#"
```

Where `organize-by-extension.sh` might look like:
```bash
#!/bin/bash
file="$1"
ext="${file##*.}"
case "$ext" in
    pdf) mv "$file" "/home/user/Documents/PDFs/" ;;
    jpg|png|gif) mv "$file" "/home/user/Pictures/" ;;
    mp3|wav|flac) mv "$file" "/home/user/Music/" ;;
    mp4|avi|mkv) mv "$file" "/home/user/Videos/" ;;
esac
```

### 14. Container and Microservice Monitoring

Monitor Docker containers and trigger actions:

```bash
# Monitor Docker socket for container events
/var/run/docker.sock IN_MODIFY docker-event-handler.sh

# Monitor kubernetes manifests
/etc/kubernetes/manifests IN_MODIFY,recursive=true kubectl apply -f "$@/$#"
```

### 15. Web Development Automation

Automate web development tasks:

```bash
# Auto-reload development server
/project/web/src IN_MODIFY,recursive=true killall -HUP node

# Auto-compile SASS/CSS
/project/web/scss IN_MODIFY,recursive=true sass "$@/$#" "/project/web/css/${#%.scss}.css"

# Auto-minify JavaScript
/project/web/js/src IN_MODIFY,recursive=true uglifyjs "$@/$#" -o "/project/web/js/dist/$#"
```

## Event Mask Reference

### Common Event Combinations

```bash
# File creation and modification
IN_CREATE,IN_MODIFY

# File closure (useful for editors that write atomically)
IN_CLOSE_WRITE

# All file operations
IN_ALL_EVENTS

# Directory operations only
IN_CREATE,IN_DELETE,IN_ONLYDIR

# File moves (useful for upload directories)
IN_MOVED_TO

# File access tracking
IN_ACCESS
```

### Options Reference

```bash
# Disable loop prevention (allow events during command execution)
IN_CREATE,loopable=true

# Disable recursive monitoring
IN_CREATE,recursive=false

# Include hidden files and directories
IN_CREATE,dotdirs=true

# Combine multiple options
IN_CREATE,recursive=true,dotdirs=false,loopable=false
```

## Wildcard Reference

- `$$` - Literal dollar sign
- `$@` - Full path of the watched directory
- `$#` - Name of the file that triggered the event
- `$%` - Event name in text format (e.g., "IN_CREATE")
- `$&` - Event flags in numeric format

## Best Practices

### 1. Use Absolute Paths
Always use absolute paths in incron tables:
```bash
# Good
/home/user/documents IN_CREATE process.sh "$@/$#"

# Bad
~/documents IN_CREATE process.sh "$@/$#"
```

### 2. Handle Spaces in Filenames
Quote variables to handle spaces:
```bash
/uploads IN_CREATE process.sh "$@/$#"
```

### 3. Use Loop Prevention for File Modifications
When your command might modify the watched files:
```bash
/data IN_MODIFY,loopable=false process-data.sh "$@/$#"
```

### 4. Test Commands Before Adding to incron
Test your commands manually first:
```bash
# Test the command
echo "test" > /tmp/testfile
process.sh "/tmp/testfile"

# Then add to incron
/tmp IN_CREATE process.sh "$@/$#"
```

### 5. Use Logging for Debugging
Add logging to understand what's happening:
```bash
/watch/dir IN_CREATE logger "Processing file: $@/$#" && process.sh "$@/$#"
```

### 6. Consider Performance
For high-frequency events, consider batching:
```bash
# Instead of processing each file immediately
/busy/dir IN_CREATE process-single.sh "$@/$#"

# Consider a batch processor
/busy/dir IN_CREATE touch /tmp/process-queue && process-batch.sh
```

## Troubleshooting

### Check if Events are Being Generated
```bash
# Add a simple logger to see all events
/test/dir IN_ALL_EVENTS logger "Event: $% File: $@/$#"
```

### Debug Command Execution
```bash
# Log command execution
/test/dir IN_CREATE logger "Executing: command $@/$#" && command "$@/$#"
```

### Check incron Status
```bash
# List current table
incrontab -l

# Check daemon status
systemctl status incrond

# Check logs
journalctl -u incrond -f
```
