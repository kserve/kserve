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
	"encoding/json"
	"errors"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/kserve/kserve/mcv/pkg/config"
	"github.com/kserve/kserve/mcv/pkg/constants"
	logging "github.com/sirupsen/logrus"
	"golang.org/x/exp/maps"
)

const (
	MOCK DeviceType = iota
	AMD
	NVML
	ROCM

	// GPU architecture and backend constants
	gfxArchMI210   = "gfx90a"
	hipBackend     = "hip"
	stubbedAMDName = "STUBBED AMD"
)

var (
	deviceRegistry *Registry
	once           sync.Once
	cacheFilePath  = constants.DefaultCacheFilePath
	Timeout        = 10                                   // Timeout in minutes for device detection (0 = disabled)
	CacheTTL       = time.Duration(Timeout) * time.Minute // Cache Time-To-Live
)

type (
	DeviceType        int
	deviceStartupFunc func() Device // Function prototype to startup a new device instance.
	Registry          struct {
		Registry map[string]map[DeviceType]DeviceInfo // Static map of supported Devices Startup functions
	}
)

type DeviceInfo struct {
	startupFunc deviceStartupFunc
	instance    Device
}
type DeviceCache struct {
	Timestamp time.Time
	Devices   map[string]CachedDevice // Store serialized device information
}

type CachedDevice struct {
	Name       string          `json:"name"`
	DeviceType DeviceType      `json:"deviceType"`
	HwType     string          `json:"hwType"`
	TritonInfo []TritonGPUInfo `json:"tritonInfo"`
	Summaries  []DeviceSummary `json:"summaries"`
}

func (d DeviceType) String() string {
	return [...]string{"MOCK", "AMD", "NVML", "ROCM"}[d]
}

type Device interface {
	// Name returns the name of the device
	Name() string
	// DevType returns the type of the device (nvml, ...)
	DevType() DeviceType
	// GetHwType returns the type of hw the device is (gpu, processor)
	HwType() string
	// InitLib the external library loading, if any.
	InitLib() error
	// Init initizalizes and start the metric device
	Init() error
	// Shutdown stops the metric device
	Shutdown() bool
	// GetGPUInfo returns the triton info for a specific GPU
	GetGPUInfo(gpuID int) (TritonGPUInfo, error) // TODO rename
	GetSummary(gpuID int) (DeviceSummary, error)
	// GetAllGPUInfo returns the triton info for a all GPUs on the host
	GetAllGPUInfo() ([]TritonGPUInfo, error) // TODO rename
	GetAllSummaries() ([]DeviceSummary, error)
}

type DeviceSummary struct {
	ID            string
	DriverVersion string
	ProductName   string
}

type GPUFleetSummary struct {
	GPUs []GPUGroup `json:"gpus" yaml:"gpus"`
}

type GPUGroup struct {
	GPUType       string `json:"gpuType" yaml:"gpuType"`
	DriverVersion string `json:"driverVersion" yaml:"driverVersion"`
	IDs           []int  `json:"ids" yaml:"ids"`
}

// Registry gets the default device Registry instance
func GetRegistry() *Registry {
	logging.Debugf("Retrieving the global device registry")
	once.Do(func() {
		Timeout = config.Timeout()
		logging.Debugf("Timeout set to %v", Timeout)
		CacheTTL = time.Duration(Timeout) * time.Minute
		deviceRegistry = newRegistry()
		registerDevices(deviceRegistry)
		if Timeout == 0 {
			logging.Debug("Device cache TTL disabled (timeout = 0)")
			return
		}
		logging.Debugf("Device cache TTL set to %v", CacheTTL)
	})
	return deviceRegistry
}

// NewRegistry creates a new instance of Registry without registering devices
func newRegistry() *Registry {
	return &Registry{
		Registry: map[string]map[DeviceType]DeviceInfo{},
	}
}

// SetRegistry replaces the global registry instance
// NOTE: All plugins will need to be manually registered
// after this function is called.
func SetRegistry(registry *Registry) {
	deviceRegistry = registry
	registerDevices(deviceRegistry)
}

// Register all available devices in the global registry
func registerDevices(r *Registry) {
	if config.IsStubEnabled() {
		cacheFilePath = constants.StubbedCacheFile
		logging.Debugf("Running in stubbed mode, loading static device config")
		staticCheck(r)
	} else {
		// Call individual device check functions
		amdCheck(r)
		rocmCheck(r)
		nvmlCheck(r)
	}
}

