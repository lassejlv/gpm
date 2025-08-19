package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
)

type PackageManager struct {
	nodeModulesPath string
	registryURL     string
	cache           *Cache
}

type PackageInfo struct {
	Name    string   `json:"name"`
	Version string   `json:"version"`
	Dist    DistInfo `json:"dist"`
}

type DistInfo struct {
	Tarball string `json:"tarball"`
	Shasum  string `json:"shasum"`
}

type RegistryResponse struct {
	Versions map[string]PackageInfo `json:"versions"`
	DistTags map[string]string      `json:"dist-tags"`
}

func NewPackageManager() *PackageManager {
	return &PackageManager{
		nodeModulesPath: "./node_modules",
		registryURL:     "https://registry.npmjs.org",
		cache:           NewCache(),
	}
}

func (pm *PackageManager) Install(packageName, version string) (string, bool, error) {
	// Ensure node_modules directory exists
	if err := pm.ensureNodeModulesDir(); err != nil {
		return "", false, fmt.Errorf("failed to create node_modules directory: %v", err)
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = fmt.Sprintf(" %s Resolving %s@%s", color.CyanString("→"), color.CyanString(packageName), color.HiBlackString(version))
	s.Color("cyan")
	s.Start()

	pkgInfo, err := pm.getPackageInfo(packageName, version)
	s.Stop()
	fmt.Print("\r                                                                \r")

	if err != nil {
		return "", false, fmt.Errorf("failed to get package info: %v", err)
	}

	packagePath := filepath.Join(pm.nodeModulesPath, packageName)
	if pm.isPackageInstalled(packagePath, pkgInfo.Version) {
		fmt.Printf(" %s %s@%s %s\n", color.HiGreenString("✓"), color.CyanString(packageName), color.HiBlackString(pkgInfo.Version), color.HiBlackString("(cached)"))
		return pkgInfo.Version, true, nil
	}

	if pm.cache.hasPackage(packageName, pkgInfo.Version) {
		if err := pm.installFromCache(packageName, pkgInfo.Version, packagePath); err == nil {
			return pkgInfo.Version, true, nil
		}
	}

	if err := pm.downloadAndExtract(pkgInfo, packagePath); err != nil {
		return "", false, fmt.Errorf("failed to download and extract package: %v", err)
	}

	return pkgInfo.Version, false, nil
}

func (pm *PackageManager) ensureNodeModulesDir() error {
	return os.MkdirAll(pm.nodeModulesPath, 0755)
}

func (pm *PackageManager) getPackageInfo(packageName, version string) (*PackageInfo, error) {
	url := fmt.Sprintf("%s/%s", pm.registryURL, packageName)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("package '%s' not found in npm registry", packageName)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm registry error: status %d", resp.StatusCode)
	}

	var registryResp RegistryResponse
	if err := json.NewDecoder(resp.Body).Decode(&registryResp); err != nil {
		return nil, fmt.Errorf("failed to parse registry response: %v", err)
	}

	// Resolve version
	if version == "latest" {
		if latestVersion, ok := registryResp.DistTags["latest"]; ok {
			version = latestVersion
		} else {
			return nil, fmt.Errorf("no latest version found for %s", packageName)
		}
	} else if strings.Contains(version, "x") || strings.Contains(version, "||") || strings.Contains(version, "^") || strings.Contains(version, "~") {
		resolvedVersion := pm.resolveVersionRange(version, registryResp.Versions)
		if resolvedVersion == "" {
			if latestVersion, ok := registryResp.DistTags["latest"]; ok {
				version = latestVersion
			} else {
				return nil, fmt.Errorf("could not resolve version range %s for package %s", version, packageName)
			}
		} else {
			version = resolvedVersion
		}
	}

	pkgInfo, ok := registryResp.Versions[version]
	if !ok {
		return nil, fmt.Errorf("version %s not found for package %s", version, packageName)
	}

	return &pkgInfo, nil
}

func (pm *PackageManager) isPackageInstalled(packagePath, version string) bool {
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

func (pm *PackageManager) downloadAndExtract(pkgInfo *PackageInfo, destPath string) error {
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Get(pkgInfo.Dist.Tarball)
	if err != nil {
		return fmt.Errorf("failed to download package: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download package: status %d", resp.StatusCode)
	}

	bar := progressbar.NewOptions64(
		resp.ContentLength,
		progressbar.OptionSetDescription(fmt.Sprintf(" %s %s", color.CyanString("↓"), pkgInfo.Name)),
		progressbar.OptionSetWidth(20),
		progressbar.OptionShowBytes(true),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetRenderBlankState(false),
		progressbar.OptionThrottle(50*time.Millisecond),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerHead:    "█",
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
	)

	reader := progressbar.NewReader(resp.Body, bar)

	gzipReader, err := gzip.NewReader(&reader)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	if err := pm.extractAndCache(tarReader, destPath, pkgInfo.Name, pkgInfo.Version); err != nil {
		return fmt.Errorf("failed to extract package: %v", err)
	}

	return nil
}

func (pm *PackageManager) extractAndCache(tarReader *tar.Reader, destPath, packageName, version string) error {
	cachePath := pm.cache.getPackagePath(packageName, version)

	if err := os.RemoveAll(destPath); err != nil {
		return err
	}
	if err := os.RemoveAll(cachePath); err != nil {
		return err
	}

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := strings.TrimPrefix(header.Name, "package/")
		if path == "" || path == header.Name {
			continue
		}

		target := filepath.Join(destPath, path)
		cacheTarget := filepath.Join(cachePath, path)

		cleanDest := filepath.Clean(destPath)
		cleanTarget := filepath.Clean(target)
		if !strings.HasPrefix(cleanTarget, cleanDest+string(os.PathSeparator)) && cleanTarget != cleanDest {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
			if err := os.MkdirAll(cacheTarget, os.FileMode(header.Mode)); err != nil {
				return err
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(cacheTarget), 0755); err != nil {
				return err
			}

			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			cacheFile, err := os.OpenFile(cacheTarget, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				file.Close()
				return err
			}

			writer := io.MultiWriter(file, cacheFile)
			if _, err := io.Copy(writer, tarReader); err != nil {
				file.Close()
				cacheFile.Close()
				return err
			}
			file.Close()
			cacheFile.Close()
		}
	}

	return nil
}

