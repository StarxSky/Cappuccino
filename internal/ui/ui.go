package ui

import (
	"fmt"
	"image"
	"strconv"
	"strings"
	"time"

	"github.com/gizak/termui/v3"
	"github.com/gizak/termui/v3/widgets"
	"github.com/starxsky/Cappuccino/internal/config"
	"github.com/starxsky/Cappuccino/internal/metrics"
)

const helpText = `Cappuccino - Help

Keys
  q / Ctrl-C     Quit
  h / ?          Toggle this help
  :              Command mode (REPL)
  + / -          Sampling interval up / down
  c              Cycle bar color
  Esc            Close help / cancel command

Commands (press ':' then type, Enter to run)
  help                    Show this help
  quit | exit             Quit
  interval <seconds>      Sampling / display interval
  avg <seconds>           Averaging window
  color <0-8>             Bar color
  cores <on|off>          Toggle per-core display
  maxcount <n>            Restart powermetrics every n samples (0=off)
  clear                   Clear status message

Press Esc or 'h' to close.`

type GridItem = termui.GridItem

var _ GridItem

type UI struct {
	grid        *termui.Grid
	showCores   bool
	color       int
	socInfo     *metrics.SoCInfo
	cpuMaxPower float64
	gpuMaxPower float64
	aneMaxPower float64

	cfg    *config.Config
	status *statusBar
	help   *widgets.Paragraph

	helpVisible bool
	cmdMode     bool
	cmdInput    string
	statusMsg   string
	statusAt    time.Time

	cpu1Gauge      *widgets.Gauge
	cpu2Gauge      *widgets.Gauge
	gpuGauge       *widgets.Gauge
	aneGauge       *widgets.Gauge
	ramGauge       *widgets.Gauge
	eCoreGauges    []*widgets.Gauge
	pCoreGauges    []*widgets.Gauge
	pCoreGaugesExt []*widgets.Gauge

	cpuPowerChart *widgets.Plot
	gpuPowerChart *widgets.Plot

	avgPackagePowerList []float64
	avgCPUPowerList     []float64
	avgGPUPowerList     []float64
	avgLen              int

	cpuPeakPower     float64
	gpuPeakPower     float64
	packagePeakPower float64
}

func NewUI(cfg *config.Config, socInfo *metrics.SoCInfo) *UI {
	ui := &UI{
		showCores:   cfg.ShowCores,
		color:       cfg.Color,
		socInfo:     socInfo,
		cpuMaxPower: socInfo.CPUMaxPower,
		gpuMaxPower: socInfo.GPUMaxPower,
		aneMaxPower: 8.0,
		cfg:         cfg,
		avgLen:      cfg.AvgSeconds / cfg.Interval,
	}

	ui.status = newStatusBar()
	ui.help = widgets.NewParagraph()
	ui.help.Title = " Help "
	ui.help.Text = helpText

	ui.initGauges()
	ui.initCharts()
	ui.buildLayout()
	return ui
}

func (u *UI) initGauges() {
	u.cpu1Gauge = widgets.NewGauge()
	u.cpu1Gauge.Title = "E-CPU Usage"
	u.cpu1Gauge.Percent = 0
	u.cpu1Gauge.BarColor = termui.Color(u.color)

	u.cpu2Gauge = widgets.NewGauge()
	u.cpu2Gauge.Title = "P-CPU Usage"
	u.cpu2Gauge.Percent = 0
	u.cpu2Gauge.BarColor = termui.Color(u.color)

	u.gpuGauge = widgets.NewGauge()
	u.gpuGauge.Title = "GPU Usage"
	u.gpuGauge.Percent = 0
	u.gpuGauge.BarColor = termui.Color(u.color)

	u.aneGauge = widgets.NewGauge()
	u.aneGauge.Title = "ANE"
	u.aneGauge.Percent = 0
	u.aneGauge.BarColor = termui.Color(u.color)

	u.ramGauge = widgets.NewGauge()
	u.ramGauge.Title = "RAM Usage"
	u.ramGauge.Percent = 0
	u.ramGauge.BarColor = termui.Color(u.color)

	eCoreCount := u.socInfo.ECoreCount
	u.eCoreGauges = make([]*widgets.Gauge, eCoreCount)
	for i := 0; i < eCoreCount; i++ {
		g := widgets.NewGauge()
		g.Title = fmt.Sprintf("Core-%d", i+1)
		g.Percent = 0
		g.BarColor = termui.Color(u.color)
		u.eCoreGauges[i] = g
	}

	pCoreCount := u.socInfo.PCoreCount
	displayPCount := pCoreCount
	if displayPCount > 8 {
		displayPCount = 8
	}
	u.pCoreGauges = make([]*widgets.Gauge, displayPCount)
	for i := 0; i < displayPCount; i++ {
		g := widgets.NewGauge()
		g.Title = fmt.Sprintf("Core-%d", i+1)
		g.Percent = 0
		g.BarColor = termui.Color(u.color)
		u.pCoreGauges[i] = g
	}

	if pCoreCount > 8 {
		u.pCoreGaugesExt = make([]*widgets.Gauge, pCoreCount-8)
		for i := 0; i < pCoreCount-8; i++ {
			g := widgets.NewGauge()
			g.Title = fmt.Sprintf("Core-%d", i+9)
			g.Percent = 0
			g.BarColor = termui.Color(u.color)
			u.pCoreGaugesExt[i] = g
		}
	}
}

