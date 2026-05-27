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

package devices

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	logging "github.com/sirupsen/logrus"

	"github.com/kserve/kserve/mcv/pkg/config"
)

const (
	nvmlHwType   = config.GPU
	NVMLWarpSize = 32
)

var (
	nvmlAccImpl = gpuNvml{}
	nvmlType    DeviceType
)

type gpuNvml struct {
	libInited  bool
	devices    map[int]GPUDevice // List of GPU identifiers for the device
	name       string
	deviceType DeviceType
	hwType     string
	tritonInfo []TritonGPUInfo
	summaries  []DeviceSummary
}

// SetName sets the name of the NVML device.
func (d *gpuNvml) SetName(name string) {
	d.name = name
}

// SetDeviceType sets the device type of the NVML device.
func (d *gpuNvml) SetDeviceType(deviceType DeviceType) {
	d.deviceType = deviceType
}

// SetHwType sets the hardware type of the NVML device.
func (d *gpuNvml) SetHwType(hwType string) {
	d.hwType = hwType
}

// SetTritonInfo sets the Triton GPU information for the NVML device.
// When restoring from cache, this also populates the devices map.
func (d *gpuNvml) SetTritonInfo(info []TritonGPUInfo) {
	d.tritonInfo = info

	// Rebuild devices map from cached triton info
	// Reinitialize to prevent stale GPUs from previous state
	d.devices = make(map[int]GPUDevice)
	for _, tritonInfo := range info {
		d.devices[tritonInfo.ID] = GPUDevice{
			ID:         tritonInfo.ID,
			TritonInfo: tritonInfo,
			// Summary will be set by SetSummaries
		}
	}
}

// SetSummaries sets the summaries for the NVML device.
// When restoring from cache, this also updates the Summary field in devices map.
func (d *gpuNvml) SetSummaries(summaries []DeviceSummary) {
	d.summaries = summaries

	// Update Summary in devices map if it exists
	if d.devices != nil {
		for _, summary := range summaries {
			// Parse GPU ID from summary.ID (which is a string like "0", "1", etc.)
			var gpuID int
			if _, err := fmt.Sscanf(summary.ID, "%d", &gpuID); err == nil {
				if dev, exists := d.devices[gpuID]; exists {
					dev.Summary = summary
					d.devices[gpuID] = dev
				}
			}
		}
	}
}

func nvmlCheck(r *Registry) {
	if err := nvml.Init(); err != nvml.SUCCESS {
		logging.Debugf("Error initializing nvml: %v", nvmlErrorString(err))
		return
	}
	logging.Debug("Initializing nvml Successful")
	nvmlType = NVML
	if err := addDeviceInterface(r, nvmlType, nvmlHwType, nvmlDeviceStartup); err == nil {
		logging.Debugf("Using %s to obtain GPU info", nvmlAccImpl.Name())
	} else {
		logging.Debugf("Error registering nvml: %v", err)
	}
}

func nvmlDeviceStartup() Device {
	a := nvmlAccImpl
	if err := a.InitLib(); err != nil {
		logging.Debugf("Error initializing %s: %v", nvmlType.String(), err)
		return nil
	}
	if err := a.Init(); err != nil {
		logging.Errorf("failed to Init device: %v", err)
		return nil
	}
	logging.Debugf("Using %s to obtain GPU info", nvmlType.String())
	logging.Debugf("NVML device startup completed")
	return &a
}

func (n *gpuNvml) Name() string {
	return nvmlType.String()
}

func (n *gpuNvml) DevType() DeviceType {
	return nvmlType
}

func (n *gpuNvml) HwType() string {
	return nvmlHwType
}

// Init initizalize and start the GPU Triton info collection
// the nvml only works if the container has support to GPU, e.g., it is using nvidia-docker2
// otherwise it will fail to load the libnvidia-ml.so.1
func (n *gpuNvml) InitLib() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("could not init nvml: %v", r)
		}
	}()
	if ret := nvml.Init(); ret != nvml.SUCCESS {
		err = fmt.Errorf("failed to init nvml. %s", nvmlErrorString(ret))
		return err
	}
	n.libInited = true
	return nil
}

