package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
)

type TUI struct {
	reader *bufio.Reader
}

func NewTUI() *TUI {
	return &TUI{
		reader: bufio.NewReader(os.Stdin),
	}
}

func (t *TUI) SelectPackagesToUpgrade(upgrades []UpgradeInfo) ([]UpgradeInfo, error) {
	if len(upgrades) == 0 {
		return upgrades, nil
	}

	upgradeCount := 0
	for _, upgrade := range upgrades {
		if upgrade.NeedsUpgrade {
			upgradeCount++
		}
	}

	if upgradeCount == 0 {
		fmt.Printf(" %s All packages are up to date\n", color.GreenString("✓"))
		return []UpgradeInfo{}, nil
	}

	fmt.Printf("\n %s %d package(s) can be upgraded:\n\n", color.YellowString("⬆"), upgradeCount)

	var upgradeablePackages []UpgradeInfo
	index := 1

	for _, upgrade := range upgrades {
		if upgrade.NeedsUpgrade {
			arrow := color.BlueString("→")
			current := color.RedString(upgrade.CurrentVersion)
			latest := color.GreenString(upgrade.LatestVersion)
			name := color.CyanString(upgrade.Name)
			indexStr := color.HiBlackString(fmt.Sprintf("[%d]", index))

			devTag := ""
			if upgrade.IsDev {
				devTag = color.HiBlackString(" (dev)")
			}

			fmt.Printf("   %s %s %s %s %s%s\n", indexStr, name, current, arrow, latest, devTag)
			upgradeablePackages = append(upgradeablePackages, upgrade)
			index++
		}
	}

	fmt.Println()
	fmt.Printf(" %s Select packages to upgrade:\n", color.CyanString("?"))
	fmt.Printf("   %s\n", color.HiBlackString("Enter numbers (e.g., 1,3,5) or 'a' for all, 'n' for none:"))
	fmt.Print(" > ")

	input, err := t.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}

	input = strings.TrimSpace(input)

	if input == "" || strings.ToLower(input) == "n" || strings.ToLower(input) == "none" {
		fmt.Printf(" %s No packages selected for upgrade\n", color.YellowString("ℹ"))
		return []UpgradeInfo{}, nil
	}

	if strings.ToLower(input) == "a" || strings.ToLower(input) == "all" {
		fmt.Printf(" %s Selected all %d packages for upgrade\n", color.GreenString("✓"), len(upgradeablePackages))
		return upgradeablePackages, nil
	}

	selected, err := t.parseSelection(input, len(upgradeablePackages))
	if err != nil {
		return nil, fmt.Errorf("invalid selection: %v", err)
	}

	var selectedPackages []UpgradeInfo
	for _, i := range selected {
		selectedPackages = append(selectedPackages, upgradeablePackages[i-1])
	}

	if len(selectedPackages) > 0 {
		fmt.Printf(" %s Selected %d package(s) for upgrade:", color.GreenString("✓"), len(selectedPackages))
		for _, pkg := range selectedPackages {
			fmt.Printf(" %s", color.CyanString(pkg.Name))
		}
		fmt.Println()
	} else {
		fmt.Printf(" %s No packages selected for upgrade\n", color.YellowString("ℹ"))
	}

	return selectedPackages, nil
}

func (t *TUI) parseSelection(input string, maxIndex int) ([]int, error) {
	var selected []int
	seen := make(map[int]bool)

	parts := strings.Split(input, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range format: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start number: %s", rangeParts[0])
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end number: %s", rangeParts[1])
			}

			if start > end {
				start, end = end, start
			}

			for i := start; i <= end; i++ {
				if i < 1 || i > maxIndex {
					return nil, fmt.Errorf("number %d is out of range (1-%d)", i, maxIndex)
				}
				if !seen[i] {
					selected = append(selected, i)
					seen[i] = true
				}
			}
		} else {
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", part)
			}

			if num < 1 || num > maxIndex {
				return nil, fmt.Errorf("number %d is out of range (1-%d)", num, maxIndex)
			}

			if !seen[num] {
				selected = append(selected, num)
				seen[num] = true
			}
		}
	}

	return selected, nil
}

func (t *TUI) ConfirmAction(message string) bool {
	fmt.Printf(" %s %s (y/N): ", color.YellowString("?"), message)

	input, err := t.reader.ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}
