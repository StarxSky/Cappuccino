package system

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/starxsky/Cappuccino/internal/metrics"
)

func GetSoCInfo() (*metrics.SoCInfo, error) {
	cpuInfo, err := getCPUInfo()
	if err != nil {
		return nil, err
	}

	coreCounts, err := getCoreCounts()
	if err != nil {
		return nil, err
	}

	gpuCores := getGPUCores()

	eCoreCount := coreCounts["hw.perflevel1.logicalcpu"]
	pCoreCount := coreCounts["hw.perflevel0.logicalcpu"]

	soc := &metrics.SoCInfo{
		Name:         cpuInfo["machdep.cpu.brand_string"].(string),
		CoreCount:    cpuInfo["machdep.cpu.core_count"].(int),
		ECoreCount:   eCoreCount,
		PCoreCount:   pCoreCount,
		GPUCoreCount: gpuCores,
	}

	setPowerAndBandwidth(soc)
	return soc, nil
}

func getCPUInfo() (map[string]interface{}, error) {
	cmd := exec.Command("sysctl", "-a")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	info := make(map[string]interface{})
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "machdep.cpu.brand_string") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				info["machdep.cpu.brand_string"] = strings.TrimSpace(parts[1])
			}
		}
		if strings.Contains(line, "machdep.cpu.core_count") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				val, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
				info["machdep.cpu.core_count"] = val
			}
		}
	}
	return info, nil
}

func getCoreCounts() (map[string]int, error) {
	cmd := exec.Command("sysctl", "hw.perflevel0.logicalcpu", "hw.perflevel1.logicalcpu")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ":")
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			counts[key] = val
		}
	}
	return counts, nil
}

func getGPUCores() int {
	cmd := exec.Command("system_profiler", "-detailLevel", "basic", "SPDisplaysDataType")
	output, err := cmd.Output()
	if err != nil {
		return 0
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Total Number of Cores") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				val, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
				return val
			}
		}
	}
	return 0
}

// List the basic specifications of Apple SoCs.
func setPowerAndBandwidth(soc *metrics.SoCInfo) {
	switch soc.Name {
	case "Apple M1 Max":
		soc.CPUMaxPower = 30
		soc.GPUMaxPower = 60
		soc.CPUMaxBW = 250
		soc.GPUMaxBW = 400
	case "Apple M1 Pro":
		soc.CPUMaxPower = 30
		soc.GPUMaxPower = 30
		soc.CPUMaxBW = 200
		soc.GPUMaxBW = 200
	case "Apple M1":
		soc.CPUMaxPower = 20
		soc.GPUMaxPower = 20
		soc.CPUMaxBW = 70
		soc.GPUMaxBW = 70
	case "Apple M1 Ultra":
		soc.CPUMaxPower = 60
		soc.GPUMaxPower = 120
		soc.CPUMaxBW = 500
		soc.GPUMaxBW = 800
	case "Apple M2":
		soc.CPUMaxPower = 25
		soc.GPUMaxPower = 15
		soc.CPUMaxBW = 100
		soc.GPUMaxBW = 100
	case "Apple M2 Pro":
		soc.CPUMaxPower = 35
		soc.GPUMaxPower = 40
		soc.CPUMaxBW = 200
		soc.GPUMaxBW = 400
	case "Apple M2 Max":
		soc.CPUMaxPower = 35
		soc.GPUMaxPower = 80
		soc.CPUMaxBW = 400
		soc.GPUMaxBW = 800
	case "Apple M2 Ultra":
		soc.CPUMaxPower = 70
		soc.GPUMaxPower = 160
		soc.CPUMaxBW = 800
		soc.GPUMaxBW = 1600
	case "Apple M3":
		soc.CPUMaxPower = 27
		soc.GPUMaxPower = 20
		soc.CPUMaxBW = 100
		soc.GPUMaxBW = 100
	case "Apple M3 Pro":
		soc.CPUMaxPower = 35
		soc.GPUMaxPower = 40
		soc.CPUMaxBW = 150
		soc.GPUMaxBW = 300
	case "Apple M3 Max":
		soc.CPUMaxPower = 50
		soc.GPUMaxPower = 80
		soc.CPUMaxBW = 300
		soc.GPUMaxBW = 600
	case "Apple M3 Ultra":
		soc.CPUMaxPower = 100
		soc.GPUMaxPower = 160
		soc.CPUMaxBW = 600
		soc.GPUMaxBW = 1200
	default:
		soc.CPUMaxPower = 20
		soc.GPUMaxPower = 20
		soc.CPUMaxBW = 70
		soc.GPUMaxBW = 70
	}
}

