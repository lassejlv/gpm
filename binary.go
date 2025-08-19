package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fatih/color"
)

type BinaryManager struct {
	nodeModulesPath string
	binPath         string
}

func NewBinaryManager() *BinaryManager {
	return &BinaryManager{
		nodeModulesPath: "./node_modules",
		binPath:         "./node_modules/.bin",
	}
}

func (bm *BinaryManager) setupPackageBinaries(packageName string) error {
	packagePath := filepath.Join(bm.nodeModulesPath, packageName)
	packageJSONPath := filepath.Join(packagePath, "package.json")

	if !fileExists(packageJSONPath) {
		return nil
	}

	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Name      string            `json:"name"`
		Bin       map[string]string `json:"bin"`
		BinString string            `json:"bin"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	if err := os.MkdirAll(bm.binPath, 0755); err != nil {
		return fmt.Errorf("failed to create .bin directory: %v", err)
	}

	binaries := make(map[string]string)

	if pkg.Bin != nil {
		binaries = pkg.Bin
	} else if pkg.BinString != "" {
		binaries[packageName] = pkg.BinString
	}

	for binName, binPath := range binaries {
		if err := bm.createBinaryLink(packageName, binName, binPath); err != nil {
			fmt.Printf(" %s Failed to link binary %s: %v\n", color.YellowString("⚠"), binName, err)
		}
	}

	return nil
}

func (bm *BinaryManager) createBinaryLink(packageName, binName, binPath string) error {
	sourcePath := filepath.Join(bm.nodeModulesPath, packageName, binPath)
	targetPath := filepath.Join(bm.binPath, binName)

	if !fileExists(sourcePath) {
		return fmt.Errorf("binary source not found: %s", sourcePath)
	}

	if fileExists(targetPath) {
		os.Remove(targetPath)
	}

	if runtime.GOOS == "windows" {
		return bm.createWindowsBinary(sourcePath, targetPath)
	} else {
		return bm.createUnixBinary(sourcePath, targetPath)
	}
}

func (bm *BinaryManager) createUnixBinary(sourcePath, targetPath string) error {
	relativeSource, err := filepath.Rel(filepath.Dir(targetPath), sourcePath)
	if err != nil {
		return err
	}

	script := fmt.Sprintf(`#!/bin/sh
basedir=$(dirname "$(echo "$0" | sed -e 's,\\,/,g')")

case "$(uname -s)" in
    *CYGWIN*|*MINGW*|*MSYS*) basedir=$(cygpath -w "$basedir");;
esac

if [ -x "$basedir/%s" ]; then
  exec "$basedir/%s" "$@"
else
  exec node "$basedir/%s" "$@"
fi
`, relativeSource, relativeSource, relativeSource)

	if err := os.WriteFile(targetPath, []byte(script), 0755); err != nil {
		return err
	}

	return nil
}

func (bm *BinaryManager) createWindowsBinary(sourcePath, targetPath string) error {
	relativeSource, err := filepath.Rel(filepath.Dir(targetPath), sourcePath)
	if err != nil {
		return err
	}

	relativeSource = strings.ReplaceAll(relativeSource, "/", "\\")

	cmdScript := fmt.Sprintf(`@ECHO off
GOTO start
:find_dp0
SET dp0=%%~dp0
EXIT /b
:start
SETLOCAL
CALL :find_dp0

IF EXIST "%%dp0\%s" (
  SET "_prog=%%dp0\%s"
) ELSE (
  SET "_prog=%%dp0\%s"
  SET PATHEXT=%%PATHEXT:;.JS;=;%%
)

endLocal & goto #_undefined_# 2>NUL || title %%COMSPEC%% & "%%_prog%%" %%*
`, relativeSource, relativeSource, relativeSource)

	cmdPath := targetPath + ".cmd"
	if err := os.WriteFile(cmdPath, []byte(cmdScript), 0755); err != nil {
		return err
	}

	psScript := fmt.Sprintf(`#!/usr/bin/env pwsh
$basedir=Split-Path $MyInvocation.MyCommand.Definition -Parent

$exe=""
if ($PSVersionTable.PSVersion -lt "6.0" -or $IsWindows) {
  $exe=".exe"
}
$ret=0
if (Test-Path "$basedir/%s$exe") {
  & "$basedir/%s$exe" $args
  $ret=$LASTEXITCODE
} else {
  & "node$exe" "$basedir/%s" $args
  $ret=$LASTEXITCODE
}
exit $ret
`, relativeSource, relativeSource, relativeSource)

	ps1Path := targetPath + ".ps1"
	if err := os.WriteFile(ps1Path, []byte(psScript), 0755); err != nil {
		return err
	}

	return nil
}

func (bm *BinaryManager) removePackageBinaries(packageName string) error {
	packagePath := filepath.Join(bm.nodeModulesPath, packageName)
	packageJSONPath := filepath.Join(packagePath, "package.json")

	if !fileExists(packageJSONPath) {
		return nil
	}

	data, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return nil
	}

	var pkg struct {
		Name      string            `json:"name"`
		Bin       map[string]string `json:"bin"`
		BinString string            `json:"bin"`
	}

	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}

	binaries := make(map[string]string)

	if pkg.Bin != nil {
		binaries = pkg.Bin
	} else if pkg.BinString != "" {
		binaries[packageName] = pkg.BinString
	}

	for binName := range binaries {
		targetPath := filepath.Join(bm.binPath, binName)
		os.Remove(targetPath)
		os.Remove(targetPath + ".cmd")
		os.Remove(targetPath + ".ps1")
	}

	return nil
}

func (bm *BinaryManager) listBinaries() ([]string, error) {
	if !fileExists(bm.binPath) {
		return []string{}, nil
	}

	entries, err := os.ReadDir(bm.binPath)
	if err != nil {
		return nil, err
	}

	var binaries []string
	seen := make(map[string]bool)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".cmd") || strings.HasSuffix(name, ".ps1") {
			base := strings.TrimSuffix(strings.TrimSuffix(name, ".cmd"), ".ps1")
			if !seen[base] {
				binaries = append(binaries, base)
				seen[base] = true
			}
		} else {
			if !seen[name] {
				binaries = append(binaries, name)
				seen[name] = true
			}
		}
	}

	return binaries, nil
}

func (bm *BinaryManager) setupAllBinaries() error {
	if !fileExists(bm.nodeModulesPath) {
		return nil
	}

	entries, err := os.ReadDir(bm.nodeModulesPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packageName := entry.Name()
		if packageName == ".bin" {
			continue
		}

		// Handle scoped packages
		if strings.HasPrefix(packageName, "@") {
			scopePath := filepath.Join(bm.nodeModulesPath, packageName)
			scopeEntries, err := os.ReadDir(scopePath)
			if err != nil {
				continue
			}

			for _, scopeEntry := range scopeEntries {
				if scopeEntry.IsDir() {
					fullPackageName := packageName + "/" + scopeEntry.Name()
					if err := bm.setupPackageBinaries(fullPackageName); err != nil {
						// Continue on error
					}
				}
			}
		} else {
			if err := bm.setupPackageBinaries(packageName); err != nil {
				// Continue on error
			}
		}
	}

	return nil
}
