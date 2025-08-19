package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Description     string            `json:"description,omitempty"`
	Main            string            `json:"main,omitempty"`
	Scripts         map[string]string `json:"scripts,omitempty"`
	Keywords        []string          `json:"keywords,omitempty"`
	Author          string            `json:"author,omitempty"`
	License         string            `json:"license,omitempty"`
	Dependencies    map[string]string `json:"dependencies,omitempty"`
	DevDependencies map[string]string `json:"devDependencies,omitempty"`
}

func updatePackageJSON(packageName, version string, isDev bool) error {
	data, err := os.ReadFile("package.json")
	if err != nil {
		return fmt.Errorf("failed to read package.json: %v", err)
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return fmt.Errorf("failed to parse package.json: %v", err)
	}

	if pkg.Dependencies == nil {
		pkg.Dependencies = make(map[string]string)
	}
	if pkg.DevDependencies == nil {
		pkg.DevDependencies = make(map[string]string)
	}

	if isDev {
		pkg.DevDependencies[packageName] = "^" + version
	} else {
		pkg.Dependencies[packageName] = "^" + version
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