func (pm *PackageManager) InstallDependencies(packageName string, lockFile *LockFile) error {
	packagePath := filepath.Join(pm.nodeModulesPath, packageName)
	packageJSONPath := filepath.Join(packagePath, "package.json")

	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	for depName := range pkg.Dependencies {
		depPath := filepath.Join(pm.nodeModulesPath, depName)
		if _, err := os.Stat(depPath); err == nil {
			continue
		}

		installedVersion, err := pm.installSimple(depName, "latest")
		if err != nil {
			continue
		}

		if err := lockFile.addPackage(depName, installedVersion, depName, false); err != nil {
			continue
		}
	}

	return nil
}

func (pm *PackageManager) installSimple(packageName, version string) (string, error) {
	pkgInfo, err := pm.getPackageInfo(packageName, version)
	if err != nil {
		return "", err
	}

	packagePath := filepath.Join(pm.nodeModulesPath, packageName)
	if pm.isPackageInstalled(packagePath, pkgInfo.Version) {
		return pkgInfo.Version, nil
	}

	if pm.cache.hasPackage(packageName, pkgInfo.Version) {
		if err := pm.installFromCache(packageName, pkgInfo.Version, packagePath); err == nil {
			return pkgInfo.Version, nil
		}
	}

	if err := pm.downloadAndExtract(pkgInfo, packagePath); err != nil {
		return "", err
	}

	return pkgInfo.Version, nil
}

func (pm *PackageManager) installFromCache(packageName, version, destPath string) error {
	cachePath := pm.cache.getPackagePath(packageName, version)
	return copyDirectory(cachePath, destPath)
}

func (pm *PackageManager) resolveVersionRange(versionRange string, availableVersions map[string]PackageInfo) string {
	if strings.Contains(versionRange, "||") {
		parts := strings.Split(versionRange, "||")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			resolved := pm.resolveSingleVersion(part, availableVersions)
			if resolved != "" {
				return resolved
			}
		}
	} else {
		return pm.resolveSingleVersion(versionRange, availableVersions)
	}
	return ""
}

func (pm *PackageManager) resolveSingleVersion(version string, availableVersions map[string]PackageInfo) string {
	version = strings.TrimSpace(version)

	if strings.Contains(version, "x") {
		pattern := strings.ReplaceAll(version, "x", "")
		pattern = strings.TrimSuffix(pattern, ".")

		var bestVersion string
		for v := range availableVersions {
			if strings.HasPrefix(v, pattern) {
				if bestVersion == "" || pm.compareVersions(v, bestVersion) > 0 {
					bestVersion = v
				}
			}
		}
		return bestVersion
	}

	if strings.HasPrefix(version, "^") {
		baseVersion := strings.TrimPrefix(version, "^")
		parts := strings.Split(baseVersion, ".")
		if len(parts) >= 1 {
			majorVersion := parts[0]
			var bestVersion string
			for v := range availableVersions {
				vParts := strings.Split(v, ".")
				if len(vParts) >= 1 && vParts[0] == majorVersion {
					if bestVersion == "" || pm.compareVersions(v, bestVersion) > 0 {
						bestVersion = v
					}
				}
			}
			return bestVersion
		}
	}

	if strings.HasPrefix(version, "~") {
		baseVersion := strings.TrimPrefix(version, "~")
		parts := strings.Split(baseVersion, ".")
		if len(parts) >= 2 {
			majorMinor := parts[0] + "." + parts[1]
			var bestVersion string
			for v := range availableVersions {
				if strings.HasPrefix(v, majorMinor+".") {
					if bestVersion == "" || pm.compareVersions(v, bestVersion) > 0 {
						bestVersion = v
					}
				}
			}
			return bestVersion
		}
	}

	if _, exists := availableVersions[version]; exists {
		return version
	}

	return ""
}

func (pm *PackageManager) compareVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &p1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &p2)
		}

		if p1 > p2 {
			return 1
		} else if p1 < p2 {
			return -1
		}
	}

	return 0
}
