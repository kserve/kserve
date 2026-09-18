package devices

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/redhat-et/GKM/mcv/pkg/config"
	"github.com/redhat-et/GKM/mcv/pkg/utils"
	logging "github.com/sirupsen/logrus"
)

const gaudiHwType = config.GPU

var (
	gaudiAccImpl = gpuGaudi{}
	gaudiType    DeviceType
)

const (
	productHL325L = "HL-325L"
	productHL325  = "HL-325"
	productHL225  = "HL-225"
	productHL225B = "HL-225B"
	archGaudi3    = "gaudi3"
	archGaudi2    = "gaudi2"
)

// productToArch maps hl-smi product names to architecture strings.
var productToArch = map[string]string{
	productHL325L: archGaudi3,
	productHL325:  archGaudi3,
	productHL225:  archGaudi2,
	productHL225B: archGaudi2,
}

type gpuGaudi struct {
	name       string
	deviceType DeviceType
	hwType     string
	tritonInfo []TritonGPUInfo
	summaries  []DeviceSummary
	devices    map[int]GPUDevice
}

type hlsmiDevice struct {
	Index         int
	Name          string
	BusID         string
	DriverVersion string
	UUID          string
	ModuleID      int
	MemoryTotal   int64
	MemoryFree    int64
	MemoryUsed    int64
}

func (g *gpuGaudi) SetName(name string) {
	g.name = name
}

func (g *gpuGaudi) SetDeviceType(deviceType DeviceType) {
	g.deviceType = deviceType
}

func (g *gpuGaudi) SetHwType(hwType string) {
	g.hwType = hwType
}

func (g *gpuGaudi) SetTritonInfo(info []TritonGPUInfo) {
	g.tritonInfo = info
	g.devices = make(map[int]GPUDevice, len(info))
	for _, ti := range info {
		g.devices[ti.ID] = GPUDevice{
			ID:         ti.ID,
			TritonInfo: ti,
		}
	}
}

func (g *gpuGaudi) SetSummaries(summaries []DeviceSummary) {
	g.summaries = summaries
	if g.devices != nil {
		for _, summary := range summaries {
			var gpuID int
			if _, err := fmt.Sscanf(summary.ID, "%d", &gpuID); err == nil {
				if dev, exists := g.devices[gpuID]; exists {
					dev.Summary = summary
					g.devices[gpuID] = dev
				}
			}
		}
	}
}

func gaudiCheck(r *Registry) {
	if !utils.HasApp("hl-smi") {
		logging.Debug("hl-smi not found, skipping Gaudi detection")
		return
	}

	gaudiType = GAUDI
	if err := addDeviceInterface(r, gaudiType, gaudiHwType, gaudiDeviceStartup); err == nil {
		logging.Debugf("Using %s to obtain GPU info", gaudiAccImpl.Name())
	} else {
		logging.Debugf("Error registering Gaudi: %v", err)
	}
}

func gaudiDeviceStartup() Device {
	a := gaudiAccImpl
	if err := a.InitLib(); err != nil {
		logging.Debugf("Error initializing %s: %v", gaudiType.String(), err)
		return nil
	}
	if err := a.Init(); err != nil {
		logging.Errorf("failed to Init device: %v", err)
		return nil
	}
	logging.Debugf("Using %s to obtain GPU info", gaudiType.String())
	return &a
}

func (g *gpuGaudi) Name() string {
	return gaudiType.String()
}

func (g *gpuGaudi) DevType() DeviceType {
	return gaudiType
}

func (g *gpuGaudi) HwType() string {
	return gaudiHwType
}

func (g *gpuGaudi) InitLib() error {
	return nil
}

