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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/kserve/kserve/mcv/pkg/config"
	"github.com/kserve/kserve/mcv/pkg/utils"
	logging "github.com/sirupsen/logrus"
)

const amdHwType = config.GPU

var (
	amdAccImpl = gpuAMD{}
	amdType    DeviceType
)

type gpuAMD struct {
	name       string
	deviceType DeviceType
	hwType     string
	tritonInfo []TritonGPUInfo
	summaries  []DeviceSummary
	devices    map[int]GPUDevice
}

// SetName sets the name of the AMD device.
func (d *gpuAMD) SetName(name string) {
	d.name = name
}

// SetDeviceType sets the device type of the AMD device.
func (d *gpuAMD) SetDeviceType(deviceType DeviceType) {
	d.deviceType = deviceType
}

// SetHwType sets the hardware type of the AMD device.
func (d *gpuAMD) SetHwType(hwType string) {
	d.hwType = hwType
}

// SetTritonInfo sets the Triton GPU information for the AMD device.
// When restoring from cache, this also populates the devices map.
func (d *gpuAMD) SetTritonInfo(info []TritonGPUInfo) {
	d.tritonInfo = info

	// Reinitialize to prevent stale GPUs from previous state
	d.devices = make(map[int]GPUDevice, len(info))
	for _, tritonInfo := range info {
		d.devices[tritonInfo.ID] = GPUDevice{
			ID:         tritonInfo.ID,
			TritonInfo: tritonInfo,
			// Summary will be set by SetSummaries
		}
	}
}

