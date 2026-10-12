package powermetrics

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	// External
	"howett.net/plist"

	// Private Moudles
	"github.com/starxsky/Cappuccino/internal/metrics"
)

// plistNumber accepts both integer and real plist values. powermetrics emits
// whole numbers as integers (e.g. `cpu_energy` can be `58` or `57.7352`), and
// howett.net/plist rejects decoding an integer into a float64 field.
type plistNumber float64

func (n *plistNumber) UnmarshalPlist(unmarshal func(interface{}) error) error {
	var v interface{}
	if err := unmarshal(&v); err != nil {
		return err
	}
	switch t := v.(type) {
	case int:
		*n = plistNumber(t)
	case int64:
		*n = plistNumber(t)
	case uint64:
		*n = plistNumber(t)
	case float32:
		*n = plistNumber(t)
	case float64:
		*n = plistNumber(t)
	default:
		return fmt.Errorf("plist: unexpected number type %T", v)
	}
	return nil
}

type PowerMetricsData struct {
	Timestamp         time.Time          `plist:"timestamp"`
	Processor         ProcessorData      `plist:"processor"`
	GPU               GPUData            `plist:"gpu"`
	ThermalPressure   string             `plist:"thermal_pressure"`
	BandwidthCounters []BandwidthCounter `plist:"bandwidth_counters"`
}

type ProcessorData struct {
	Clusters      []ClusterData `plist:"clusters"`
	ANEEnergy     plistNumber   `plist:"ane_energy"`
	CPUEnergy     plistNumber   `plist:"cpu_energy"`
	GPUEnergy     plistNumber   `plist:"gpu_energy"`
	CombinedPower plistNumber   `plist:"combined_power"`
}

type ClusterData struct {
	Name      string      `plist:"name"`
	FreqHz    plistNumber `plist:"freq_hz"`
	IdleRatio plistNumber `plist:"idle_ratio"`
	CPUs      []CPUData   `plist:"cpus"`
}

type CPUData struct {
	CPU       int         `plist:"cpu"`
	FreqHz    plistNumber `plist:"freq_hz"`
	IdleRatio plistNumber `plist:"idle_ratio"`
}

type GPUData struct {
	FreqHz    plistNumber `plist:"freq_hz"`
	IdleRatio plistNumber `plist:"idle_ratio"`
}

type BandwidthCounter struct {
	Name  string      `plist:"name"`
	Value plistNumber `plist:"value"`
}

var currentTimecode string
var powermetricsProcess *exec.Cmd

func StartPowermetrics(timecode string, intervalMs int) error {
	currentTimecode = timecode

	matches, _ := filepath.Glob("/tmp/cappuccino_powermetrics*")
	for _, f := range matches {
		os.Remove(f)
	}

	args := []string{
		"nice", "-n", "10",
		"/usr/bin/powermetrics",
		"--samplers", "cpu_power,gpu_power,thermal",
		"-o", "/tmp/cappuccino_powermetrics" + timecode,
		"-f", "plist",
		"-i", strconv.Itoa(intervalMs),
	}

	powermetricsProcess = exec.Command(args[0], args[1:]...)
	powermetricsProcess.Stdout = nil
	powermetricsProcess.Stderr = nil

	return powermetricsProcess.Start()
}

func StopPowermetrics() {
	if powermetricsProcess != nil && powermetricsProcess.Process != nil {
		powermetricsProcess.Process.Kill()
		powermetricsProcess.Wait()
	}
}

