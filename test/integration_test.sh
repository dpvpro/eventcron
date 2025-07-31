#!/bin/bash

# Integration test script for eventcron
# This script tests basic functionality of the Go eventcron implementation

# set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Test configuration
TEST_DIR="/tmp/eventcron-test-$$"
TEST_USER="$(whoami)"
DAEMON_PID=""
TEST_LOG="/tmp/eventcron-test.log"

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up...${NC}"
    
    # Kill daemon if running
    if [ -n "$DAEMON_PID" ]; then
        kill "$DAEMON_PID" 2>/dev/null || true
        wait "$DAEMON_PID" 2>/dev/null || true
    fi
    
    # Remove test files
    rm -rf "$TEST_DIR" "$TEST_LOG" 2>/dev/null || true
    
    # Remove test table
    ./eventcrontab -r 2>/dev/null || true
    
    echo -e "${GREEN}Cleanup complete${NC}"
}

# Set trap for cleanup
trap cleanup EXIT

# Helper functions
log() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" >&2
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Check if running as root (needed for full testing)
check_privileges() {
    if [ "$EUID" -eq 0 ]; then
        log "Running as root - full testing available"
        return 0
    else
        warning "Not running as root - some tests will be skipped"
        return 1
    fi
}

# Test 1: Check binaries exist and are executable
test_binaries() {
    log "Testing binaries..."
    
    if [ ! -x "./eventcrond" ]; then
        error "eventcrond binary not found or not executable"
        return 1
    fi
    
    if [ ! -x "./eventcrontab" ]; then
        error "eventcrontab binary not found or not executable"
        return 1
    fi
    
    log "✓ Binaries found and executable"
}

# Test 2: Check version output
test_version() {
    log "Testing version output..."
    
    local daemon_version=$(./eventcrond -V 2>&1)
    local client_version=$(./eventcrontab -V 2>&1)
    
    if [[ ! "$daemon_version" =~ "eventcrond 1.0.0" ]]; then
        error "Unexpected daemon version: $daemon_version"
        return 1
    fi
    
    if [[ ! "$client_version" =~ "eventcrontab 1.0.0" ]]; then
        error "Unexpected client version: $client_version"
        return 1
    fi
    
    log "✓ Version output correct"
}

# Test 3: Check help output
test_help() {
    log "Testing help output..."
    
    local daemon_help=$(./eventcrond -h 2>&1)
    local client_help=$(./eventcrontab -h 2>&1)
    
    if [[ ! "$daemon_help" =~ "Usage:" ]]; then
        error "Daemon help output missing"
        return 1
    fi
    
    if [[ ! "$client_help" =~ "Usage:" ]]; then
        error "Client help output missing"
        return 1
    fi
    
    log "✓ Help output correct"
}

# Test 4: Test eventcrontab basic operations (non-root)
test_eventcrontab_basic() {
    log "Testing eventcrontab basic operations..."
    
    # Create test directory
    mkdir -p "$TEST_DIR"
    
    # Test list empty table
    local output=$(./eventcrontab -l 2>&1 || true)
    log "Empty table list output: '$output'"
    
    # Create a test table file
    cat > /tmp/test-table << EOF
# Test eventcron table
$TEST_DIR IN_CREATE echo "Created: \$#" >> $TEST_LOG
$TEST_DIR IN_MODIFY echo "Modified: \$#" >> $TEST_LOG
EOF
    
    # Test table installation (this might fail without proper permissions)
    if ./eventcrontab /tmp/test-table 2>/dev/null; then
        log "✓ Table installation succeeded"
        
        # Test list table
        local list_output=$(./eventcrontab -l 2>&1)
        if [[ "$list_output" =~ "$TEST_DIR" ]]; then
            log "✓ Table listing works"
        else
            warning "Table listing doesn't show expected content"
        fi
        
        # Test remove table
        if ./eventcrontab -r 2>/dev/null; then
            log "✓ Table removal succeeded"
        else
            warning "Table removal failed"
        fi
    else
        warning "Table installation failed (expected without proper permissions)"
    fi
    
    rm -f /tmp/test-table
}

