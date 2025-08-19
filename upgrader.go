package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
)

type UpgradeManager struct {
	pm       *PackageManager
	lockFile *LockFile
}

type UpgradeInfo struct {
	Name           string
	CurrentVersion string
	LatestVersion  string
	NeedsUpgrade   bool
	IsDev          bool
}

func NewUpgradeManager(pm *PackageManager, lockFile *LockFile) *UpgradeManager {
	return &UpgradeManager{
		pm:       pm,
		lockFile: lockFile,
	}
}

func (um *UpgradeManager) CheckUpgrades(packageNames []string) ([]UpgradeInfo, error) {
	var upgrades []UpgradeInfo

	for _, packageName := range packageNames {
		info, err := um.checkSinglePackage(packageName)
		if err != nil {
			continue
		}
		upgrades = append(upgrades, info)
	}

	return upgrades, nil
}

func (um *UpgradeManager) checkSinglePackage(packageName string) (UpgradeInfo, error) {
	info := UpgradeInfo{Name: packageName}

	currentVersion := um.getCurrentVersion(packageName)
	if currentVersion == "" {
		return info, fmt.Errorf("package not installed")
	}
	info.CurrentVersion = currentVersion

	latestVersion, err := um.getLatestVersion(packageName)
	if err != nil {
		return info, err
	}
	info.LatestVersion = latestVersion

	info.NeedsUpgrade = um.needsUpgrade(currentVersion, latestVersion)
	info.IsDev = um.isDevDependency(packageName)

	return info, nil
}

func (um *UpgradeManager) getCurrentVersion(packageName string) string {
	packagePath := filepath.Join("node_modules", packageName, "package.json")
	if !fileExists(packagePath) {
		return ""
	}

	data, err := os.ReadFile(packagePath)
	if err != nil {
		return ""
	}

	var pkg struct {
		Version string `json:"version"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}

	return pkg.Version
}

func (um *UpgradeManager) getLatestVersion(packageName string) (string, error) {
	pkgInfo, err := um.pm.getPackageInfo(packageName, "latest")
	if err != nil {
		return "", err
	}
	return pkgInfo.Version, nil
}

func (um *UpgradeManager) needsUpgrade(current, latest string) bool {
	return compareVersions(current, latest) < 0
}

func (um *UpgradeManager) isDevDependency(packageName string) bool {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return false
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}

	if pkg.DevDependencies != nil {
		_, exists := pkg.DevDependencies[packageName]
		return exists
	}

	return false
}

func (um *UpgradeManager) ShowUpgradePreview(upgrades []UpgradeInfo) {
	if len(upgrades) == 0 {
		fmt.Printf(" %s No packages to upgrade\n", color.GreenString("✓"))
		return
	}

	upgradeCount := 0
	for _, upgrade := range upgrades {
		if upgrade.NeedsUpgrade {
			upgradeCount++
		}
	}

	if upgradeCount == 0 {
		fmt.Printf(" %s All packages are up to date\n", color.GreenString("✓"))
		return
	}

	fmt.Printf("\n %s %d package(s) can be upgraded:\n\n", color.YellowString("⬆"), upgradeCount)

	for _, upgrade := range upgrades {
		if upgrade.NeedsUpgrade {
			arrow := color.BlueString("→")
			current := color.RedString(upgrade.CurrentVersion)
			latest := color.GreenString(upgrade.LatestVersion)
			name := color.CyanString(upgrade.Name)

			devTag := ""
			if upgrade.IsDev {
				devTag = color.HiBlackString(" (dev)")
			}

			fmt.Printf("   %s %s %s %s%s\n", name, current, arrow, latest, devTag)
		}
	}
	fmt.Println()
}

func compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int

		if i < len(parts1) {
			p1 = parseVersionPart(parts1[i])
		}
		if i < len(parts2) {
			p2 = parseVersionPart(parts2[i])
		}

		if p1 < p2 {
			return -1
		} else if p1 > p2 {
			return 1
		}
	}

	return 0
}

func parseVersionPart(part string) int {
	cleaned := strings.Split(part, "-")[0]
	cleaned = strings.Split(cleaned, "+")[0]

	if num, err := strconv.Atoi(cleaned); err == nil {
		return num
	}
	return 0
}