func ParsePowermetrics() (*metrics.CPUMetrics, *metrics.GPUMetrics, metrics.ThermalPressure, metrics.BandwidthMetrics, float64, error) {
	path := "/tmp/cappuccino_powermetrics" + currentTimecode

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, "", nil, 0, err
	}

	parts := bytes.Split(data, []byte{0})

	var pm PowerMetricsData
	var decodeErr error
	decoded := false
	for i := len(parts) - 1; i >= 0; i-- {
		if len(parts[i]) == 0 {
			continue
		}
		if _, err := plist.Unmarshal(parts[i], &pm); err == nil {
			decoded = true
			break
		} else {
			decodeErr = err
		}
	}
	if !decoded {
		if decodeErr == nil {
			decodeErr = fmt.Errorf("no powermetrics output yet")
		}
		return nil, nil, "", nil, 0, decodeErr
	}

	// An empty or partially written file can decode into a zero-value struct.
	// Treat that as "not ready" instead of a valid all-zero sample.
	if len(pm.Processor.Clusters) == 0 {
		return nil, nil, "", nil, 0, fmt.Errorf("powermetrics sample not ready")
	}

	cpuMetrics := parseCPUMetrics(pm.Processor)
	gpuMetrics := parseGPUMetrics(pm.GPU)
	thermalPressure := metrics.ThermalPressure(pm.ThermalPressure)
	bandwidthMetrics := parseBandwidthMetrics(pm.BandwidthCounters)

	return cpuMetrics, gpuMetrics, thermalPressure, bandwidthMetrics, float64(pm.Timestamp.UnixMilli()), nil
}

func parseCPUMetrics(proc ProcessorData) *metrics.CPUMetrics {
	eCore := make(map[int]int)
	pCore := make(map[int]int)
	var eClusterActive, eClusterFreq, pClusterActive, pClusterFreq int
	var eClusterFreqs, pClusterFreqs []int
	var eClusterActives, pClusterActives []int

	for _, cluster := range proc.Clusters {
		freqMHz := int(cluster.FreqHz / 1e6)
		active := int((1 - cluster.IdleRatio) * 100)

		isECluster := strings.HasPrefix(cluster.Name, "E")

		for _, cpu := range cluster.CPUs {
			cpuFreq := int(cpu.FreqHz / 1e6)
			cpuActive := int((1 - cpu.IdleRatio) * 100)
			if isECluster {
				eCore[cpu.CPU] = cpuActive
				eClusterFreqs = append(eClusterFreqs, cpuFreq)
				eClusterActives = append(eClusterActives, cpuActive)
			} else {
				pCore[cpu.CPU] = cpuActive
				pClusterFreqs = append(pClusterFreqs, cpuFreq)
				pClusterActives = append(pClusterActives, cpuActive)
			}
		}

		if isECluster {
			eClusterFreq = freqMHz
			eClusterActive = active
		} else {
			pClusterFreq = freqMHz
			pClusterActive = active
		}
	}

	if eClusterActive == 0 && len(eClusterActives) > 0 {
		sum := 0
		for _, v := range eClusterActives {
			sum += v
		}
		eClusterActive = sum / len(eClusterActives)
	}
	if eClusterFreq == 0 && len(eClusterFreqs) > 0 {
		maxFreq := 0
		for _, v := range eClusterFreqs {
			if v > maxFreq {
				maxFreq = v
			}
		}
		eClusterFreq = maxFreq
	}
	if pClusterActive == 0 && len(pClusterActives) > 0 {
		sum := 0
		for _, v := range pClusterActives {
			sum += v
		}
		pClusterActive = sum / len(pClusterActives)
	}
	if pClusterFreq == 0 && len(pClusterFreqs) > 0 {
		maxFreq := 0
		for _, v := range pClusterFreqs {
			if v > maxFreq {
				maxFreq = v
			}
		}
		pClusterFreq = maxFreq
	}

	return &metrics.CPUMetrics{
		EClusterActive:  eClusterActive,
		EClusterFreqMHz: eClusterFreq,
		PClusterActive:  pClusterActive,
		PClusterFreqMHz: pClusterFreq,
		ECoreActive:     eCore,
		PCoreActive:     pCore,
		ANEWatts:        float64(proc.ANEEnergy) / 1000,
		CPUWatts:        float64(proc.CPUEnergy) / 1000,
		GPUWatts:        float64(proc.GPUEnergy) / 1000,
		PackageWatts:    float64(proc.CombinedPower) / 1000,
	}
}

