package devices

import (
	"fmt"
	"os"

	"github.com/jaypipes/ghw"
	"github.com/jaypipes/ghw/pkg/accelerator"
	"github.com/jaypipes/ghw/pkg/pci"
	"github.com/jaypipes/pcidb"
	logging "github.com/sirupsen/logrus"

	"github.com/redhat-et/GKM/mcv/pkg/config"
	"github.com/redhat-et/GKM/mcv/pkg/constants"
)

// EnvStubProfile selects which GPU type the stub simulates.
// Set to "amd" (default), "gaudi", or "nvidia".
// Example: MCV_STUB_PROFILE=amd ./mcv
const EnvStubProfile = "MCV_STUB_PROFILE"

// stubCardSpec holds per-card data for a single simulated accelerator.
type stubCardSpec struct {
	Name     string
	UUID     string
	Arch     string
	WarpSize int
	MemoryMB uint64
	Backend  string
}

// stubProfile groups all data needed to simulate a GPU in stub mode.
// Add a new profile here to support an additional GPU type; no other
// files need to change.
type stubProfile struct {
	DeviceName    string
	DeviceType    DeviceType
	HwType        string
	DriverVersion string
	PciVendorName string
	PciVendorID   string
	Cards         []stubCardSpec
}

// Pre-built profiles — one per GPU family. Add entries here to extend.
var (
	gaudiStubProfile = &stubProfile{
		DeviceName:    "STUBBED Habana",
		DeviceType:    GAUDI,
		HwType:        config.GPU,
		DriverVersion: "1.24.1-b336d5e",
		PciVendorName: "STUBBED Habana",
		PciVendorID:   "1da3", // Habana Labs (Intel) PCI vendor ID
		Cards: []stubCardSpec{
			{Name: productHL325L, UUID: "01P4-HL3090A0-18-UAN071-12-03-01", Arch: archGaudi3, WarpSize: 0, MemoryMB: 131072, Backend: constants.BackendHPU},
			{Name: productHL325L, UUID: "01P4-HL3090A0-18-UAN071-05-08-05", Arch: archGaudi3, WarpSize: 0, MemoryMB: 131072, Backend: constants.BackendHPU},
		},
	}

	amdStubProfile = &stubProfile{
		DeviceName:    "STUBBED AMD",
		DeviceType:    AMD,
		HwType:        config.GPU,
		DriverVersion: "6.12.10-100.fc40.x86_64",
		PciVendorName: "STUBBED AMD",
		PciVendorID:   "1002", // AMD PCI vendor ID
		Cards: []stubCardSpec{
			{Name: "card0", UUID: "daff740f-0000-1000-8062-0165038984ec", Arch: gfxArchMI210, WarpSize: 64, MemoryMB: 65520, Backend: constants.BackendHIP},
			{Name: "card1", UUID: "acff740f-0000-1000-806b-c6ef57f28db1", Arch: gfxArchMI210, WarpSize: 64, MemoryMB: 65520, Backend: constants.BackendHIP},
		},
	}

	// nvidiaStubProfile is a placeholder — fill in real values when needed.
	nvidiaStubProfile = &stubProfile{
		DeviceName:    "STUBBED NVIDIA",
		DeviceType:    NVML,
		HwType:        config.GPU,
		DriverVersion: "550.54.15",
		PciVendorName: "STUBBED NVIDIA",
		PciVendorID:   "10de", // NVIDIA PCI vendor ID
		Cards: []stubCardSpec{
			{Name: "Tesla A100-SXM4-80GB", UUID: "GPU-00000000-0000-0000-0000-000000000001", Arch: "sm_80", WarpSize: 32, MemoryMB: 81920, Backend: constants.BackendCUDA},
			{Name: "Tesla A100-SXM4-80GB", UUID: "GPU-00000000-0000-0000-0000-000000000002", Arch: "sm_80", WarpSize: 32, MemoryMB: 81920, Backend: constants.BackendCUDA},
		},
	}
)

// activeStubProfile returns the stub profile indicated by MCV_STUB_PROFILE.
// Defaults to AMD when the variable is unset. Logs a warning and falls back
// to AMD for any unrecognized value.
func activeStubProfile() *stubProfile {
	val := os.Getenv(EnvStubProfile)
	switch val {
	case "", "amd":
		return amdStubProfile
	case "gaudi":
		return gaudiStubProfile
	case "nvidia":
		return nvidiaStubProfile
	default:
		logging.Warnf("Unknown MCV_STUB_PROFILE=%q; falling back to amd. Valid values: amd, gaudi, nvidia", val)
		return amdStubProfile
	}
}

// stubbedDeviceCache builds a DeviceCache from a stub profile.
func stubbedDeviceCache(p *stubProfile) *DeviceCache {
	tritonInfo := make([]TritonGPUInfo, len(p.Cards))
	summaries := make([]DeviceSummary, len(p.Cards))
	for i, c := range p.Cards {
		tritonInfo[i] = TritonGPUInfo{
			Name:          c.Name,
			UUID:          c.UUID,
			Arch:          c.Arch,
			WarpSize:      c.WarpSize,
			MemoryTotalMB: c.MemoryMB,
			Backend:       c.Backend,
			ID:            i,
		}
		summaries[i] = DeviceSummary{
			ID:            fmt.Sprintf("%d", i),
			DriverVersion: p.DriverVersion,
			ProductName:   c.Name,
		}
	}
	return &DeviceCache{
		Devices: map[string]CachedDevice{
			config.GPU: {
				Name:       p.DeviceName,
				DeviceType: p.DeviceType,
				HwType:     p.HwType,
				TritonInfo: tritonInfo,
				Summaries:  summaries,
			},
		},
	}
}

// stubbedAcceleratorInfo builds a ghw.AcceleratorInfo from a stub profile.
func stubbedAcceleratorInfo(p *stubProfile) *ghw.AcceleratorInfo {
	devices := make([]*accelerator.AcceleratorDevice, len(p.Cards))
	for i, c := range p.Cards {
		devices[i] = &accelerator.AcceleratorDevice{
			Address: fmt.Sprintf("0000:00:%02x.0", i+1),
			PCIDevice: &pci.Device{
				Vendor: &pcidb.Vendor{
					Name: p.PciVendorName,
					ID:   p.PciVendorID,
				},
				Product: &pcidb.Product{
					Name: "STUBBED " + c.Name,
					ID:   "STUBBED " + c.Name,
				},
				Driver: "dummy",
				Class: &pcidb.Class{
					Name: "controller",
					ID:   "0300",
				},
			},
		}
	}
	return &ghw.AcceleratorInfo{Devices: devices}
}