func (r *Registry) MustRegister(a string, d DeviceType, deviceStartup deviceStartupFunc) {
	_, ok := r.Registry[a][d]
	if ok {
		logging.Debugf("Device with type %s already exists", d)
		return
	}
	logging.Debugf("Adding the device to the registry [%s][%s]", a, d.String())
	r.Registry[a] = map[DeviceType]DeviceInfo{
		d: {startupFunc: deviceStartup},
	}
}

func (r *Registry) Unregister(d DeviceType) {
	for a := range r.Registry {
		_, exists := r.Registry[a][d]
		if exists {
			delete(r.Registry[a], d)
			return
		}
	}
	logging.Debugf("Device with type %s doesn't exist", d)
}

// GetAllDeviceTypes returns a slice with all the registered devices.
func (r *Registry) GetAllDeviceTypes() []string {
	devices := append([]string{}, maps.Keys(r.Registry)...)
	return devices
}

func addDeviceInterface(registry *Registry, dtype DeviceType, accType string, deviceStartup deviceStartupFunc) error {
	switch accType {
	case config.GPU:
		switch dtype {
		case AMD:
			registry.Unregister(ROCM)
		case ROCM:
			if _, ok := registry.Registry[config.GPU][AMD]; ok {
				return errors.New("AMD already registered. Skipping ROCM")
			}
		}

		logging.Debugf("Try to Register %s", dtype)
		registry.MustRegister(accType, dtype, deviceStartup)

	default:
		logging.Debugf("Try to Register %s", dtype)
		registry.MustRegister(accType, dtype, deviceStartup)
	}

	logging.Debugf("Registered %s", dtype)

	return nil
}