func parseGPUMetrics(gpu GPUData) *metrics.GPUMetrics {
	// powermetrics reports GPU freq_hz in MHz (matching its DVFM state table),
	// unlike the CPU clusters which report Hz. Handle both defensively.
	freqMHz := int(gpu.FreqHz)
	if gpu.FreqHz >= 1e6 {
		freqMHz = int(gpu.FreqHz / 1e6)
	}
	return &metrics.GPUMetrics{
		FreqMHz: freqMHz,
		Active:  int((1 - gpu.IdleRatio) * 100),
	}
}

func parseBandwidthMetrics(counters []BandwidthCounter) metrics.BandwidthMetrics {
	dataFields := []string{
		"PCPU0 DCS RD", "PCPU0 DCS WR",
		"PCPU1 DCS RD", "PCPU1 DCS WR",
		"PCPU2 DCS RD", "PCPU2 DCS WR",
		"PCPU3 DCS RD", "PCPU3 DCS WR",
		"PCPU DCS RD", "PCPU DCS WR",
		"ECPU0 DCS RD", "ECPU0 DCS WR",
		"ECPU1 DCS RD", "ECPU1 DCS WR",
		"ECPU DCS RD", "ECPU DCS WR",
		"GFX DCS RD", "GFX DCS WR",
		"ISP DCS RD", "ISP DCS WR",
		"STRM CODEC DCS RD", "STRM CODEC DCS WR",
		"PRORES DCS RD", "PRORES DCS WR",
		"VDEC DCS RD", "VDEC DCS WR",
		"VENC0 DCS RD", "VENC0 DCS WR",
		"VENC1 DCS RD", "VENC1 DCS WR",
		"VENC2 DCS RD", "VENC2 DCS WR",
		"VENC3 DCS RD", "VENC3 DCS WR",
		"VENC DCS RD", "VENC DCS WR",
		"JPG0 DCS RD", "JPG0 DCS WR",
		"JPG1 DCS RD", "JPG1 DCS WR",
		"JPG2 DCS RD", "JPG2 DCS WR",
		"JPG3 DCS RD", "JPG3 DCS WR",
		"JPG DCS RD", "JPG DCS WR",
		"DCS RD", "DCS WR",
	}

	bw := make(metrics.BandwidthMetrics)
	for _, f := range dataFields {
		bw[f] = 0
	}

	for _, c := range counters {
		if _, ok := bw[c.Name]; ok {
			bw[c.Name] = float64(c.Value) / 1e9
		}
	}

	bw["PCPU DCS RD"] += bw["PCPU0 DCS RD"] + bw["PCPU1 DCS RD"] + bw["PCPU2 DCS RD"] + bw["PCPU3 DCS RD"]
	bw["PCPU DCS WR"] += bw["PCPU0 DCS WR"] + bw["PCPU1 DCS WR"] + bw["PCPU2 DCS WR"] + bw["PCPU3 DCS WR"]
	bw["JPG DCS RD"] += bw["JPG0 DCS RD"] + bw["JPG1 DCS RD"] + bw["JPG2 DCS RD"] + bw["JPG3 DCS RD"]
	bw["JPG DCS WR"] += bw["JPG0 DCS WR"] + bw["JPG1 DCS WR"] + bw["JPG2 DCS WR"] + bw["JPG3 DCS WR"]
	bw["VENC DCS RD"] += bw["VENC0 DCS RD"] + bw["VENC1 DCS RD"] + bw["VENC2 DCS RD"] + bw["VENC3 DCS RD"]
	bw["VENC DCS WR"] += bw["VENC0 DCS WR"] + bw["VENC1 DCS WR"] + bw["VENC2 DCS WR"] + bw["VENC3 DCS WR"]

	bw["MEDIA DCS"] = bw["ISP DCS RD"] + bw["ISP DCS WR"] +
		bw["STRM CODEC DCS RD"] + bw["STRM CODEC DCS WR"] +
		bw["PRORES DCS RD"] + bw["PRORES DCS WR"] +
		bw["VDEC DCS RD"] + bw["VDEC DCS WR"] +
		bw["VENC DCS RD"] + bw["VENC DCS WR"] +
		bw["JPG DCS RD"] + bw["JPG DCS WR"]

	return bw
}
