package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type LockFile struct {
	Version     string                 `yaml:"lockfileVersion"`
	CreatedAt   time.Time              `yaml:"createdAt"`
	Packages    map[string]LockPackage `yaml:"packages"`
	Specifiers  map[string]string      `yaml:"specifiers"`
	DevPackages map[string]string      `yaml:"devPackages,omitempty"`
	mu          sync.RWMutex           `yaml:"-"`
}

type LockPackage struct {
	Name         string            `yaml:"name"`
	Version      string            `yaml:"version"`
	Resolved     string            `yaml:"resolved"`
	Integrity    string            `yaml:"integrity,omitempty"`
	Dependencies map[string]string `yaml:"dependencies,omitempty"`
	DevDep       bool              `yaml:"dev,omitempty"`
}

const lockFileName = "gpm-lock.yaml"

func loadLockFile() (*LockFile, error) {
	if !fileExists(lockFileName) {
		return &LockFile{
			Version:     "1.0",
			CreatedAt:   time.Now(),
			Packages:    make(map[string]LockPackage),
			Specifiers:  make(map[string]string),
			DevPackages: make(map[string]string),
		}, nil
	}

	data, err := os.ReadFile(lockFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read lockfile: %v", err)
	}

	var lockFile LockFile
	if err := yaml.Unmarshal(data, &lockFile); err != nil {
		return nil, fmt.Errorf("failed to parse lockfile: %v", err)
	}

	if lockFile.Packages == nil {
		lockFile.Packages = make(map[string]LockPackage)
	}
	if lockFile.Specifiers == nil {
		lockFile.Specifiers = make(map[string]string)
	}
	if lockFile.DevPackages == nil {
		lockFile.DevPackages = make(map[string]string)
	}

	return &lockFile, nil
}

func (lf *LockFile) saveLockFile() error {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	lf.CreatedAt = time.Now()

	data, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("failed to marshal lockfile: %v", err)
	}

	if err := os.WriteFile(lockFileName, data, 0644); err != nil {
		return fmt.Errorf("failed to write lockfile: %v", err)
	}

	return nil
}

func (lf *LockFile) addPackage(name, version, specifier string, isDev bool) error {
	packageKey := fmt.Sprintf("%s@%s", name, version)

	deps, err := getPackageDependencies(name)
	if err != nil {
		deps = make(map[string]string)
	}

	lockPkg := LockPackage{
		Name:         name,
		Version:      version,
		Resolved:     fmt.Sprintf("https://registry.npmjs.org/%s/-/%s-%s.tgz", name, name, version),
		Dependencies: deps,
		DevDep:       isDev,
	}

	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	lf.Packages[packageKey] = lockPkg
	lf.Specifiers[name] = specifier

	if isDev {
		lf.DevPackages[name] = specifier
	}

	return nil
}

func (lf *LockFile) hasPackage(name, version string) bool {
	packageKey := fmt.Sprintf("%s@%s", name, version)
	
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	_, exists := lf.Packages[packageKey]
	return exists
}

func (lf *LockFile) getPackageVersion(name string) string {
	lf.mu.RLock()
	defer lf.mu.RUnlock()
	
	for _, pkg := range lf.Packages {
		if pkg.Name == name {
			return pkg.Version
		}
	}
	return ""
}

func getPackageDependencies(packageName string) (map[string]string, error) {
	packagePath := filepath.Join("node_modules", packageName, "package.json")

	if !fileExists(packagePath) {
		return make(map[string]string), nil
	}

	data, err := os.ReadFile(packagePath)
	if err != nil {
		return make(map[string]string), nil
	}

	var pkg struct {
		Dependencies map[string]string `json:"dependencies"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return make(map[string]string), nil
	}

	if pkg.Dependencies == nil {
		return make(map[string]string), nil
	}

	return pkg.Dependencies, nil
}

func (lf *LockFile) removePackage(name string) {
	lf.mu.Lock()
	defer lf.mu.Unlock()
	
	var keysToRemove []string

	for key, pkg := range lf.Packages {
		if pkg.Name == name {
			keysToRemove = append(keysToRemove, key)
		}
	}

	for _, keyToRemove := range keysToRemove {
		delete(lf.Packages, keyToRemove)
	}

	delete(lf.Specifiers, name)
	delete(lf.DevPackages, name)
}
