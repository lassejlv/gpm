package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fatih/color"
)

func uninstallPackage(packageName string, lockFile *LockFile) error {
	nodeModulesPath := "./node_modules"
	packagePath := filepath.Join(nodeModulesPath, packageName)

	if !fileExists(packagePath) {
		fmt.Printf(" %s %s is not installed\n", color.YellowString("⚠"), color.CyanString(packageName))
		return nil
	}

	fmt.Printf(" %s Removing %s...\n", color.RedString("✗"), color.CyanString(packageName))


	bm := NewBinaryManager()
	if err := bm.removePackageBinaries(packageName); err != nil {
		fmt.Printf(" %s Failed to remove binaries for %s: %v\n", color.YellowString("⚠"), packageName, err)
	}

	if err := os.RemoveAll(packagePath); err != nil {
		return fmt.Errorf("failed to remove package directory: %v", err)
	}

	if err := removeFromPackageJSON(packageName); err != nil {
		fmt.Printf(" %s Failed to update package.json: %v\n", color.YellowString("⚠"), err)
	}

	lockFile.removePackage(packageName)

	fmt.Printf(" %s %s %s\n", color.HiGreenString("✓"), color.CyanString(packageName), color.RedString("removed"))
	return nil
}

func removeFromPackageJSON(packageName string) error {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return fmt.Errorf("failed to read package.json: %v", err)
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("failed to parse package.json: %v", err)
	}

	removed := false
	if pkg.Dependencies != nil {
		if _, exists := pkg.Dependencies[packageName]; exists {
			delete(pkg.Dependencies, packageName)
			removed = true
		}
	}

	if pkg.DevDependencies != nil {
		if _, exists := pkg.DevDependencies[packageName]; exists {
			delete(pkg.DevDependencies, packageName)
			removed = true
		}
	}

	if !removed {
		return nil
	}

	updatedData, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal package.json: %v", err)
	}

	if err := os.WriteFile("package.json", updatedData, 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %v", err)
	}

	return nil
}