func (u *UI) initCharts() {
	u.cpuPowerChart = widgets.NewPlot()
	u.cpuPowerChart.Title = "CPU Power"
	// Plot renders lines between points, so it needs at least two samples.
	// Seed with zeros to avoid an index-out-of-range panic on the first frame.
	u.cpuPowerChart.Data = [][]float64{{0, 0}}
	u.cpuPowerChart.MaxVal = 100
	u.cpuPowerChart.AxesColor = termui.ColorWhite
	u.cpuPowerChart.LineColors[0] = termui.Color(u.color)
	u.cpuPowerChart.Marker = widgets.MarkerBraille

	u.gpuPowerChart = widgets.NewPlot()
	u.gpuPowerChart.Title = "GPU Power"
	u.gpuPowerChart.Data = [][]float64{{0, 0}}
	u.gpuPowerChart.MaxVal = 100
	u.gpuPowerChart.AxesColor = termui.ColorWhite
	u.gpuPowerChart.LineColors[0] = termui.Color(u.color)
	u.gpuPowerChart.Marker = widgets.MarkerBraille
}

func (u *UI) buildLayout() {
	u.grid = termui.NewGrid()

	if u.showCores {
		u.buildCoresLayout()
	} else {
		u.buildSimpleLayout()
	}
}

func (u *UI) buildSimpleLayout() {
	cpuGauges := termui.NewGrid()
	cpuGauges.SetRect(0, 0, 100, 50)
	cpuGauges.Set(
		termui.NewRow(0.5,
			termui.NewCol(0.5, u.cpu1Gauge),
			termui.NewCol(0.5, u.cpu2Gauge),
		),
		termui.NewRow(0.5,
			termui.NewCol(0.5, u.gpuGauge),
			termui.NewCol(0.5, u.aneGauge),
		),
	)

	memory := termui.NewGrid()
	memory.SetRect(0, 0, 100, 50)
	memory.Set(
		termui.NewRow(1.0, termui.NewCol(1.0, u.ramGauge)),
	)

	power := termui.NewGrid()
	power.SetRect(0, 0, 100, 50)
	power.Set(
		termui.NewRow(0.5,
			termui.NewCol(0.5, u.cpuPowerChart),
			termui.NewCol(0.5, u.gpuPowerChart),
		),
	)

	u.grid.SetRect(0, 0, 100, 100)
	u.grid.Set(
		termui.NewRow(0.5,
			termui.NewCol(1.0, cpuGauges),
		),
		termui.NewRow(0.25,
			termui.NewCol(1.0, memory),
		),
		termui.NewRow(0.25,
			termui.NewCol(1.0, power),
		),
	)
}