func loadCache() (*DeviceCache, error) {
	if _, err := os.Stat(cacheFilePath); os.IsNotExist(err) {
		return nil, errors.New("cache file does not exist")
	}

	logging.Debugf("Loading device cache from %s", cacheFilePath)
	file, err := os.Open(cacheFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var cache DeviceCache
	if err := json.NewDecoder(file).Decode(&cache); err != nil {
		return nil, err
	}

	if Timeout > 0 {
		// Check if the cache is still valid - if it's expired, update it
		if time.Since(cache.Timestamp) > CacheTTL {
			return nil, errors.New("cache expired")
		}
	}

	logging.Debugf("Loaded %d devices from the device cache", len(cache.Devices))
	return &cache, nil
}

func saveCache(devices map[string]Device) error {
	cache := DeviceCache{
		Timestamp: time.Now(),
		Devices:   make(map[string]CachedDevice),
	}

	for name, device := range devices {
		tritonInfo, err := device.GetAllGPUInfo()
		if err != nil {
			logging.Errorf("Failed to get GPU info for device %s: %v", name, err)
			continue
		}

		summaries, err := device.GetAllSummaries()
		if err != nil {
			logging.Errorf("Failed to get summaries for device %s: %v", name, err)
			continue
		}

		// Store all relevant information in the cache
		cache.Devices[name] = CachedDevice{
			Name:       device.Name(),
			DeviceType: device.DevType(),
			HwType:     device.HwType(),
			TritonInfo: tritonInfo,
			Summaries:  summaries,
		}
	}

	file, err := os.Create(cacheFilePath)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(cache)
}

// Startup initializes and returns a new Device according to the given DeviceType [NVML|OTHER].
func Startup(a string, registry *Registry) Device {
	logging.Debugf("Starting up device of type %s", a)

	cache, err := loadCache()
	if err == nil {
		if cachedDevice, ok := cache.Devices[a]; ok {
			logging.Debugf("Using cached configuration for %s", a)
			if deviceInfo, ok := registry.Registry[a][cachedDevice.DeviceType]; ok {
				// Create an empty instance of the device
				var device Device
				switch cachedDevice.DeviceType {
				case AMD:
					device = &gpuAMD{}
				case NVML:
					device = &gpuNvml{}
				case ROCM:
					device = &gpuROCm{}
				default:
					logging.Errorf("Unsupported device type %s", cachedDevice.DeviceType.String())
					return nil
				}

				// Initialize the device with cached data
				if err := initializeDeviceFromCache(device, &cachedDevice); err != nil {
					logging.Errorf("Failed to initialize device %s from cache: %v", a, err)
					return nil
				}

				// Update the registry with the restored device instance
				deviceInfo.instance = device
				registry.Registry[a][cachedDevice.DeviceType] = deviceInfo

				logging.Debugf("Restored device instance for %s from cache", a)
				return device
			}
			logging.Errorf("No startup function found for cached device type %s", cachedDevice.DeviceType.String())
		}
	}

	// If no cache or restoration failed, probe for the device
	for d := range registry.Registry[a] {
		// Check if there are already instances of the device
		deviceInfo, ok := registry.Registry[a][d]
		if !ok {
			continue
		}

		// Attempt to start the device
		logging.Debugf("Starting up %s", d.String())
		device := deviceInfo.startupFunc()
		if device == nil {
			logging.Errorf("Failed to start device of type %s", d.String())
			continue
		}

		// Add the new device instance
		deviceInfo.instance = device
		registry.Registry[a][d] = deviceInfo

		// Save the device to the cache
		saveCache(map[string]Device{a: device})

		return device
	}

	// The device type is unsupported
	logging.Errorf("unsupported Device")
	return nil
}

// initializeDeviceFromCache initializes a device instance with cached data.
func initializeDeviceFromCache(device Device, cachedDevice *CachedDevice) error {
	// Set device properties based on cached data
	if setter, ok := device.(interface {
		SetName(string)
		SetDeviceType(DeviceType)
		SetHwType(string)
		SetTritonInfo([]TritonGPUInfo)
		SetSummaries([]DeviceSummary)
	}); ok {
		setter.SetName(cachedDevice.Name)
		setter.SetDeviceType(cachedDevice.DeviceType)
		setter.SetHwType(cachedDevice.HwType)
		setter.SetTritonInfo(cachedDevice.TritonInfo)
		setter.SetSummaries(cachedDevice.Summaries)
		logging.Debugf("Device %s restored from cache with %d summaries and %d Triton GPU info entries",
			cachedDevice.Name, len(cachedDevice.Summaries), len(cachedDevice.TritonInfo))
		return nil
	}
	return errors.New("device does not support initialization from cache")
}

// SummarizeDevice generates a summary of GPU devices grouped by their product name
// and driver version. It fetches all summaries from the provided device, organizes
// them into groups, and returns a sorted summary of GPU groups.
//
// The function performs the following steps:
// 1. Fetches all summaries from the device using the GetAllSummaries method.
// 2. Groups the summaries by a combination of product name and driver version.
// 3. Converts string-based IDs to integers for sorting purposes.
// 4. Builds a deterministic, sorted output of GPU groups based on GPU type and driver version.
//
// Parameters:
//   - device: The Device interface that provides access to GPU summaries.
//
// Returns:
//   - *GPUFleetSummary: A pointer to a GPUFleetSummary containing the grouped and sorted GPU data.
//   - error: An error if fetching summaries from the device fails.
func SummarizeDevice(device Device) (*GPUFleetSummary, error) {
	// Fetch summaries from the device
	summaries, err := device.GetAllSummaries()
	if err != nil {
		return nil, err
	}

	// Group by (ProductName, DriverVersion)
	type key struct {
		product string
		driver  string
	}
	groups := map[key]*GPUGroup{}

	for _, s := range summaries {
		idInt, _ := strconv.Atoi(s.ID) // IDs are strings in DeviceSummary; best-effort parse

		k := key{product: s.ProductName, driver: s.DriverVersion}
		if _, ok := groups[k]; !ok {
			groups[k] = &GPUGroup{
				GPUType:       s.ProductName,
				DriverVersion: s.DriverVersion,
				IDs:           []int{},
			}
		}
		groups[k].IDs = append(groups[k].IDs, idInt)
	}

	// Build deterministic, sorted output
	out := &GPUFleetSummary{GPUs: make([]GPUGroup, 0, len(groups))}
	for _, g := range groups {
		sort.Ints(g.IDs)
		out.GPUs = append(out.GPUs, *g)
	}
	sort.Slice(out.GPUs, func(i, j int) bool {
		if out.GPUs[i].GPUType == out.GPUs[j].GPUType {
			return out.GPUs[i].DriverVersion < out.GPUs[j].DriverVersion
		}
		return out.GPUs[i].GPUType < out.GPUs[j].GPUType
	})

	return out, nil
}
