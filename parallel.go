package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
)

type PackageJob struct {
	Name         string
	Version      string
	IsDev        bool
	OriginalSpec string
}

type PackageResult struct {
	Job              PackageJob
	InstalledVersion string
	Error            error
	FromCache        bool
}

type ParallelInstaller struct {
	pm         *PackageManager
	lockFile   *LockFile
	timer      *Timer
	maxWorkers int
}

func NewParallelInstaller(pm *PackageManager, lockFile *LockFile, timer *Timer) *ParallelInstaller {
	return &ParallelInstaller{
		pm:         pm,
		lockFile:   lockFile,
		timer:      timer,
		maxWorkers: 4,
	}
}

func (pi *ParallelInstaller) InstallPackages(jobs []PackageJob, writeToPackageJSON bool) error {
	if len(jobs) == 0 {
		return nil
	}

	totalJobs := len(jobs)
	jobChan := make(chan PackageJob, totalJobs)
	resultChan := make(chan PackageResult, totalJobs)


	progressDone := make(chan bool)
	go pi.showProgress(totalJobs, resultChan, progressDone)


	var wg sync.WaitGroup
	for i := 0; i < pi.maxWorkers; i++ {
		wg.Add(1)
		go pi.worker(jobChan, resultChan, &wg)
	}


	for _, job := range jobs {
		jobChan <- job
	}
	close(jobChan)


	go func() {
		wg.Wait()
		close(resultChan)
	}()


	<-progressDone

	return nil
}

func (pi *ParallelInstaller) showProgress(total int, results <-chan PackageResult, done chan<- bool) {
	defer close(done)

	completed := 0
	failed := 0
	cached := 0
	downloaded := 0
	var errors []error

	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frameIndex := 0

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case result, ok := <-results:
			if !ok {

				fmt.Print("\r                                                                \r")

				if failed > 0 {
					fmt.Printf(" %s %d/%d packages installed, %d failed\n",
						color.YellowString("⚠"), completed, total, failed)
					for _, err := range errors {
						fmt.Printf("   %s\n", err)
					}
				} else {
					fmt.Printf(" %s All %d packages installed successfully!\n",
						color.HiGreenString("✓"), completed)
				}


				bm := NewBinaryManager()
				if err := bm.setupAllBinaries(); err != nil {
					fmt.Printf(" %s Failed to setup some binaries: %v\n", color.YellowString("⚠"), err)
				}


				if completed > 0 {
					fmt.Printf(" %s %d cached, %d downloaded\n",
						color.MagentaString("→"),
						cached,
						downloaded)
				}
				return
			}

			if result.Error != nil {
				failed++
				errors = append(errors, fmt.Errorf("%s: %v", result.Job.Name, result.Error))
			} else {
				completed++
				if result.FromCache {
					cached++
				} else {
					downloaded++
				}


				if err := pi.lockFile.addPackage(result.Job.Name, result.InstalledVersion, result.Job.OriginalSpec, result.Job.IsDev); err != nil {

				}


				if result.Job.Name != "" {
					updatePackageJSON(result.Job.Name, result.InstalledVersion, result.Job.IsDev)
				}
			}

		case <-ticker.C:
			frame := frames[frameIndex%len(frames)]
			fmt.Printf("\r %s Installing packages...  %d / %d  completed",
				color.CyanString(frame), completed, total)
			frameIndex++
		}
	}
}

func (pi *ParallelInstaller) worker(jobs <-chan PackageJob, results chan<- PackageResult, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		result := PackageResult{Job: job}


		version := "latest"
		if job.Version != "" {
			version = job.Version
		}


		existingVersion := pi.lockFile.getPackageVersion(job.Name)
		if existingVersion != "" && isPackageInstalled(fmt.Sprintf("node_modules/%s", job.Name), existingVersion) {
			result.InstalledVersion = existingVersion
			result.FromCache = true
			results <- result
			continue
		}


		if pi.timer != nil {
			pi.timer.Pause()
		}


		installedVersion, wasCached, err := pi.pm.Install(job.Name, version)

		if pi.timer != nil {
			pi.timer.Resume()
		}

		if err != nil {
			result.Error = err
			results <- result
			continue
		}

		result.InstalledVersion = installedVersion
		result.FromCache = wasCached


		if !wasCached {
			if err := pi.pm.InstallDependencies(job.Name, pi.lockFile); err != nil {

				fmt.Printf(" %s Warning: Failed to install dependencies for %s: %v\n", color.YellowString("⚠"), job.Name, err)
			}
		}

		results <- result
	}
}

func (pi *ParallelInstaller) InstallFromSpecs(packageSpecs []string, isDev bool, writeToPackageJSON bool) error {
	var jobs []PackageJob

	for _, spec := range packageSpecs {
		name, version := parsePackageSpec(spec)
		originalSpec := spec
		if version == "latest" {
			originalSpec = name
		}

		jobs = append(jobs, PackageJob{
			Name:         name,
			Version:      version,
			IsDev:        isDev,
			OriginalSpec: originalSpec,
		})
	}

	return pi.InstallPackages(jobs, writeToPackageJSON)
}

func parsePackageSpec(packageSpec string) (string, string) {
	if strings.HasPrefix(packageSpec, "@") {
		parts := strings.SplitN(packageSpec, "@", 3)
		if len(parts) == 2 {
			return "@" + parts[1], "latest"
		} else if len(parts) == 3 {
			return "@" + parts[1], parts[2]
		}
		return packageSpec, "latest"
	} else {
		parts := strings.Split(packageSpec, "@")
		if len(parts) > 1 {
			return parts[0], parts[1]
		}
		return parts[0], "latest"
	}
}