// SetSummaries sets the summaries for the AMD device.
// When restoring from cache, this also updates the Summary field in devices map.
func (d *gpuAMD) SetSummaries(summaries []DeviceSummary) {
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

type AMDGPUInfo struct {
	GPUInfo  map[int]*AMDCardInfo
	ListInfo map[int]*AMDListInfo
}

type AMDCardInfo struct {
	GPU              int            `json:"gpu"`
	ASIC             AMDASIC        `json:"asic"`
	Bus              AMDBus         `json:"bus"`
	VBIOS            AMDVBIOS       `json:"vbios"`
	Driver           AMDDriver      `json:"driver"`
	Board            AMDBoard       `json:"board"`
	RAS              AMDRAS         `json:"ras"`
	Partition        AMDPartition   `json:"partition"`
	SOCPState        string         `json:"soc_pstate"`
	XGMIPlpd         AMDXGMIPlpd    `json:"xgmi_plpd"`
	ProcessIsolation string         `json:"process_isolation"`
	NUMA             AMDNUMA        `json:"numa"`
	VRAM             AMDVRAM        `json:"vram"`
	CacheInfo        []AMDCacheInfo `json:"cache_info"`
}

type AMDASIC struct {
	MarketName            string      `json:"market_name"`
	VendorID              string      `json:"vendor_id"`
	VendorName            string      `json:"vendor_name"`
	SubvendorID           string      `json:"subvendor_id"`
	DeviceID              string      `json:"device_id"`
	SubsystemID           string      `json:"subsystem_id"`
	RevID                 string      `json:"rev_id"`
	ASICSerial            string      `json:"asic_serial"`
	OAMID                 interface{} `json:"oam_id"`
	NumComputeUnits       int         `json:"num_compute_units"`
	TargetGraphicsVersion string      `json:"target_graphics_version"`
}

type AMDBus struct {
	BDF                  string `json:"bdf"`
	MaxPCIeWidth         int    `json:"max_pcie_width"`
	PCIeInterfaceVersion string `json:"pcie_interface_version"`
	SlotType             string `json:"slot_type"`
}

type AMDVBIOS struct {
	Name       string `json:"name"`
	BuildDate  string `json:"build_date"`
	PartNumber string `json:"part_number"`
	Version    string `json:"version"`
}

type AMDDriver struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type AMDBoard struct {
	ModelNumber      string `json:"model_number"`
	ProductSerial    string `json:"product_serial"`
	FRUID            string `json:"fru_id"`
	ProductName      string `json:"product_name"`
	ManufacturerName string `json:"manufacturer_name"`
}

type AMDRAS struct {
	EEPROMVersion   string            `json:"eeprom_version"`
	ParitySchema    string            `json:"parity_schema"`
	SingleBitSchema string            `json:"single_bit_schema"`
	DoubleBitSchema string            `json:"double_bit_schema"`
	PoisonSchema    string            `json:"poison_schema"`
	ECCBlockState   map[string]string `json:"ecc_block_state"`
}

type AMDPartition struct {
	ComputePartition string `json:"compute_partition"`
	MemoryPartition  string `json:"memory_partition"`
	PartitionID      int    `json:"partition_id"`
}

type AMDXGMIPlpd struct {
	NumSupported int       `json:"num_supported"`
	CurrentID    int       `json:"current_id"`
	PLPDs        []AMDPLPD `json:"plpds"`
}

type AMDPLPD struct {
	PolicyID          int    `json:"policy_id"`
	PolicyDescription string `json:"policy_description"`
}

type AMDNUMA struct {
	Node     int `json:"node"`
	Affinity int `json:"affinity"`
}

type AMDVRAM struct {
	Type     string      `json:"type"`
	Vendor   string      `json:"vendor"`
	Size     AMDVRAMSize `json:"size"`
	BitWidth int         `json:"bit_width"`
}

type AMDVRAMSize struct {
	Value int    `json:"value"`
	Unit  string `json:"unit"`
}

type AMDCacheInfo struct {
	Cache            int          `json:"cache"`
	CacheProperties  []string     `json:"cache_properties"`
	CacheSize        AMDUnitValue `json:"cache_size"`
	CacheLevel       int          `json:"cache_level"`
	MaxNumCUShared   int          `json:"max_num_cu_shared"`
	NumCacheInstance int          `json:"num_cache_instance"`
}

type AMDUnitValue struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

type AMDListInfo struct {
	GPU         int    `json:"gpu"`
	BDF         string `json:"bdf"`
	UniqueID    string `json:"uuid"`
	KFDID       int    `json:"kfd_id"`
	NodeID      int    `json:"node_id"`
	PartitionID int    `json:"partition_id"`
}

var gpuToGFXMap = map[string]string{
	"Instinct MI210":                                 gfxArchMI210, // Aldebaran/MI200 [Instinct MI210]
	"Instinct MI300":                                 "gfx90c",     // MI300 series
	"Polaris 10 (RX 400 series)":                     "gfx803",
	"Polaris 11 (RX 500 series)":                     "gfx804",
	"Polaris 30 (RX Vega series)":                    "gfx810",
	"Vega 10 (Radeon VII)":                           "gfx900",
	"Vega 20 (Vega Frontier Edition, Radeon Pro WX)": "gfx906",
	"Navi 10 (RX 5000 series)":                       "gfx908",
	"RDNA (Radeon RX 6000 series)":                   "gfx1010",
	"RDNA 2 (Radeon RX 6000 series)":                 "gfx1030",
	"RDNA 3 (future models)":                         "gfx1100",
}

// Translate product name to GFX architecture
func TranslateGPUToArch(productName string) string {
	switch {
	case strings.Contains(productName, "Instinct MI210"):
		return gfxArchMI210 // Aldebaran/MI200 [Instinct MI210]
	case strings.Contains(productName, "Instinct MI300"):
		return "gfx90c" // MI300 series
	case strings.Contains(productName, "Polaris 10"):
		return "gfx803" // Polaris 10 (RX 400 series)
	case strings.Contains(productName, "Polaris 11"):
		return "gfx804" // Polaris 11 (RX 500 series)
	case strings.Contains(productName, "Polaris 30"):
		return "gfx810" // Polaris 30 (RX Vega series)
	case strings.Contains(productName, "Vega 10"):
		return "gfx900" // Vega 10 (Radeon VII)
	case strings.Contains(productName, "Vega 20"):
		return "gfx906" // Vega 20 (Vega Frontier Edition, Radeon Pro WX)
	case strings.Contains(productName, "Navi 10"):
		return "gfx908" // Navi 10 (RX 5000 series)
	case strings.Contains(productName, "RDNA"):
		return "gfx1010" // RDNA (Radeon RX 6000 series)
	case strings.Contains(productName, "RDNA 2"):
		return "gfx1030" // RDNA 2 (Radeon RX 6000 series)
	case strings.Contains(productName, "RDNA 3"):
		return "gfx1100" // RDNA 3 (future models)
	default:
		return "Unknown architecture for this GPU"
	}
}

func amdCheck(r *Registry) {
	if err := initAMDLib(); err != nil {
		logging.Debugf("Error initializing AMD SMI: %v", err)
		return
	}
	amdType = AMD
	if err := addDeviceInterface(r, amdType, amdHwType, amdDeviceStartup); err == nil {
		logging.Debugf("Using %s to obtain GPU info", amdAccImpl.Name())
	} else {
		logging.Debugf("Error registering amd-smi: %v", err)
	}
}

func amdDeviceStartup() Device {
	a := amdAccImpl
	if err := a.InitLib(); err != nil {
		logging.Errorf("Error initializing %s: %v", amdType.String(), err)
		return nil
	}
	if err := a.Init(); err != nil {
		logging.Errorf("Failed to init device: %v", err)
		return nil
	}
	logging.Debugf("Using %s to obtain GPU info", amdType.String())
	logging.Debugf("AMD device startup completed")
	return &a
}

func initAMDLib() error {
	if utils.HasApp("amd-smi") {
		return nil
	}
	return errors.New("couldn't find amd-smi")
}

func (r *gpuAMD) InitLib() error {
	return initAMDLib()
}

func (r *gpuAMD) Name() string {
	return amdType.String()
}

func (r *gpuAMD) DevType() DeviceType {
	return amdType
}

func (r *gpuAMD) HwType() string {
	return amdHwType
}

func (r *gpuAMD) Init() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gpuInfoList, err := getAllAMDGPUInfo(ctx)
	if err != nil {
		return fmt.Errorf("failed to get GPU information: %v", err)
	}

	r.devices = make(map[int]GPUDevice, len(gpuInfoList.GPUInfo))
	for gpuID, info := range gpuInfoList.GPUInfo {
		memTotal := calculateMemoryMB(info.VRAM.Size.Value, info.VRAM.Size.Unit)
		name := "card" + strconv.Itoa(gpuID)
		prodName, _ := GetProductName(gpuID) // TODO error checking in the future
		r.devices[gpuID] = GPUDevice{
			ID: gpuID,
			TritonInfo: TritonGPUInfo{
				Name:              name,
				UUID:              gpuInfoList.ListInfo[gpuID].UniqueID,
				ComputeCapability: "",
				Arch:              TranslateGPUToArch(info.Board.ProductName),
				WarpSize:          64,
				MemoryTotalMB:     memTotal,
				Backend:           hipBackend,
				ID:                gpuID,
			},
			Summary: DeviceSummary{
				ID:            strconv.Itoa(gpuID),
				ProductName:   prodName,
				DriverVersion: gpuInfoList.GPUInfo[gpuID].Driver.Version,
			},
		}
	}
	return nil
}

