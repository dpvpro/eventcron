// Package incron provides user permission checking functionality
package incron

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"strings"
	"syscall"
)

// CheckUserPermission checks if a user has permission to use incron
// This implements the same logic as the original C++ version:
// 1. If allow file exists, user must be listed there
// 2. If deny file exists, user must NOT be listed there
// 3. If neither exists, all users are allowed
func CheckUserPermission(username string) (bool, error) {
	// Check if allow file exists
	allowExists := fileExists(DefaultAllowFile)
	denyExists := fileExists(DefaultDenyFile)
	
	if allowExists {
		// If allow file exists, user must be explicitly allowed
		allowed, err := userInFile(username, DefaultAllowFile)
		if err != nil {
			return false, fmt.Errorf("error reading allow file: %v", err)
		}
		return allowed, nil
	}
	
	if denyExists {
		// If deny file exists, user must NOT be denied
		denied, err := userInFile(username, DefaultDenyFile)
		if err != nil {
			return false, fmt.Errorf("error reading deny file: %v", err)
		}
		return !denied, nil
	}
	
	// If neither file exists, all users are allowed
	return true, nil
}

// userInFile checks if a username is listed in the given file
func userInFile(username, filePath string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer file.Close()
	
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Check if this line matches the username
		if line == username {
			return true, nil
		}
	}
	
	return false, scanner.Err()
}

// fileExists checks if a file exists
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

// GetCurrentUser returns the current user's username
func GetCurrentUser() (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("failed to get current user: %v", err)
	}
	return currentUser.Username, nil
}

// IsRoot checks if the current process is running as root
func IsRoot() bool {
	return os.Geteuid() == 0
}

// GetUserByName looks up a user by username
func GetUserByName(username string) (*user.User, error) {
	return user.Lookup(username)
}

// GetUserByUID looks up a user by UID
func GetUserByUID(uid string) (*user.User, error) {
	return user.LookupId(uid)
}

// CanAccessPath checks if a user can access the given path
// This is a simplified version - in practice, you might want to
// implement more sophisticated permission checking
func CanAccessPath(username, path string) (bool, error) {
	userInfo, err := GetUserByName(username)
	if err != nil {
		return false, fmt.Errorf("user not found: %s", username)
	}
	
	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil // Path doesn't exist
		}
		return false, err
	}
	
	// Get file ownership and permissions
	stat := info.Sys().(*syscall.Stat_t)
	
	// Convert user info
	uid := fmt.Sprintf("%d", stat.Uid)
	gid := fmt.Sprintf("%d", stat.Gid)
	
	// Check if user owns the file
	if userInfo.Uid == uid {
		return true, nil
	}
	
	// Check if user is in the file's group
	if userInfo.Gid == gid {
		return true, nil
	}
	
	// Check world permissions (simplified - just check if others can read)
	mode := info.Mode()
	if mode&0004 != 0 { // Others can read
		return true, nil
	}
	
	return false, nil
}

// SetupPermissions creates necessary directories and sets proper permissions
func SetupPermissions() error {
	// Create user table directory
	if err := os.MkdirAll(DefaultUserTableDir, 0755); err != nil {
		return fmt.Errorf("failed to create user table directory: %v", err)
	}
	
	// Create system table directory
	if err := os.MkdirAll(DefaultSystemTableDir, 0755); err != nil {
		return fmt.Errorf("failed to create system table directory: %v", err)
	}
	
	// Set proper ownership for user table directory (root:root)
	if err := os.Chown(DefaultUserTableDir, 0, 0); err != nil {
		return fmt.Errorf("failed to set ownership for user table directory: %v", err)
	}
	
	// Set proper ownership for system table directory (root:root)
	if err := os.Chown(DefaultSystemTableDir, 0, 0); err != nil {
		return fmt.Errorf("failed to set ownership for system table directory: %v", err)
	}
	
	return nil
}

// CheckRootPrivileges ensures the current process has root privileges
func CheckRootPrivileges() error {
	if !IsRoot() {
		return fmt.Errorf("this program must be run as root")
	}
	return nil
}

// DropPrivileges drops root privileges to the specified user
func DropPrivileges(username string) error {
	if !IsRoot() {
		return fmt.Errorf("cannot drop privileges: not running as root")
	}
	
	userInfo, err := GetUserByName(username)
	if err != nil {
		return fmt.Errorf("failed to lookup user %s: %v", username, err)
	}
	
	// Parse UID and GID
	uid, err := parseUID(userInfo.Uid)
	if err != nil {
		return fmt.Errorf("invalid UID for user %s: %v", username, err)
	}
	
	gid, err := parseGID(userInfo.Gid)
	if err != nil {
		return fmt.Errorf("invalid GID for user %s: %v", username, err)
	}
	
	// Set GID first
	if err := syscall.Setgid(gid); err != nil {
		return fmt.Errorf("failed to set GID: %v", err)
	}
	
	// Set UID
	if err := syscall.Setuid(uid); err != nil {
		return fmt.Errorf("failed to set UID: %v", err)
	}
	
	return nil
}

// parseUID parses a UID string to int
func parseUID(uidStr string) (int, error) {
	var uid int
	_, err := fmt.Sscanf(uidStr, "%d", &uid)
	return uid, err
}

// parseGID parses a GID string to int
func parseGID(gidStr string) (int, error) {
	var gid int
	_, err := fmt.Sscanf(gidStr, "%d", &gid)
	return gid, err
}