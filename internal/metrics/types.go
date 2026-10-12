package metrics 

// Metrics For CPU 
type CPUMetrics struct {
	EClusterActive int // The Efficient Cluster Activity
	EClusterFreqMHz int  

	PClusterActive int  // Performance Cluster Activity 
	PClusterFreqMHz int 

	ECoreActive map[int] int 
	PCoreActive map[int] int

	// Power of the monitored object 
	ANEWatts float64 // The power of Apple Neural Engine  ( unit : watts)
	CPUWatts float64 
	GPUWatts float64 
	PackageWatts float64 
}


// The Metric for GPU 
type GPUMetrics struct {
	FreqMHz int 
	Active int 
}

// Bandwidth 
type BandwidthMetrics map [string] float64 

type RAMMetrics struct { 
	TotalGB float64 
	FreeGB float64 
	UsedGB float64 

	FreePercent int 

	SwapTotalGB float64 
	SwapUsedGB float64 
	SwapFreeGB float64 

	SwapFreePercent *int // To avoid ambiguity, pointers are used to denote strictness.

}

// The Soc Infomation 
type SoCInfo struct { 
	// Basic Info
	Name string  // CPU Name 
	CoreCount int  // Count the number of Core of CPU
	// Power
	CPUMaxPower float64 
	GPUMaxPower float64 

	// Bandwidth
	CPUMaxBW float64 // the max bandwidth for CPU
	GPUMaxBW float64 

	// Count Cores 
	ECoreCount int 
	PCoreCount int 
	GPUCoreCount int 

}


type ThermalPressure string 

const (
	ThermalNominal ThermalPressure =  "Nominal"
	ThermalThrottled ThermalPressure = "Throttled"
)

/*
ThermalNominal = Thermal status is normal; there is no significant performance limitation due to temperature.
ThermalThrottled = "Throttled": The system is downclocking or limiting speed due to high temperatures—commonly known as "thermal throttling."
*/