func (u *UI) buildCoresLayout() {
	eCoreCols := make([]interface{}, len(u.eCoreGauges))
	for i, g := range u.eCoreGauges {
		eCoreCols[i] = termui.NewCol(1.0/float64(len(u.eCoreGauges)), g)
	}

	pCoreCols := make([]interface{}, len(u.pCoreGauges))
	for i, g := range u.pCoreGauges {
		pCoreCols[i] = termui.NewCol(1.0/float64(len(u.pCoreGauges)), g)
	}

	var pCoreExtCols []interface{}
	if len(u.pCoreGaugesExt) > 0 {
		pCoreExtCols = make([]interface{}, len(u.pCoreGaugesExt))
		for i, g := range u.pCoreGaugesExt {
			pCoreExtCols[i] = termui.NewCol(1.0/float64(len(u.pCoreGaugesExt)), g)
		}
	}

	processorGrid := termui.NewGrid()
	processorRows := []interface{}{
		termui.NewRow(0.15, termui.NewCol(1.0, u.cpu1Gauge)),
		termui.NewRow(0.2, termui.NewRow(1.0, eCoreCols...)),
		termui.NewRow(0.15, termui.NewCol(1.0, u.cpu2Gauge)),
	}

	if len(pCoreCols) > 0 {
		processorRows = append(processorRows, termui.NewRow(0.2, termui.NewRow(1.0, pCoreCols...)))
	}
	if len(pCoreExtCols) > 0 {
		processorRows = append(processorRows, termui.NewRow(0.15, termui.NewRow(1.0, pCoreExtCols...)))
	}

	processorRows = append(processorRows,
		termui.NewRow(0.15, termui.NewCol(1.0, u.gpuGauge)),
		termui.NewRow(0.15, termui.NewCol(1.0, u.aneGauge)),
	)
	processorGrid.Set(processorRows...)

	memoryGrid := termui.NewGrid()
	memoryGrid.Set(termui.NewRow(1.0, termui.NewCol(1.0, u.ramGauge)))

	powerGrid := termui.NewGrid()
	powerGrid.Set(
		termui.NewRow(0.5, termui.NewCol(1.0, u.cpuPowerChart)),
		termui.NewRow(0.5, termui.NewCol(1.0, u.gpuPowerChart)),
	)

	u.grid.SetRect(0, 0, 100, 100)
	u.grid.Set(
		termui.NewRow(0.6,
			termui.NewCol(0.6, processorGrid),
			termui.NewCol(0.4,
				termui.NewRow(0.5, termui.NewCol(1.0, memoryGrid)),
				termui.NewRow(0.5, termui.NewCol(1.0, powerGrid)),
			),
		),
	)
}

func (u *UI) Update(cpu *metrics.CPUMetrics, gpu *metrics.GPUMetrics, ram *metrics.RAMMetrics, thermalPressure metrics.ThermalPressure, interval int) {
	u.updateCPUGauges(cpu)
	u.updateGPUGauge(gpu)
	u.updateANEGauge(cpu, interval)
	u.updateRAMGauge(ram)
	u.updatePowerCharts(cpu, thermalPressure, interval)
}

func (u *UI) updateCPUGauges(cpu *metrics.CPUMetrics) {
	u.cpu1Gauge.Title = fmt.Sprintf("E-CPU Usage: %d%% @ %d MHz", cpu.EClusterActive, cpu.EClusterFreqMHz)
	u.cpu1Gauge.Percent = cpu.EClusterActive

	u.cpu2Gauge.Title = fmt.Sprintf("P-CPU Usage: %d%% @ %d MHz", cpu.PClusterActive, cpu.PClusterFreqMHz)
	u.cpu2Gauge.Percent = cpu.PClusterActive

	if u.showCores {
		for core, active := range cpu.ECoreActive {
			if core < len(u.eCoreGauges) {
				u.eCoreGauges[core].Title = fmt.Sprintf("Core-%d %d%%", core+1, active)
				u.eCoreGauges[core].Percent = active
			}
		}
		for core, active := range cpu.PCoreActive {
			if core < len(u.pCoreGauges) {
				u.pCoreGauges[core].Title = fmt.Sprintf("Core-%d %d%%", core+1, active)
				u.pCoreGauges[core].Percent = active
			} else if len(u.pCoreGaugesExt) > 0 && core-8 < len(u.pCoreGaugesExt) {
				u.pCoreGaugesExt[core-8].Title = fmt.Sprintf("Core-%d %d%%", core+1, active)
				u.pCoreGaugesExt[core-8].Percent = active
			}
		}
	}
}

