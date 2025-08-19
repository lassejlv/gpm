package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
)

func installPackage(pm *PackageManager, packageSpec string, isDev bool, writeToPackageJSON bool, installDeps bool, lockFile *LockFile, timer *Timer) error {
	var name, version string

	if strings.HasPrefix(packageSpec, "@") {
		parts := strings.SplitN(packageSpec, "@", 3)
		if len(parts) == 2 {
			name = "@" + parts[1]
			version = "latest"
		} else if len(parts) == 3 {
			name = "@" + parts[1]
			version = parts[2]
		} else {
			name = packageSpec
			version = "latest"
		}
	} else {
		nameParts := strings.Split(packageSpec, "@")
		name = nameParts[0]
		version = "latest"
		if len(nameParts) > 1 {
			version = nameParts[1]
		}
	}

	existingVersion := lockFile.getPackageVersion(name)
	if existingVersion != "" && isPackageInstalled(filepath.Join("node_modules", name), existingVersion) {
		fmt.Printf(" %s %s@%s %s\n", color.HiGreenString("✓"), color.CyanString(name), color.HiBlackString(existingVersion), color.HiBlackString("(cached)"))
		return nil
	}

	if timer != nil {
		timer.Pause()
	}

	installedVersion, wasCached, err := pm.Install(name, version)

	if timer != nil {
		timer.Resume()
	}

	if err != nil {
		return err
	}

	if wasCached {
		fmt.Printf(" %s %s@%s %s\n", color.HiGreenString("✓"), color.CyanString(name), color.HiBlackString(installedVersion), color.HiBlackString("(from cache)"))
		return nil
	}

	if installDeps {
		if err := pm.InstallDependencies(name, lockFile); err != nil {
			fmt.Print("\r                                                    \r")
			fmt.Printf(" %s Warning: Failed to install some dependencies for %s: %v\n", color.YellowString("⚠"), name, err)
		}
	}

	originalSpec := packageSpec
	if version == "latest" {
		originalSpec = name
	}

	if err := lockFile.addPackage(name, installedVersion, originalSpec, isDev); err != nil {
		fmt.Print("\r                                                    \r")
		fmt.Printf(" %s Failed to update lockfile: %v\n", color.YellowString("⚠"), err)
	}

	if writeToPackageJSON {
		if err := updatePackageJSON(name, installedVersion, isDev); err != nil {
			fmt.Print("\r                                                    \r")
			fmt.Printf(" %s Failed to update package.json: %v\n", color.YellowString("⚠"), err)
			return nil
		}
	}

	fmt.Print("\r                                                    \r")
	fmt.Printf(" %s %s@%s %s\n",
		color.HiGreenString("✓"),
		color.CyanString(name),
		color.HiBlackString(installedVersion),
		color.GreenString("added"))

	return nil
}

func installFromPackageJSON(pm *PackageManager, lockFile *LockFile) error {
	timer := NewTimer()
	timer.Start()
	data, err := os.ReadFile("package.json")
	if err != nil {
		return fmt.Errorf("failed to read package.json: %v", err)
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("failed to parse package.json: %v", err)
	}

	totalPackages := len(pkg.Dependencies) + len(pkg.DevDependencies)
	if totalPackages == 0 {
		fmt.Println("No dependencies found in package.json")
		return nil
	}

	fmt.Printf("\n %s Installing %d packages\n", color.CyanString("⚡"), totalPackages)

	for name, version := range pkg.Dependencies {
		packageSpec := name
		if version != "" && version != "latest" {
			cleanVersion := strings.TrimPrefix(strings.TrimPrefix(version, "^"), "~")
			if cleanVersion != version && cleanVersion != "" {
				packageSpec = name + "@" + cleanVersion
			}
		}

		if err := installPackage(pm, packageSpec, false, false, true, lockFile, timer); err != nil {
			fmt.Printf(" %s Failed to install %s: %v\n", color.RedString("✗"), color.CyanString(name), err)
			return err
		}
	}

	for name, version := range pkg.DevDependencies {
		packageSpec := name
		if version != "" && version != "latest" {
			cleanVersion := strings.TrimPrefix(strings.TrimPrefix(version, "^"), "~")
			if cleanVersion != version && cleanVersion != "" {
				packageSpec = name + "@" + cleanVersion
			}
		}

		if err := installPackage(pm, packageSpec, true, false, true, lockFile, timer); err != nil {
			fmt.Printf(" %s Failed to install dev dependency %s: %v\n", color.RedString("✗"), color.CyanString(name), err)
			return err
		}
	}

	if err := lockFile.saveLockFile(); err != nil {
		fmt.Printf(" %s Failed to save lockfile: %v\n", color.YellowString("⚠"), err)
	}

	elapsed := timer.Stop()
	fmt.Printf("\n %s All packages installed successfully! %s %s\n",
		color.HiGreenString("✓"),
		color.HiGreenString("🎉"),
		color.HiBlackString(fmt.Sprintf("(%s)", formatDuration(elapsed))))
	return nil
}

func isPackageInstalled(packagePath, version string) bool {
	packageJSONPath := filepath.Join(packagePath, "package.json")

	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return false
	}

	var pkg struct {
		Version string `json:"version"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}

	return pkg.Version == version
}
