package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gizak/termui/v3"
	"github.com/spf13/cobra"
	"github.com/starxsky/Cappuccino/internal/config"
	"github.com/starxsky/Cappuccino/internal/metrics"
	"github.com/starxsky/Cappuccino/internal/powermetrics"
	"github.com/starxsky/Cappuccino/internal/system"
	"github.com/starxsky/Cappuccino/internal/ui"
)

var cfg *config.Config

func main() {
	cfg = config.DefaultConfig()

	rootCmd := &cobra.Command{
		Use:   "Cappuccino",
		Short: "Performance monitoring CLI tool for Apple Silicon",
		Run:   runCappuccino,
	}

	rootCmd.Flags().IntVarP(&cfg.Interval, "interval", "i", cfg.Interval, "Display interval and sampling interval for powermetrics (seconds)")
	rootCmd.Flags().IntVarP(&cfg.Color, "color", "c", cfg.Color, "Choose display color (0-8)")
	rootCmd.Flags().IntVarP(&cfg.AvgSeconds, "avg", "a", cfg.AvgSeconds, "Interval for averaged values (seconds)")
	rootCmd.Flags().BoolVar(&cfg.ShowCores, "show-cores", cfg.ShowCores, "Show per-core usage")
	rootCmd.Flags().IntVar(&cfg.MaxCount, "max-count", cfg.MaxCount, "Max show count to restart powermetrics")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCappuccino(cmd *cobra.Command, args []string) {
	fmt.Println("Cappuccino - Performance monitoring CLI tool for Apple Silicon")
	fmt.Println("You can update Cappuccino by running `go install github.com/starxsky/Cappuccino@latest`")
	fmt.Println("Get help at `https://github.com/starxsky/Cappuccino`")
	fmt.Println("P.S. You are recommended to run Cappuccino with `sudo Cappuccino`")
	fmt.Println("\n[1/3] Loading Cappuccino")

	socInfo, err := system.GetSoCInfo()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get SoC info: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[2/3] Starting powermetrics process")

	timecode := fmt.Sprintf("%d", time.Now().Unix())
	intervalMs := cfg.Interval * 1000

	if err := powermetrics.StartPowermetrics(timecode, intervalMs); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start powermetrics: %v\n", err)
		os.Exit(1)
	}
	defer powermetrics.StopPowermetrics()

	fmt.Println("[3/3] Waiting for first reading...")

	// Wait for first reading (powermetrics takes ~interval to produce first sample)
	var cpuMetrics *metrics.CPUMetrics
	var gpuMetrics *metrics.GPUMetrics
	var thermalPressure metrics.ThermalPressure
	var timestamp float64

	firstReadTimeout := time.After(10 * time.Second)
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()

waitFirst:
	for {
		select {
		case <-firstReadTimeout:
			fmt.Fprintf(os.Stderr, "Timeout waiting for powermetrics output\n")
			powermetrics.StopPowermetrics()
			os.Exit(1)
		case <-tick.C:
			cpuMetrics, gpuMetrics, thermalPressure, _, timestamp, err = powermetrics.ParsePowermetrics()
			if err != nil || cpuMetrics == nil {
				continue
			}
			fmt.Println("First reading received!")
			break waitFirst
		}
	}

	// Only switch to the fullscreen UI once there is data ready to draw, so
	// the terminal never sits on a blank alternate screen while waiting.
	if err := initTermUI(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize UI: %v\n", err)
		os.Exit(1)
	}

	u := ui.NewUI(cfg, socInfo)
	defer u.Close()

	if ramMetrics, err := system.GetRAMMetrics(); err == nil {
		u.Update(cpuMetrics, gpuMetrics, ramMetrics, thermalPressure, cfg.Interval)
		u.Render()
	}

	lastTimestamp := timestamp

	uiEvents := termui.PollEvents()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	count := 0
	ticker := time.NewTicker(time.Duration(cfg.Interval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-sigChan:
			fmt.Println("\nStopping...")
			return
		case e := <-uiEvents:
			if e.Type == termui.ResizeEvent {
				u.Render()
				continue
			}
			res := u.HandleKey(e.ID)
			if res.Quit {
				return
			}
			if res.Interval > 0 {
				cfg.Interval = res.Interval
				intervalMs = cfg.Interval * 1000
				u.RecalcAvg()
				powermetrics.StopPowermetrics()
				timecode = fmt.Sprintf("%d", time.Now().Unix())
				if err := powermetrics.StartPowermetrics(timecode, intervalMs); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to restart powermetrics: %v\n", err)
					return
				}
				lastTimestamp = 0
				ticker.Reset(time.Duration(cfg.Interval) * time.Second)
			}
			u.Render()
		case <-ticker.C:
			if cfg.MaxCount > 0 {
				if count >= cfg.MaxCount {
					count = 0
					powermetrics.StopPowermetrics()
					timecode = fmt.Sprintf("%d", time.Now().Unix())
					if err := powermetrics.StartPowermetrics(timecode, intervalMs); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to restart powermetrics: %v\n", err)
						return
					}
				}
				count++
			}

			cpuMetrics, gpuMetrics, thermalPressure, _, timestamp, err = powermetrics.ParsePowermetrics()
			if err != nil || cpuMetrics == nil {
				continue
			}

			if timestamp <= lastTimestamp {
				continue
			}
			lastTimestamp = timestamp

			ramMetrics, err := system.GetRAMMetrics()
			if err != nil {
				continue
			}

			u.Update(cpuMetrics, gpuMetrics, ramMetrics, thermalPressure, cfg.Interval)
			u.Render()
		}
	}
}

func initTermUI() error {
	return termui.Init()
}
