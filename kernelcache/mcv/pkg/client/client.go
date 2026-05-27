/*
Copyright 2026 The KServe Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package client provides high-level APIs for extracting Kernel caches
// from OCI images, detecting system GPU and accelerator hardware, and
// running compatibility checks between system GPUs and image metadata.
package client

import (
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/kserve/kserve/mcv/pkg/accelerator"
	"github.com/kserve/kserve/mcv/pkg/accelerator/devices"
	"github.com/kserve/kserve/mcv/pkg/config"
	"github.com/kserve/kserve/mcv/pkg/constants"
	"github.com/kserve/kserve/mcv/pkg/fetcher"
	"github.com/kserve/kserve/mcv/pkg/logformat"
	"github.com/kserve/kserve/mcv/pkg/preflightcheck"
	logging "github.com/sirupsen/logrus"
)

// Options encapsulates configurable settings for cache extraction operations.
type Options struct {
	// The name of the OCI image (e.g., quay.io/user/image:tag)
	ImageName string
	// Path to store the extracted cache; if not specified, defaults are: ~/.triton/cache (vanilla Triton), ~/.cache/vllm (vLLM)
	CacheDir string
	// Whether to enable GPU logic for preflight checks (nil = auto-detect, false = disable, true = force)
	EnableGPU *bool
	// Logging level: debug, info, warning, error
	LogLevel string
	// If true, enables full hardware checks including kernel dummy key validation (for baremetal envs only)
	EnableBaremetal *bool
	// If true, skips summary-level preflight GPU compatibility checks
	SkipPrecheck *bool
}

type HwOptions struct {
	EnableStub *bool // If true, enables stub mode (Dummy devices); for testing/dev only (false = disable, true = force))
	Timeout    int   // Timeout in seconds for hardware detection operations (0 = disable timeout)
}

func InspectCacheImage(img string) (labels map[string]string, err error) {
	if img == "" {
		return nil, fmt.Errorf("image name must be specified")
	}

	_, err = name.ParseReference(img, name.StrictValidation)
	if err != nil {
		return nil, fmt.Errorf("error validating image name: %v", err)
	}

	return fetcher.NewImgFetcher().InspectImg(img)
}

// ExtractCache pulls and extracts a kernel cache from the specified OCI image.
// It uses the provided options to configure behavior such as GPU checks, logging, and
// output directory. If GPU checks are enabled, it also verifies hardware compatibility.
func ExtractCache(opts Options) (matchedIDs, unmatchedIDs []int, err error) {
	if opts.ImageName == "" {
		return nil, nil, fmt.Errorf("image name must be specified")
	}

	if !config.IsInitialized() {
		if _, err = config.Initialize(config.ConfDir); err != nil {
			return nil, nil, fmt.Errorf("failed to initialize config: %w", err)
		}
	}

	if err = logformat.ConfigureLogging(opts.LogLevel); err != nil {
		return nil, nil, fmt.Errorf("error configuring logging: %v", err)
	}

	if opts.SkipPrecheck != nil {
		config.SetSkipPrecheck(*opts.SkipPrecheck)
		if *opts.SkipPrecheck {
			logging.Debug("preflight checks disabled via client options")
		}
	}

	if opts.EnableBaremetal != nil {
		config.SetEnabledBaremetal(*opts.EnableBaremetal)
		if !*opts.EnableBaremetal {
			logging.Debug("Baremetal checks disabled via client options")
		}
	}

	if opts.EnableGPU != nil {
		if *opts.EnableGPU {
			enable := true
			// double check we have hardware accelerators
			if acc := devices.DetectAccelerators(); acc == nil || len(acc.Devices) == 0 {
				logging.Warn("No accelerators detected, GPU logic disabled.")
				enable = false
			} else {
				logging.Debugf("Detected %d accelerator(s), enabling GPU logic", len(acc.Devices))
			}
			config.SetEnabledGPU(enable)
		} else {
			logging.Debug("GPU support disabled via client options")
			config.SetEnabledGPU(false)
		}
	}

	if !config.IsGPUEnabled() {
		logging.Debug("GPU support is disabled So skipping accelerator detection and disabling preflight check")
		config.SetSkipPrecheck(true) // No GPU, so skip preflight
	}

	if opts.CacheDir != "" {
		cacheDir := opts.CacheDir
		if err = os.MkdirAll(cacheDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("failed to create cache dir: %w", err)
		}
		constants.ExtractCacheDir = cacheDir
	}

	// If caller asked to skip preflight, do not run it here or downstream.
	// Otherwise, run it ONCE here, and then set SkipPrecheck=true so downstream won’t repeat it.
	shouldRunPreflight := config.IsGPUEnabled() && !config.IsSkipPrecheckEnabled()
	if shouldRunPreflight {
		matchedIDs, unmatchedIDs, err = PreflightCheck(opts.ImageName)
		if err != nil {
			return nil, nil, fmt.Errorf("preflight check failed: %w", err)
		}
		// Prevent duplicate preflight inside extract
		config.SetSkipPrecheck(true)
		logging.WithFields(logging.Fields{
			"matched":   matchedIDs,
			"unmatched": unmatchedIDs,
		}).Info("Preflight completed")
	} else if config.IsSkipPrecheckEnabled() {
		logging.Debug("Skipping preflight (requested by options)")
	} else if !config.IsSkipPrecheckEnabled() {
		logging.Debug("Skipping preflight (GPU disabled)")
	}

	return matchedIDs, unmatchedIDs, fetcher.New().FetchAndExtractCache(opts.ImageName)
}

// GetSystemGPUInfo returns a summary of GPU devices with information
//
//	gpuType: e.g. nvidia-a100
//	driverVersion: e.g. 535.43.02
//	ids: e.g. [0, 1, 2, 3, 4, 5, 6, 7]
//
// If GPU support is not explicitly enabled, it auto-detects hardware
// accelerators and enables GPU logic if supported hardware is found.
// If no GPUs are found, it returns nil without an error.
func GetSystemGPUInfo(opts HwOptions) (*devices.GPUFleetSummary, error) {
	if !config.IsInitialized() {
		if _, err := config.Initialize(config.ConfDir); err != nil {
			return nil, fmt.Errorf("failed to initialize config: %w", err)
		}
	}

	if opts.EnableStub != nil {
		config.SetEnabledStub(*opts.EnableStub)
		if *opts.EnableStub {
			logging.Debug("Stub Mode enabled via client options")
		} else {
			logging.Debug("Stub Mode disabled via client options")
		}
	}

	config.SetTimeout(opts.Timeout)
	if opts.Timeout > 0 {
		logging.Debugf("Hardware detection timeout set to %d seconds", opts.Timeout)
	} else {
		logging.Debug("Hardware detection timeout disabled")
	}

	// Auto-detect accelerator hardware if GPU is not already enabled
	if accs := devices.DetectAccelerators(); accs == nil || len(accs.Devices) == 0 {
		logging.Info("No accelerators detected, GPU logic disabled.")
		return nil, nil
	} else {
		logging.Infof("Detected %d accelerator(s)", len(accs.Devices))
		logging.Debug("Initializing the accelerator(s)")
		// Initialize the GPU accelerator
		acc, err := accelerator.New(config.GPU, true)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize GPU accelerator: %w", err)
		}

		if acc == nil || acc.Device() == nil {
			return nil, fmt.Errorf("accelerator initialization returned nil")
		}

		// Register the accelerator
		accelerator.GetAcceleratorRegistry().RegisterAccelerator(acc)

		// Fetch GPU device information
		summary, err := accelerator.SummarizeGPUs()
		if err != nil {
			return nil, fmt.Errorf("failed to get GPU info: %w", err)
		}
		return summary, nil
	}
}

// PrintGPUSummary prints the fleet summary in a human-friendly form.
func PrintGPUSummary(summary *devices.GPUFleetSummary) {
	if summary == nil || len(summary.GPUs) == 0 {
		fmt.Println("No GPUs found.")
		return
	}

	fmt.Println("GPU Fleet:")
	for _, g := range summary.GPUs {
		fmt.Printf("  - GPU Type: %s\n", g.GPUType)
		fmt.Printf("    Driver Version: %s\n", g.DriverVersion)
		fmt.Printf("    IDs: %v\n", g.IDs)
	}
}

// PreflightCheck performs a compatibility check between the system’s detected GPUs
// and the image’s embedded metadata (via summary label). This is a lightweight check
// (label-only) intended to quickly identify supported GPUs for a given image.
//
// Returns slices of matched and unmatched GPUs, along with any error encountered.
func PreflightCheck(imageName string) (matchedIDs, unmatchedIDs []int, err error) {
	if !config.IsInitialized() {
		if _, err = config.Initialize(config.ConfDir); err != nil {
			return nil, nil, fmt.Errorf("failed to initialize config: %w", err)
		}
	}

	// Initialize the GPU accelerator
	acc, err := accelerator.New(config.GPU, true)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to initialize GPU accelerator: %w", err)
	}

	// Register the accelerator
	accelerator.GetAcceleratorRegistry().RegisterAccelerator(acc)
	// Get device info (handles detection + accelerator setup)
	devInfo, err := preflightcheck.GetAllGPUInfo(acc)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get system GPU info: %w", err)
	}

	// Fetch the image
	img, err := fetcher.NewImgFetcher().FetchImg(imageName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch image: %w", err)
	}

	// Run the compatibility check
	matched, unmatched, err := preflightcheck.CompareCacheSummaryLabelToGPU(img, nil, devInfo)
	if err != nil {
		return nil, nil, fmt.Errorf("preflight check failed: %w", err)
	}

	// Convert matched/unmatched TritonGPUInfo slices into GPU IDs
	matchedIDs = extractGPUIDs(matched)
	unmatchedIDs = extractGPUIDs(unmatched)

	logging.Info("Preflight GPU compatibility check passed.")
	return matchedIDs, unmatchedIDs, nil
}

func extractGPUIDs(infos []devices.TritonGPUInfo) []int {
	ids := make([]int, 0, len(infos))
	for _, ti := range infos {
		ids = append(ids, ti.ID)
	}
	return ids
}
