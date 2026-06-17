package core

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strings"
)

// OSInfo holds information about the current operating system.
type OSInfo struct {
	System string // e.g. "linux", "windows", "darwin"
	ID     string // distro ID (e.g. "ubuntu", "fedora") or same as System on non-Linux
}

// GetOS detects and returns information about the current OS.
// On Linux it reads /etc/os-release for the distro ID.
// Falls back to runtime.GOOS if the file is absent or unparseable.
func GetOS() OSInfo {
	info := OSInfo{
		System: runtime.GOOS,
		ID:     runtime.GOOS, // sensible default
	}

	fmt.Println("System:", runtime.GOOS)

	if id, ok := parseOSRelease(); ok {
		info.ID = id
		fmt.Println("Distro ID:", id)
	}

	return info
}

// parseOSRelease reads /etc/os-release and extracts the ID field.
// Returns the distro ID and true on success, or "", false otherwise.
func parseOSRelease() (string, bool) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "", false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "ID=") {
			id := strings.TrimPrefix(line, "ID=")
			id = strings.Trim(id, `"`)
			return strings.ToLower(id), true
		}
	}
	return "", false
}