func (u *UI) updateGPUGauge(gpu *metrics.GPUMetrics) {
	u.gpuGauge.Title = fmt.Sprintf("GPU Usage: %d%% @ %d MHz", gpu.Active, gpu.FreqMHz)
	u.gpuGauge.Percent = gpu.Active
}

func (u *UI) updateANEGauge(cpu *metrics.CPUMetrics, interval int) {
	aneUtilPercent := int(cpu.ANEWatts / float64(interval) / u.aneMaxPower * 100)
	u.aneGauge.Title = fmt.Sprintf("ANE Usage: %d%% @ %.1f W", aneUtilPercent, cpu.ANEWatts/float64(interval))
	u.aneGauge.Percent = aneUtilPercent
}

func (u *UI) updateRAMGauge(ram *metrics.RAMMetrics) {
	if ram.SwapTotalGB < 0.1 {
		u.ramGauge.Title = fmt.Sprintf("RAM Usage: %.1f/%.1fGB - swap inactive", ram.UsedGB, ram.TotalGB)
	} else {
		u.ramGauge.Title = fmt.Sprintf("RAM Usage: %.1f/%.1fGB - swap: %.1f/%.1fGB",
			ram.UsedGB, ram.TotalGB, ram.SwapUsedGB, ram.SwapTotalGB)
	}
	u.ramGauge.Percent = 100 - ram.FreePercent
}

func (u *UI) updatePowerCharts(cpu *metrics.CPUMetrics, thermalPressure metrics.ThermalPressure, interval int) {
	packagePower := cpu.PackageWatts / float64(interval)
	if packagePower > u.packagePeakPower {
		u.packagePeakPower = packagePower
	}
	u.avgPackagePowerList = append(u.avgPackagePowerList, packagePower)
	if len(u.avgPackagePowerList) > u.avgLen {
		u.avgPackagePowerList = u.avgPackagePowerList[1:]
	}
	avgPackagePower := average(u.avgPackagePowerList)

	thermalThrottle := "no"
	if thermalPressure != metrics.ThermalNominal {
		thermalThrottle = "yes"
	}

	u.gpuPowerChart.Title = fmt.Sprintf("CPU+GPU+ANE Power: %.2fW (avg: %.2fW peak: %.2fW) throttle: %s",
		packagePower, avgPackagePower, u.packagePeakPower, thermalThrottle)

	cpuPower := cpu.CPUWatts / float64(interval)
	if cpuPower > u.cpuPeakPower {
		u.cpuPeakPower = cpuPower
	}
	u.avgCPUPowerList = append(u.avgCPUPowerList, cpuPower)
	if len(u.avgCPUPowerList) > u.avgLen {
		u.avgCPUPowerList = u.avgCPUPowerList[1:]
	}
	avgCPUPower := average(u.avgCPUPowerList)

	cpuPowerPercent := int(cpu.CPUWatts / float64(interval) / u.cpuMaxPower * 100)
	u.cpuPowerChart.Title = fmt.Sprintf("CPU: %.2fW (avg: %.2fW peak: %.2fW)", cpuPower, avgCPUPower, u.cpuPeakPower)
	u.cpuPowerChart.Data[0] = append(u.cpuPowerChart.Data[0], float64(cpuPowerPercent))
	if len(u.cpuPowerChart.Data[0]) > 60 {
		u.cpuPowerChart.Data[0] = u.cpuPowerChart.Data[0][1:]
	}

	gpuPower := cpu.GPUWatts / float64(interval)
	if gpuPower > u.gpuPeakPower {
		u.gpuPeakPower = gpuPower
	}
	u.avgGPUPowerList = append(u.avgGPUPowerList, gpuPower)
	if len(u.avgGPUPowerList) > u.avgLen {
		u.avgGPUPowerList = u.avgGPUPowerList[1:]
	}
	avgGPUPower := average(u.avgGPUPowerList)

	gpuPowerPercent := int(cpu.GPUWatts / float64(interval) / u.gpuMaxPower * 100)
	u.gpuPowerChart.Title = fmt.Sprintf("GPU: %.2fW (avg: %.2fW peak: %.2fW)", gpuPower, avgGPUPower, u.gpuPeakPower)
	u.gpuPowerChart.Data[0] = append(u.gpuPowerChart.Data[0], float64(gpuPowerPercent))
	if len(u.gpuPowerChart.Data[0]) > 60 {
		u.gpuPowerChart.Data[0] = u.gpuPowerChart.Data[0][1:]
	}
}

