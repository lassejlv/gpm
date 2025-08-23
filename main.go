package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	if !fileExists("package.json") {
		color.Red("Error: package.json not found in current directory")
		color.Yellow("Please run this command in a directory with a package.json file")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "install", "i", "add":
		handleInstall()
	case "uninstall", "remove", "rm":
		handleUninstall()
	case "upgrade", "update":
		handleUpgrade()
	case "cache":
		handleCache()
	case "bin":
		handleBin()
	case "help", "-h", "--help":
		printUsage()
	default:
		color.Red("Unknown command: %s", command)
		printUsage()
		os.Exit(1)
	}
}

func handleInstall() {
	pm := NewPackageManager()

	lockFile, err := loadLockFile()
	if err != nil {
		color.Red("Failed to load lockfile: %v", err)
		os.Exit(1)
	}

	if len(os.Args) < 3 {
		if err := installFromPackageJSON(pm, lockFile); err != nil {
			color.Red("Failed to install packages: %v", err)
			os.Exit(1)
		}
		return
	}

	timer := NewTimer()
	timer.Start()

	packages := []string{}
	isDev := false

	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--save-dev" || arg == "-D" {
			isDev = true
		} else if !strings.HasPrefix(arg, "--") {
			packages = append(packages, arg)
		}
	}

	if len(packages) == 0 {
		color.Red("Error: Please specify a package to install")
		os.Exit(1)
	}


	parallelInstaller := NewParallelInstaller(pm, lockFile, timer)
	if err := parallelInstaller.InstallFromSpecs(packages, isDev, true); err != nil {
		color.Red("Failed to install packages: %v", err)
		os.Exit(1)
	}

	elapsed := timer.Stop()

	if err := lockFile.saveLockFile(); err != nil {
		fmt.Printf(" %s Failed to save lockfile: %v\n", color.YellowString("⚠"), err)
	}

	fmt.Printf(" %s Done in %s\n", color.HiGreenString("✓"), color.HiBlackString(formatDuration(elapsed)))
}

func handleUninstall() {
	if len(os.Args) < 3 {
		color.Red("Error: Please specify a package to uninstall")
		os.Exit(1)
	}

	lockFile, err := loadLockFile()
	if err != nil {
		color.Red("Failed to load lockfile: %v", err)
		os.Exit(1)
	}

	packages := os.Args[2:]
	for _, packageName := range packages {
		if err := uninstallPackage(packageName, lockFile); err != nil {
			color.Red("Failed to uninstall %s: %v", packageName, err)
			os.Exit(1)
		}
	}

	if err := lockFile.saveLockFile(); err != nil {
		fmt.Printf(" %s Failed to save lockfile: %v\n", color.YellowString("⚠"), err)
	}

	fmt.Printf(" %s Uninstalled %d package(s)\n", color.HiGreenString("✓"), len(packages))
}

func handleUpgrade() {
	if !fileExists("package.json") {
		color.Red("Error: package.json not found in current directory")
		os.Exit(1)
	}

	lockFile, err := loadLockFile()
	if err != nil {
		color.Red("Failed to load lockfile: %v", err)
		os.Exit(1)
	}

	pm := NewPackageManager()
	upgradeManager := NewUpgradeManager(pm, lockFile)


	skipTUI := false
	var packagesToUpgrade []string

	if len(os.Args) > 2 {
		for _, arg := range os.Args[2:] {
			if arg == "--all" || arg == "-a" {
				skipTUI = true
			} else {
				packagesToUpgrade = append(packagesToUpgrade, arg)
			}
		}
	}

	if len(packagesToUpgrade) == 0 && !skipTUI {

		data, err := os.ReadFile("package.json")
		if err != nil {
			color.Red("Failed to read package.json: %v", err)
			os.Exit(1)
		}

		var pkg PackageJSON
		if err := json.Unmarshal(data, &pkg); err != nil {
			color.Red("Failed to parse package.json: %v", err)
			os.Exit(1)
		}

		for name := range pkg.Dependencies {
			packagesToUpgrade = append(packagesToUpgrade, name)
		}
		for name := range pkg.DevDependencies {
			packagesToUpgrade = append(packagesToUpgrade, name)
		}
	}

	if len(packagesToUpgrade) == 0 {
		fmt.Printf(" %s No packages to upgrade\n", color.YellowString("⚠"))
		return
	}


	upgrades, err := upgradeManager.CheckUpgrades(packagesToUpgrade)
	if err != nil {
		color.Red("Failed to check for upgrades: %v", err)
		os.Exit(1)
	}

	var packagesNeedingUpgrade []string

	if skipTUI {

		for _, upgrade := range upgrades {
			if upgrade.NeedsUpgrade {
				packagesNeedingUpgrade = append(packagesNeedingUpgrade, upgrade.Name)
			}
		}

		if len(packagesNeedingUpgrade) == 0 {
			fmt.Printf(" %s All packages are up to date\n", color.GreenString("✓"))
			return
		}

		fmt.Printf(" %s Upgrading %d package(s)...\n", color.YellowString("⬆"), len(packagesNeedingUpgrade))
	} else {

		tui := NewTUI()
		selectedUpgrades, err := tui.SelectPackagesToUpgrade(upgrades)
		if err != nil {
			color.Red("Failed to select packages: %v", err)
			os.Exit(1)
		}

		if len(selectedUpgrades) == 0 {
			return
		}


		for _, upgrade := range selectedUpgrades {
			packagesNeedingUpgrade = append(packagesNeedingUpgrade, upgrade.Name)
		}
	}

	timer := NewTimer()
	timer.Start()


	parallelInstaller := NewParallelInstaller(pm, lockFile, timer)
	if err := parallelInstaller.InstallFromSpecs(packagesNeedingUpgrade, false, true); err != nil {
		color.Red("Failed to upgrade packages: %v", err)
		os.Exit(1)
	}

	elapsed := timer.Stop()

	if err := lockFile.saveLockFile(); err != nil {
		fmt.Printf(" %s Failed to save lockfile: %v\n", color.YellowString("⚠"), err)
	}

	fmt.Printf(" %s Upgraded %d package(s) in %s\n", color.HiGreenString("✓"), len(packagesNeedingUpgrade), color.HiBlackString(formatDuration(elapsed)))
}

