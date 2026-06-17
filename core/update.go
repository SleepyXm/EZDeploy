package core

import (
	"fmt"
	"runtime"
)

// UpdateSystem runs `apt update` followed by `apt upgrade -y` (or the dnf
// equivalent) so package lists and installed packages are current before
// any install steps run. This only applies on Linux, where EZDeploy is
// actually managing a deployment target — on Windows/Mac, which is local
// development against the binary rather than a real deploy target, this
// is a no-op.
func UpdateSystem() error {
	if runtime.GOOS != "linux" {
		fmt.Println("[!] Non-Linux OS detected, skipping system update (dev environment)")
		return nil
	}

	pm, ok := getPackageManager()
	if !ok {
		return fmt.Errorf("[!] unsupported Linux distro — cannot update system")
	}

	if pm == "apt" {
		fmt.Println("[→] Updating package lists...")
		if err := run("sudo", "apt", "update", "-y"); err != nil {
			return fmt.Errorf("apt update: %w", err)
		}
		fmt.Println("[✓] Package lists updated")

		fmt.Println("[→] Upgrading installed packages...")
		if err := run("sudo", "apt", "upgrade", "-y"); err != nil {
			return fmt.Errorf("apt upgrade: %w", err)
		}
		fmt.Println("[✓] Packages upgraded")
		return nil
	}

	// dnf combines refresh + upgrade into a single command.
	fmt.Println("[→] Updating and upgrading packages...")
	if err := run("sudo", "dnf", "update", "-y"); err != nil {
		return fmt.Errorf("dnf update: %w", err)
	}
	fmt.Println("[✓] Packages updated")

	return nil
}