// Converts VRAM size to MB, handling different units
func calculateMemoryMB(value int, unit string) uint64 {
	switch unit {
	case "GB":
		return uint64(value * 1024)
	case "KB":
		return uint64(value / 1024)
	default: // Default to MB
		return uint64(value)
	}
}
func (r *gpuAMD) Shutdown() bool {
	return true
}

func getAllAMDGPUInfo(ctx context.Context) (*AMDGPUInfo, error) {
	gpus, err := getAMDGPUInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get GPU info")
	}
	list, err := getAMDListInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get system info")
	}
	return &AMDGPUInfo{
		GPUInfo:  gpus,
		ListInfo: list,
	}, nil
}

func getAMDGPUInfo(ctx context.Context) (map[int]*AMDCardInfo, error) {
	cmd := exec.CommandContext(ctx, "amd-smi", "static", "--json")
	output, err := cmd.Output()
	if err != nil {
		logging.Debugf("failed to execute amd-smi: %v", err)
		return nil, fmt.Errorf("failed to execute amd-smi: %v", err)
	}

	// Define a wrapper struct to match the new JSON structure
	var wrapper struct {
		GPUData []*AMDCardInfo `json:"gpu_data"`
	}

	if err := json.Unmarshal(output, &wrapper); err != nil {
		logging.Debugf("failed to parse amd-smi output going to try compat mode")
		if err := json.Unmarshal(output, &wrapper.GPUData); err != nil {
			return nil, fmt.Errorf("failed to parse amd-smi output: %v", err)
		}
	}

	parsedGPUs := make(map[int]*AMDCardInfo)
	for _, gpu := range wrapper.GPUData {
		parsedGPUs[gpu.GPU] = gpu
	}
	return parsedGPUs, nil
}