func handleBin() {
	bm := NewBinaryManager()
	binaries, err := bm.listBinaries()
	if err != nil {
		color.Red("Failed to list binaries: %v", err)
		os.Exit(1)
	}

	if len(binaries) == 0 {
		fmt.Printf("\n %s No binaries found\n", color.HiBlackString("ℹ"))
		return
	}

	fmt.Printf("\n %s Available binaries (%d)\n", color.CyanString("🔧"), len(binaries))
	for _, binary := range binaries {
		fmt.Printf("   %s\n", color.CyanString(binary))
	}
	fmt.Println()
}

func handleCache() {
	if len(os.Args) < 3 {
		printCacheUsage()
		os.Exit(1)
	}

	cache := NewCache()
	subcommand := os.Args[2]

	switch subcommand {
	case "info":
		showCacheInfo(cache)
	case "clear":
		clearCache(cache)
	case "ls", "list":
		listCache(cache)
	default:
		color.Red("Unknown cache command: %s", subcommand)
		printCacheUsage()
		os.Exit(1)
	}
}

func showCacheInfo(cache *Cache) {
	size, err := cache.getCacheSize()
	if err != nil {
		color.Red("Failed to get cache info: %v", err)
		os.Exit(1)
	}

	packageCount, err := cache.getPackageCount()
	if err != nil {
		color.Red("Failed to get package count: %v", err)
		os.Exit(1)
	}

	fmt.Printf("\n %s Cache Information\n", color.CyanString("ℹ"))
	fmt.Printf(" Location: %s\n", color.HiBlackString(cache.cacheDir))
	fmt.Printf(" Size: %s\n", color.WhiteString(formatBytes(size)))
	fmt.Printf(" Packages: %s\n", color.WhiteString(fmt.Sprintf("%d", packageCount)))
}

func clearCache(cache *Cache) {
	fmt.Printf(" %s Clearing cache...", color.YellowString("⚡"))
	if err := cache.clear(); err != nil {
		fmt.Print("\r                                        \r")
		color.Red("Failed to clear cache: %v", err)
		os.Exit(1)
	}
	fmt.Print("\r                                        \r")
	fmt.Printf(" %s Cache cleared successfully!\n", color.HiGreenString("✓"))
}

func listCache(cache *Cache) {
	packages, err := cache.listPackages()
	if err != nil {
		color.Red("Failed to list cache: %v", err)
		os.Exit(1)
	}

	if len(packages) == 0 {
		fmt.Printf("\n %s Cache is empty\n", color.HiBlackString("ℹ"))
		return
	}

	fmt.Printf("\n %s Cached Packages (%d)\n", color.CyanString("📦"), len(packages))
	for _, pkg := range packages {
		fmt.Printf("   %s@%s\n", color.CyanString(pkg.Name), color.HiBlackString(pkg.Version))
	}
}

func printCacheUsage() {
	fmt.Printf("\n%s GPM Cache Commands\n\n", color.CyanString("⚡"))
	fmt.Println("Usage:")
	fmt.Println("  gpm cache info               Show cache information")
	fmt.Println("  gpm cache clear              Clear the cache")
	fmt.Println("  gpm cache ls                 List cached packages")
	fmt.Println("  gpm cache list               List cached packages")
	fmt.Println()
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func printUsage() {
	fmt.Printf("\n%s GPM - Go Package Manager for Node.js\n\n", color.CyanString("⚡"))
	fmt.Println("Usage:")
	fmt.Println("  gpm install                 Install all packages from package.json")
	fmt.Println("  gpm install <package>        Install a package")
	fmt.Println("  gpm i <package>              Install a package (short)")
	fmt.Println("  gpm install <pkg> --save-dev Install as dev dependency")
	fmt.Println("  gpm uninstall <package>      Uninstall a package")
	fmt.Println("  gpm upgrade [package]        Upgrade packages to latest")
	fmt.Println("  gpm upgrade --all            Upgrade all packages without prompt")
	fmt.Println("  gpm bin                      List available binaries")
	fmt.Println("  gpm cache <command>          Cache management")
	fmt.Println("  gpm help                     Show this help message")
	fmt.Println("\nExamples:")
	fmt.Printf("  gpm install                  %s Install from package.json\n", color.GreenString("✓"))
	fmt.Printf("  gpm install lodash           %s Install lodash\n", color.CyanString("↓"))
	fmt.Printf("  gpm i express react          %s Install multiple packages\n", color.CyanString("↓"))
	fmt.Printf("  gpm install typescript --save-dev  %s Install as dev dependency\n", color.CyanString("↓"))
	fmt.Printf("  gpm uninstall lodash         %s Remove lodash\n", color.RedString("✗"))
	fmt.Printf("  gpm upgrade                  %s Upgrade packages (interactive)\n", color.BlueString("⬆"))
	fmt.Printf("  gpm upgrade --all            %s Upgrade all packages\n", color.BlueString("⬆"))
	fmt.Printf("  gpm bin                      %s List available binaries\n", color.CyanString("🔧"))
	fmt.Printf("  gpm cache info               %s Show cache info\n", color.CyanString("ℹ"))
	fmt.Println("\nNote: Requires package.json in current directory")
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return !os.IsNotExist(err)
}