func (u *UI) Render() {
	termWidth, termHeight := termui.TerminalDimensions()
	if termWidth <= 0 || termHeight <= 0 {
		return
	}

	barHeight := 1
	if termHeight < barHeight+1 {
		barHeight = 0
	}

	u.grid.SetRect(0, 0, termWidth, termHeight-barHeight)

	items := []termui.Drawable{u.grid}
	if barHeight > 0 {
		u.status.Text = u.statusText()
		u.status.SetRect(0, termHeight-barHeight, termWidth, termHeight)
		items = append(items, u.status)
	}

	if u.helpVisible {
		helpWidth, helpHeight := u.helpSize(termWidth, termHeight)
		x0 := (termWidth - helpWidth) / 2
		y0 := (termHeight - helpHeight) / 2
		u.help.SetRect(x0, y0, x0+helpWidth, y0+helpHeight)
		items = append(items, u.help)
	}

	termui.Render(items...)
}

func (u *UI) helpSize(termWidth, termHeight int) (int, int) {
	lines := len(strings.Split(helpText, "\n"))

	width := 60
	if width > termWidth-4 {
		width = termWidth - 4
	}
	if width < 16 {
		width = termWidth
	}

	height := lines + 2
	if height > termHeight-2 {
		height = termHeight - 2
	}
	return width, height
}

func (u *UI) Close() {
	termui.Close()
}

// KeyResult carries the actions that must be handled outside the UI.
type KeyResult struct {
	Quit     bool
	Interval int
}

// HandleKey processes a termui keyboard event ID (e.g. "q", ":", "<Enter>").
func (u *UI) HandleKey(id string) KeyResult {
	if u.cmdMode {
		switch id {
		case "<Enter>":
			input := strings.TrimSpace(u.cmdInput)
			u.cmdMode = false
			u.cmdInput = ""
			return u.executeCommand(input)
		case "<Escape>", "<C-c>":
			u.cmdMode = false
			u.cmdInput = ""
			u.setStatus("command cancelled")
		case "<Backspace>", "<C-<Backspace>>":
			runes := []rune(u.cmdInput)
			if len(runes) > 0 {
				u.cmdInput = string(runes[:len(runes)-1])
			}
		case "<Space>":
			u.cmdInput += " "
		default:
			if len([]rune(id)) == 1 {
				u.cmdInput += id
			}
		}
		return KeyResult{}
	}

	switch id {
	case "q", "<C-c>":
		return KeyResult{Quit: true}
	case "h", "?":
		u.helpVisible = !u.helpVisible
	case ":":
		u.cmdMode = true
		u.cmdInput = ""
		u.helpVisible = false
	case "<Escape>":
		u.helpVisible = false
	case "c":
		u.cycleColor()
	case "+", "=":
		return u.changeIntervalTo(u.cfg.Interval + 1)
	case "-", "_":
		return u.changeIntervalTo(u.cfg.Interval - 1)
	}
	return KeyResult{}
}

func (u *UI) executeCommand(input string) KeyResult {
	if input == "" {
		return KeyResult{}
	}

	fields := strings.Fields(input)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "help", "?":
		u.helpVisible = true
	case "quit", "exit", "q":
		return KeyResult{Quit: true}
	case "interval":
		if n, ok := parseNumber(args, 1); ok {
			return u.changeIntervalTo(n)
		}
		u.setStatus("usage: interval <seconds>")
	case "avg":
		if n, ok := parseNumber(args, 1); ok {
			u.cfg.AvgSeconds = n
			u.RecalcAvg()
			u.setStatus(fmt.Sprintf("avg = %ds", n))
		} else {
			u.setStatus("usage: avg <seconds>")
		}
	case "color":
		if n, ok := parseNumber(args, 0); ok && n <= 8 {
			u.cfg.Color = n
			u.applyColor(n)
			u.setStatus(fmt.Sprintf("color = %d", n))
		} else {
			u.setStatus("usage: color <0-8>")
		}
	case "cores":
		u.setCores(args)
	case "maxcount":
		if n, ok := parseNumber(args, 0); ok {
			u.cfg.MaxCount = n
			u.setStatus(fmt.Sprintf("maxcount = %d", n))
		} else {
			u.setStatus("usage: maxcount <n>")
		}
	case "clear":
		u.statusMsg = ""
	default:
		u.setStatus("unknown command: " + cmd)
	}
	return KeyResult{}
}

