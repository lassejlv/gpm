package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Cache struct {
	cacheDir string
}

func NewCache() *Cache {
	cacheDir := getCacheDir()
	return &Cache{
		cacheDir: cacheDir,
	}
}

func getCacheDir() string {
	var cacheDir string

	switch runtime.GOOS {
	case "windows":
		cacheDir = filepath.Join(os.Getenv("APPDATA"), "gpm", "cache")
	case "darwin":
		homeDir, _ := os.UserHomeDir()
		cacheDir = filepath.Join(homeDir, "Library", "Caches", "gpm")
	default:
		homeDir, _ := os.UserHomeDir()
		cacheDir = filepath.Join(homeDir, ".cache", "gpm")
	}

	os.MkdirAll(cacheDir, 0755)
	return cacheDir
}

func (c *Cache) getPackagePath(name, version string) string {
	hash := sha256.Sum256([]byte(name + "@" + version))
	hashStr := hex.EncodeToString(hash[:])[:12]
	return filepath.Join(c.cacheDir, fmt.Sprintf("%s-%s-%s", name, version, hashStr))
}

func (c *Cache) hasPackage(name, version string) bool {
	packagePath := c.getPackagePath(name, version)
	_, err := os.Stat(packagePath)
	return err == nil
}

func (c *Cache) storePackage(name, version string, tarballReader io.Reader) error {
	packagePath := c.getPackagePath(name, version)

	if err := os.MkdirAll(filepath.Dir(packagePath), 0755); err != nil {
		return err
	}

	file, err := os.Create(packagePath + ".tgz")
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, tarballReader)
	return err
}

func (c *Cache) getPackage(name, version string) (io.ReadCloser, error) {
	packagePath := c.getPackagePath(name, version) + ".tgz"
	return os.Open(packagePath)
}

func (c *Cache) copyToNodeModules(name, version, destPath string) error {
	packagePath := c.getPackagePath(name, version)

	if !c.hasPackage(name, version) {
		return fmt.Errorf("package not in cache")
	}

	return copyDirectory(packagePath, destPath)
}

func copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		return copyFile(path, destPath)
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

func (c *Cache) getCacheSize() (int64, error) {
	var size int64

	err := filepath.Walk(c.cacheDir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

func (c *Cache) clear() error {
	return os.RemoveAll(c.cacheDir)
}

func (c *Cache) getPackageCount() (int, error) {
	count := 0
	err := filepath.Walk(c.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() && path != c.cacheDir {
			relPath, _ := filepath.Rel(c.cacheDir, path)
			if !strings.Contains(relPath, string(os.PathSeparator)) {
				count++
			}
		}
		return nil
	})
	return count, err
}

type CachedPackage struct {
	Name    string
	Version string
	Path    string
}

func (c *Cache) listPackages() ([]CachedPackage, error) {
	var packages []CachedPackage

	err := filepath.Walk(c.cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && path != c.cacheDir {
			relPath, _ := filepath.Rel(c.cacheDir, path)
			if !strings.Contains(relPath, string(os.PathSeparator)) {
				name := filepath.Base(path)
				parts := strings.Split(name, "-")
				if len(parts) >= 3 {
					version := parts[len(parts)-2]
					packageName := strings.Join(parts[:len(parts)-2], "-")

					packages = append(packages, CachedPackage{
						Name:    packageName,
						Version: version,
						Path:    path,
					})
				}
			}
		}
		return nil
	})

	return packages, err
}