func getAMDListInfo(ctx context.Context) (map[int]*AMDListInfo, error) {
	cmd := exec.CommandContext(ctx, "amd-smi", "list", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute amd-smi: %v", err)
	}

	var listInfo []*AMDListInfo
	if err = json.Unmarshal(output, &listInfo); err != nil {
		return nil, fmt.Errorf("failed to parse amd-smi output: %v", err)
	}

	parsedGPUs := make(map[int]*AMDListInfo)
	for _, gpu := range listInfo {
		parsedGPUs[gpu.GPU] = gpu
	}
	return parsedGPUs, nil
}

func (r *gpuAMD) GetAllGPUInfo() ([]TritonGPUInfo, error) {
	var allTritonInfo []TritonGPUInfo
	for gpuID := range r.devices {
		dev := r.devices[gpuID]
		allTritonInfo = append(allTritonInfo, dev.TritonInfo)
		logging.Debugf("GPU %d: %+v", gpuID, dev.TritonInfo)
	}
	return allTritonInfo, nil
}

func (r *gpuAMD) GetGPUInfo(gpuID int) (TritonGPUInfo, error) {
	dev, exists := r.devices[gpuID]
	if !exists {
		return TritonGPUInfo{}, fmt.Errorf("GPU device %d not found", gpuID)
	}
	return dev.TritonInfo, nil
}

func (r *gpuAMD) GetAllSummaries() ([]DeviceSummary, error) {
	// Check if summaries are already cached
	if len(r.summaries) > 0 {
		logging.Debugf("Returning cached summaries for AMD device %s", r.Name())
		return r.summaries, nil
	}

	// Fallback to default behavior if cache is unavailable
	var allAccInfo []DeviceSummary
	for gpuID := range r.devices {
		dev := r.devices[gpuID]
		allAccInfo = append(allAccInfo, dev.Summary)
		logging.Debugf("GPU %d: %+v", gpuID, dev.Summary)
	}
	r.summaries = allAccInfo // Cache the summaries for future calls
	return allAccInfo, nil
}

func (r *gpuAMD) GetSummary(gpuID int) (DeviceSummary, error) {
	dev, exists := r.devices[gpuID]
	if !exists {
		return DeviceSummary{}, fmt.Errorf("GPU device %d not found", gpuID)
	}
	return dev.Summary, nil
}

// NewAMDGPUDevice creates a new AMD GPU device instance.
func NewAMDGPUDevice() Device {
	return &gpuAMD{}
}