func (n *gpuNvml) Init() (err error) {
	if !n.libInited {
		if err := n.InitLib(); err != nil {
			return err
		}
	}

	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		var errs []string
		errs = append(errs, fmt.Sprintf("failed to get nvml device count: %v", nvml.ErrorString(ret)))
		if ret := nvml.Shutdown(); ret != nvml.SUCCESS {
			errs = append(errs, fmt.Sprintf("failed to shutdown nvml device: %v", nvml.ErrorString(ret)))
		}

		if len(errs) > 0 {
			return fmt.Errorf("%s", strings.Join(errs, "; "))
		}
	}

	logging.Debugf("Found %d GPU devices\n", count)

	n.devices = make(map[int]GPUDevice, count)
	for gpuID := 0; gpuID < count; gpuID++ {
		device, ret := nvml.DeviceGetHandleByIndex(gpuID)
		if ret != nvml.SUCCESS {
			var errs []string
			errs = append(errs, fmt.Sprintf("failed to get NVML device %d: %v", gpuID, nvml.ErrorString(ret)))
			if ret := nvml.Shutdown(); ret != nvml.SUCCESS {
				errs = append(errs, fmt.Sprintf("failed to shutdown nvml device: %v", nvml.ErrorString(ret)))
			}

			if len(errs) > 0 {
				return fmt.Errorf("%s", strings.Join(errs, "; "))
			}
		}

		tritonInfo, err := getNVMLTritonGPUInfo(device)
		tritonInfo.ID = gpuID
		if err != nil {
			return err
		}
		prodName, _ := GetProductName(gpuID)              // TODO error checking in the future
		driverVersion, _ := nvml.SystemGetDriverVersion() // TODO error checking in the future
		dev := GPUDevice{
			ID:         gpuID,
			TritonInfo: tritonInfo,
			Summary: DeviceSummary{ID: strconv.Itoa(gpuID),
				ProductName:   prodName,
				DriverVersion: driverVersion},
		}

		n.devices[gpuID] = dev
		logging.Debugf("GPU %d: %+v", gpuID, dev.TritonInfo)
	}

	// Removed the line n.collectionSupported = true

	return nil
}

// Shutdown stops the GPU metric collector
func (n *gpuNvml) Shutdown() bool {
	n.libInited = false
	return nvml.Shutdown() == nvml.SUCCESS
}

func getNVMLTritonGPUInfo(device nvml.Device) (TritonGPUInfo, error) {
	name, _ := device.GetName()
	uuid, _ := device.GetUUID()

	// Get the major and minor compute capability as integers
	major, minor, _ := device.GetCudaComputeCapability()

	mem, _ := device.GetMemoryInfo()
	warpSize := NVMLWarpSize
	driverVersion, _ := nvml.SystemGetDriverVersion()
	// Split the version string to extract the major version
	versionParts := strings.Split(driverVersion, ".")
	if len(versionParts) < 1 {
		return TritonGPUInfo{}, fmt.Errorf("invalid driver version format")
	}

	// Convert the major version part to an integer (this corresponds to the PTX version)
	ptxVersion, err := strconv.Atoi(versionParts[0])
	if err != nil {
		return TritonGPUInfo{}, fmt.Errorf("failed to parse PTX version: %v", err)
	}

	return TritonGPUInfo{
		Name:              name,
		UUID:              uuid,
		ComputeCapability: fmt.Sprintf("%d.%d", major, minor), // Formatting the compute capability
		Arch:              strconv.Itoa(major*10 + minor),     // Numeric string for Triton compatibility (e.g., "75")
		WarpSize:          warpSize,
		MemoryTotalMB:     mem.Total / (1024 * 1024),
		PTXVersion:        ptxVersion,
		Backend:           "cuda",
	}, nil
}

// GetGPUInfo retrieves the stored GPU info for a specific device ID.
// It returns the GPU info or an error if the device is not found.
func (n *gpuNvml) GetGPUInfo(gpuID int) (TritonGPUInfo, error) {
	dev, exists := n.devices[gpuID]
	if !exists {
		return TritonGPUInfo{}, fmt.Errorf("GPU device %d not found", gpuID)
	}
	return dev.TritonInfo, nil
}

// GetAllGPUInfo retrieves the stored GPU info for all devices on the host.
// It returns a slice of TritonGPUInfo for all GPUs.
func (n *gpuNvml) GetAllGPUInfo() ([]TritonGPUInfo, error) {
	var allTritonInfo []TritonGPUInfo

	for gpuID := range n.devices {
		dev := n.devices[gpuID]
		allTritonInfo = append(allTritonInfo, dev.TritonInfo)
		logging.Debugf("GPU %d: %+v", gpuID, dev.TritonInfo)
	}

	return allTritonInfo, nil
}

func nvmlErrorString(errno nvml.Return) string {
	switch errno {
	case nvml.SUCCESS:
		return "SUCCESS"
	case nvml.ERROR_LIBRARY_NOT_FOUND:
		return "ERROR_LIBRARY_NOT_FOUND"
	}
	return fmt.Sprintf("Error %d", errno)
}

// GetAllSummaries implements Device.
func (n *gpuNvml) GetAllSummaries() ([]DeviceSummary, error) {
	// Check if summaries are already cached
	if len(n.summaries) > 0 {
		logging.Debugf("Returning cached summaries for NVML device %s", n.Name())
		return n.summaries, nil
	}

	// Fallback to default behavior if cache is unavailable
	var allAccInfo []DeviceSummary
	for gpuID := range n.devices {
		dev := n.devices[gpuID]
		allAccInfo = append(allAccInfo, dev.Summary)
		logging.Debugf("GPU %d: %+v", gpuID, dev.Summary)
	}
	n.summaries = allAccInfo // Cache the summaries for future calls
	return allAccInfo, nil
}

// GetSummary implements Device.
func (n *gpuNvml) GetSummary(gpuID int) (DeviceSummary, error) {
	panic("unimplemented")
}