func (u *UI) changeIntervalTo(n int) KeyResult {
	if n < 1 {
		n = 1
	}
	if n > 60 {
		n = 60
	}
	if n == u.cfg.Interval {
		u.setStatus(fmt.Sprintf("interval already %ds", n))
		return KeyResult{}
	}
	u.setStatus(fmt.Sprintf("interval = %ds (restarting powermetrics)", n))
	return KeyResult{Interval: n}
}

func (u *UI) setCores(args []string) {
	if len(args) == 0 {
		u.showCores = !u.showCores
	} else {
		switch strings.ToLower(args[0]) {
		case "on", "true", "1":
			u.showCores = true
		case "off", "false", "0":
			u.showCores = false
		default:
			u.setStatus("usage: cores <on|off>")
			return
		}
	}
	u.cfg.ShowCores = u.showCores
	u.buildLayout()
	u.setStatus(fmt.Sprintf("cores = %s", onOff(u.showCores)))
}

func (u *UI) RecalcAvg() {
	if u.cfg.Interval > 0 {
		u.avgLen = u.cfg.AvgSeconds / u.cfg.Interval
	}
	if u.avgLen < 1 {
		u.avgLen = 1
	}
}

func (u *UI) cycleColor() {
	n := u.color + 1
	if n > 8 {
		n = 1
	}
	u.cfg.Color = n
	u.applyColor(n)
	u.setStatus(fmt.Sprintf("color = %d", n))
}

func (u *UI) applyColor(n int) {
	u.color = n
	color := termui.Color(n)

	gauges := []*widgets.Gauge{u.cpu1Gauge, u.cpu2Gauge, u.gpuGauge, u.aneGauge, u.ramGauge}
	gauges = append(gauges, u.eCoreGauges...)
	gauges = append(gauges, u.pCoreGauges...)
	gauges = append(gauges, u.pCoreGaugesExt...)
	for _, g := range gauges {
		if g != nil {
			g.BarColor = color
		}
	}
	u.cpuPowerChart.LineColors[0] = color
	u.gpuPowerChart.LineColors[0] = color
}

func (u *UI) setStatus(msg string) {
	u.statusMsg = msg
	u.statusAt = time.Now()
}

func (u *UI) statusText() string {
	if u.cmdMode {
		return ": " + u.cmdInput + "\u2588"
	}
	if u.statusMsg != "" && time.Since(u.statusAt) < 3*time.Second {
		return " " + u.statusMsg
	}
	return u.hintText()
}

func (u *UI) hintText() string {
	return fmt.Sprintf(
		" q: quit   ?: help   :: command   +/-: interval   c: color    [ interval %ds | avg %ds | cores %s | maxcount %d ]",
		u.cfg.Interval, u.cfg.AvgSeconds, onOff(u.showCores), u.cfg.MaxCount,
	)
}

func parseNumber(args []string, min int) (int, bool) {
	if len(args) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(args[0])
	if err != nil || n < min {
		return 0, false
	}
	return n, true
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// statusBar is a single-line bar pinned to the bottom of the screen.
type statusBar struct {
	termui.Block
	Text string
}

func newStatusBar() *statusBar {
	return &statusBar{Block: *termui.NewBlock()}
}

func (s *statusBar) Draw(buf *termui.Buffer) {
	style := termui.NewStyle(termui.ColorBlack, termui.ColorWhite, termui.ModifierBold)

	for x := s.Min.X; x < s.Max.X; x++ {
		buf.SetCell(termui.NewCell(' ', style), image.Pt(x, s.Min.Y))
	}

	runes := []rune(s.Text)
	if len(runes) > s.Dx() {
		runes = runes[:s.Dx()]
	}
	x := s.Min.X
	for _, r := range runes {
		buf.SetCell(termui.NewCell(r, style), image.Pt(x, s.Min.Y))
		x++
	}
}

func average(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