func (g *gpuGaudi) Init() error {
	devices, err := queryHLSMI()
	if err != nil {
		return fmt.Errorf("failed to query hl-smi: %w", err)
	}

	logging.Debugf("Detected %d Gaudi device(s)", len(devices))
	g.devices = make(map[int]GPUDevice, len(devices))

	for _, dev := range devices {
		arch := "unknown"
		if a, ok := productToArch[dev.Name]; ok {
			arch = a
		}

		tritonInfo := TritonGPUInfo{
			ID:            dev.Index,
			Name:          dev.Name,
			UUID:          dev.UUID,
			Backend:       "hpu",
			Arch:          arch,
			WarpSize:      0,
			MemoryTotalMB: uint64(dev.MemoryTotal),
		}

		summary := DeviceSummary{
			ID:            strconv.Itoa(dev.Index),
			ProductName:   dev.Name,
			DriverVersion: dev.DriverVersion,
		}

		g.devices[dev.Index] = GPUDevice{
			ID:         dev.Index,
			TritonInfo: tritonInfo,
			Summary:    summary,
		}

		logging.Debugf("Gaudi device %d: %s arch=%s mem=%d MiB bus=%s",
			dev.Index, dev.Name, arch, dev.MemoryTotal, dev.BusID)
	}

	return nil
}

func queryHLSMI() ([]hlsmiDevice, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hl-smi", "-Q",
		"index,name,bus_id,driver_version,uuid,module_id,memory.total,memory.free,memory.used",
		"-f", "csv")

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute hl-smi: %w", err)
	}

	return parseHLSMICSV(string(output))
}

// parseHLSMICSV parses CSV output from hl-smi.
// First line is the header, subsequent lines are device data.
// Numeric values may include unit suffixes (e.g., "131072 MiB").
func parseHLSMICSV(output string) ([]hlsmiDevice, error) {
	var devices []hlsmiDevice

	scanner := bufio.NewScanner(strings.NewReader(output))

	if !scanner.Scan() {
		return nil, fmt.Errorf("empty hl-smi output")
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		fields := strings.Split(line, ", ")
		if len(fields) < 9 {
			logging.Debugf("Skipping malformed hl-smi line: %s", line)
			continue
		}

		index, err := strconv.Atoi(strings.TrimSpace(fields[0]))
		if err != nil {
			logging.Debugf("Failed to parse device index %q: %v", fields[0], err)
			continue
		}

		moduleID, _ := strconv.Atoi(strings.TrimSpace(fields[5]))

		devices = append(devices, hlsmiDevice{
			Index:         index,
			Name:          strings.TrimSpace(fields[1]),
			BusID:         strings.TrimSpace(fields[2]),
			DriverVersion: strings.TrimSpace(fields[3]),
			UUID:          strings.TrimSpace(fields[4]),
			ModuleID:      moduleID,
			MemoryTotal:   parseMiBValue(fields[6]),
			MemoryFree:    parseMiBValue(fields[7]),
			MemoryUsed:    parseMiBValue(fields[8]),
		})
	}

	if len(devices) == 0 {
		return nil, fmt.Errorf("no devices found in hl-smi output")
	}

	return devices, nil
}

func parseMiBValue(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, " MiB")
	s = strings.TrimSuffix(s, " W")
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

func (g *gpuGaudi) Shutdown() bool {
	return true
}

func (g *gpuGaudi) GetGPUInfo(gpuID int) (TritonGPUInfo, error) {
	dev, exists := g.devices[gpuID]
	if !exists {
		return TritonGPUInfo{}, fmt.Errorf("GPU device %d not found", gpuID)
	}
	return dev.TritonInfo, nil
}

func (g *gpuGaudi) GetAllGPUInfo() ([]TritonGPUInfo, error) {
	if len(g.tritonInfo) > 0 {
		return g.tritonInfo, nil
	}

	var allTritonInfo []TritonGPUInfo
	for id := range g.devices {
		allTritonInfo = append(allTritonInfo, g.devices[id].TritonInfo)
	}
	g.tritonInfo = allTritonInfo
	return allTritonInfo, nil
}

func (g *gpuGaudi) GetSummary(gpuID int) (DeviceSummary, error) {
	dev, exists := g.devices[gpuID]
	if !exists {
		return DeviceSummary{}, fmt.Errorf("GPU device %d not found", gpuID)
	}
	return dev.Summary, nil
}

func (g *gpuGaudi) GetAllSummaries() ([]DeviceSummary, error) {
	if len(g.summaries) > 0 {
		return g.summaries, nil
	}

	var allSummaries []DeviceSummary
	for id := range g.devices {
		allSummaries = append(allSummaries, g.devices[id].Summary)
	}
	g.summaries = allSummaries
	return allSummaries, nil
}