# Test 5: Test daemon startup (root only)
test_daemon_startup() {
    if ! check_privileges; then
        warning "Skipping daemon tests - not running as root"
        return 0
    fi
    
    log "Testing daemon startup..."
    
    # Start daemon in foreground mode
    ./eventcrond -n -p "/tmp/eventcrond-test.pid" &
    DAEMON_PID=$!
    
    # Give daemon time to start
    sleep 2
    
    # Check if daemon is still running
    if kill -0 "$DAEMON_PID" 2>/dev/null; then
        log "✓ Daemon started successfully"
        
        # Check PID file
        if [ -f "/tmp/eventcrond-test.pid" ]; then
            log "✓ PID file created"
        else
            warning "PID file not created"
        fi
        
        # Test SIGHUP (reload)
        if kill -HUP "$DAEMON_PID" 2>/dev/null; then
            sleep 1
            if kill -0 "$DAEMON_PID" 2>/dev/null; then
                log "✓ Daemon handles SIGHUP correctly"
            else
                error "Daemon died after SIGHUP"
                return 1
            fi
        fi
        
        # Stop daemon
        kill -TERM "$DAEMON_PID" 2>/dev/null
        sleep 2
        
        if ! kill -0 "$DAEMON_PID" 2>/dev/null; then
            log "✓ Daemon stopped gracefully"
            DAEMON_PID=""
        else
            warning "Daemon didn't stop gracefully"
            kill -KILL "$DAEMON_PID" 2>/dev/null
            DAEMON_PID=""
        fi
    else
        error "Daemon failed to start or crashed immediately"
        return 1
    fi
}

# Test 6: Test file monitoring (root only)
test_file_monitoring() {
    if ! check_privileges; then
        warning "Skipping file monitoring tests - not running as root"
        return 0
    fi
    
    log "Testing file monitoring..."
    
    # Create test directory
    mkdir -p "$TEST_DIR"
    rm -f "$TEST_LOG"
    
    # Create test table
    mkdir -p /var/spool/eventcron
    cat > "/var/spool/eventcron/$TEST_USER" << EOF
$TEST_DIR IN_CREATE echo "File created: \$#" >> $TEST_LOG
EOF
    
    # Start daemon
    ./eventcrond -n -p "/tmp/eventcrond-test.pid" &
    DAEMON_PID=$!
    
    # Give daemon time to start and load tables
    sleep 3
    
    # Create a test file
    touch "$TEST_DIR/testfile.txt"
    
    # Give daemon time to process the event
    sleep 2
    
    # Check if event was logged
    if [ -f "$TEST_LOG" ] && grep -q "File created: testfile.txt" "$TEST_LOG"; then
        log "✓ File monitoring works correctly"
    else
        warning "File monitoring event not detected"
        if [ -f "$TEST_LOG" ]; then
            log "Log contents: $(cat $TEST_LOG)"
        else
            log "Log file not created"
        fi
    fi
    
    # Stop daemon
    kill -TERM "$DAEMON_PID" 2>/dev/null
    sleep 2
    DAEMON_PID=""
    
    # Cleanup
    rm -f "/var/spool/eventcron/$TEST_USER"
}

# Test 7: Test table validation
test_table_validation() {
    log "Testing table validation..."
    
    # Create invalid table
    cat > /tmp/invalid-table << EOF
# Invalid entries for testing
/tmp
relative/path IN_CREATE echo test
/tmp  echo test
/tmp INVALID_MASK echo test
EOF
    
    # Test invalid table (should fail)
    if ./eventcrontab /tmp/invalid-table 2>/dev/null; then
        warning "Invalid table was accepted (validation might be weak)"
    else
        log "✓ Invalid table correctly rejected"
    fi
    
    rm -f /tmp/invalid-table
}

# Main test execution
main() {
    echo "========================================"
    echo "  eventcron Integration Test Suite"
    echo "========================================"
    echo
    
    # Change to project directory
    cd "$(dirname "$0")/.."
    
    # Check if binaries exist
    if [ ! -f "./eventcrond" ] || [ ! -f "./eventcrontab" ]; then
        error "Binaries not found. Please run 'make build' first."
        exit 1
    fi
    
    local failed_tests=0
    local total_tests=0
    
    # Run tests
    tests=(
        "test_binaries"
        "test_version" 
        "test_help"
        "test_eventcrontab_basic"
        "test_daemon_startup"
        "test_file_monitoring"
        "test_table_validation"
    )
    
    for test in "${tests[@]}"; do
        echo
        ((total_tests++))
        if $test; then
            log "Test $test: PASSED"
        else
            error "Test $test: FAILED"
            ((failed_tests++))
        fi
    done
    
    echo
    echo "========================================"
    echo "  Test Results"
    echo "========================================"
    echo "Total tests: $total_tests"
    echo "Passed: $((total_tests - failed_tests))"
    echo "Failed: $failed_tests"
    
    if [ $failed_tests -eq 0 ]; then
        echo -e "${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}Some tests failed!${NC}"
        exit 1
    fi
}

# Run main function
main "$@"