func GetRAMMetrics() (*metrics.RAMMetrics, error) {
	cmd := exec.Command("vm_stat")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	// Apple Silicon uses 16 KiB pages; read the actual size from vm_stat's
	// header instead of assuming 4 KiB, otherwise memory figures are off ~4x.
	pageSize := uint64(4096)
	pages := make(map[string]uint64)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if idx := strings.Index(line, "page size of"); idx >= 0 {
			fields := strings.Fields(line[idx+len("page size of"):])
			if len(fields) > 0 {
				if ps, err := strconv.ParseUint(fields[0], 10, 64); err == nil && ps > 0 {
					pageSize = ps
				}
			}
		}
		parts := strings.Split(line, ":")
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			valStr := strings.TrimSpace(strings.TrimSuffix(parts[1], "."))
			val, _ := strconv.ParseUint(valStr, 10, 64)
			pages[key] = val
		}
	}

	freePages := pages["Pages free"] + pages["Pages speculative"]
	activePages := pages["Pages active"]
	inactivePages := pages["Pages inactive"]
	wiredPages := pages["Pages wired down"]
	compressedPages := pages["Pages occupied by compressor"]

	totalPages := freePages + activePages + inactivePages + wiredPages + compressedPages
	usedPages := activePages + wiredPages + compressedPages

	totalGB := float64(totalPages*pageSize) / (1024 * 1024 * 1024)
	usedGB := float64(usedPages*pageSize) / (1024 * 1024 * 1024)
	freeGB := float64(freePages*pageSize) / (1024 * 1024 * 1024)
	freePercent := int(100 - (float64(freePages) / float64(totalPages) * 100))

	cmd = exec.Command("sysctl", "vm.swapusage")
	output, err = cmd.Output()
	var swapTotalGB, swapUsedGB, swapFreeGB float64
	var swapFreePercent *int
	if err == nil {
		line := strings.TrimSpace(string(output))
		if strings.Contains(line, "total =") {
			parts := strings.Fields(line)
			for i, p := range parts {
				if p == "total" && i+2 < len(parts) {
					swapTotalGB = parseSize(parts[i+2])
				}
				if p == "used" && i+2 < len(parts) {
					swapUsedGB = parseSize(parts[i+2])
				}
				if p == "free" && i+2 < len(parts) {
					swapFreeGB = parseSize(parts[i+2])
				}
			}
			if swapTotalGB > 0 {
				p := int(100 - (swapFreeGB / swapTotalGB * 100))
				swapFreePercent = &p
			}
		}
	}

	return &metrics.RAMMetrics{
		TotalGB:         round1(totalGB),
		FreeGB:          round1(freeGB),
		UsedGB:          round1(usedGB),
		FreePercent:     freePercent,
		SwapTotalGB:     round1(swapTotalGB),
		SwapUsedGB:      round1(swapUsedGB),
		SwapFreeGB:      round1(swapFreeGB),
		SwapFreePercent: swapFreePercent,
	}, nil
}

func parseSize(s string) float64 {
	s = strings.TrimSpace(s)
	var val float64
	var unit string
	fmt.Sscanf(s, "%f%s", &val, &unit)
	switch unit {
	case "K", "KB", "KiB":
		return val / 1024 / 1024
	case "M", "MB", "MiB":
		return val / 1024
	case "G", "GB", "GiB":
		return val
	case "T", "TB", "TiB":
		return val * 1024
	}
	return 0
}

func round1(f float64) float64 {
	return float64(int(f*10)) / 10
}

func ClearConsole() {
	cmd := exec.Command("clear")
	cmd.Stdout = bytes.NewBufferString("")
	cmd.Run()
}
