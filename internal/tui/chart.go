package tui

import (
	"fmt"
	"strings"

	"github.com/bfarka/opendraft/internal/control"
)

var blockLevels = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// renderHRHistoryChart renders a multi-row Unicode bar chart of the rolling HR history
// with a horizontal reference line at effBPM (the configured percentile).
func renderHRHistoryChart(history []uint8, liveBPM, effBPM uint8, windowSec, percentile int, minHR, maxHR uint8, width, height int) string {
	if width < 10 {
		width = 10
	}
	if height < 3 {
		height = 3
	}

	// Determine Y-axis bounds from curve range and observed history.
	yMin := int(minHR)
	if yMin == 0 || yMin > 90 {
		yMin = 90
	} else {
		yMin -= 10
		if yMin < 40 {
			yMin = 40
		}
	}
	yMax := int(maxHR) + 10
	if yMax <= yMin+10 {
		yMax = yMin + 40
	}
	for _, b := range history {
		if b == 0 {
			continue
		}
		if int(b) < yMin {
			yMin = int(b)
		}
		if int(b) > yMax {
			yMax = int(b)
		}
	}
	if effBPM > 0 && int(effBPM) > yMax {
		yMax = int(effBPM)
	}
	if liveBPM > 0 && int(liveBPM) > yMax {
		yMax = int(liveBPM)
	}

	// Bucket history (oldest -> newest) into `width` columns.
	cols := make([]uint8, width)
	if len(history) > 0 {
		for x := 0; x < width; x++ {
			start := (x * len(history)) / width
			end := ((x + 1) * len(history)) / width
			if end <= start {
				end = start + 1
			}
			if end > len(history) {
				end = len(history)
			}
			var maxVal uint8
			for i := start; i < end; i++ {
				if history[i] > maxVal {
					maxVal = history[i]
				}
			}
			cols[x] = maxVal
		}
	}

	// Compute row index (0 = top, height-1 = bottom) for the P80 reference line.
	pRow := -1
	if effBPM > 0 && yMax > yMin {
		norm := float64(int(effBPM)-yMin) / float64(yMax-yMin)
		if norm < 0 {
			norm = 0
		} else if norm > 1 {
			norm = 1
		}
		pRow = height - 1 - int(norm*float64(height-1)+0.5)
	}

	totalSteps := height * 8
	var lines []string
	header := fmt.Sprintf("HR History (%ds) • Live: -- • P%d: --", windowSec, percentile)
	if liveBPM > 0 || effBPM > 0 {
		header = fmt.Sprintf("HR History (%ds) • Live: %d bpm • P%d: %d bpm", windowSec, liveBPM, percentile, effBPM)
	}
	lines = append(lines, header)

	for row := 0; row < height; row++ {
		var label string
		switch row {
		case 0:
			label = fmt.Sprintf("%3d ┤", yMax)
		case height - 1:
			label = fmt.Sprintf("%3d ┤", yMin)
		case pRow:
			label = fmt.Sprintf("P%02d ┤", percentile)
		default:
			label = "    │"
		}

		var rowBuf strings.Builder
		rowBuf.WriteString(label)
		rowBottom := (height - 1 - row) * 8

		for x := 0; x < width; x++ {
			v := cols[x]
			if v == 0 {
				if row == pRow {
					rowBuf.WriteRune('┄')
				} else {
					rowBuf.WriteRune(' ')
				}
				continue
			}

			norm := float64(int(v)-yMin) / float64(yMax-yMin)
			if norm < 0 {
				norm = 0
			} else if norm > 1 {
				norm = 1
			}
			barSteps := int(norm*float64(totalSteps) + 0.5)
			if barSteps < 1 {
				barSteps = 1
			}
			fill := barSteps - rowBottom
			if fill <= 0 {
				if row == pRow {
					rowBuf.WriteRune('┄')
				} else {
					rowBuf.WriteRune(' ')
				}
			} else if fill >= 8 {
				rowBuf.WriteRune('█')
			} else {
				rowBuf.WriteRune(blockLevels[fill])
			}
		}
		lines = append(lines, rowBuf.String())
	}

	axis := fmt.Sprintf("    └%s", strings.Repeat("─", width))
	lines = append(lines, axis)
	lines = append(lines, fmt.Sprintf("    -%ds%snow", windowSec, strings.Repeat(" ", maxInt(1, width-len(fmt.Sprintf("-%ds", windowSec))-3))))
	return strings.Join(lines, "\n")
}

// renderCurvePlot renders a compact 2D HR-to-Fan curve plot with live (●) and percentile (◆) markers.
func renderCurvePlot(mapper control.LinearMapper, liveBPM, effBPM uint8, percentile int, width, height int) string {
	if width < 16 {
		width = 16
	}
	if height < 4 {
		height = 4
	}

	xMin := int(mapper.MinHR) - 15
	if xMin < 40 {
		xMin = 40
	}
	xMax := int(mapper.MaxHR) + 15
	if xMax <= xMin+10 {
		xMax = xMin + 40
	}
	if liveBPM > 0 {
		if int(liveBPM) < xMin {
			xMin = int(liveBPM)
		}
		if int(liveBPM) > xMax {
			xMax = int(liveBPM)
		}
	}
	if effBPM > 0 {
		if int(effBPM) < xMin {
			xMin = int(effBPM)
		}
		if int(effBPM) > xMax {
			xMax = int(effBPM)
		}
	}

	grid := make([][]rune, height)
	for r := 0; r < height; r++ {
		grid[r] = make([]rune, width)
		for c := 0; c < width; c++ {
			grid[r][c] = ' '
		}
	}

	bpmToCol := func(bpm uint8) int {
		if xMax == xMin {
			return 0
		}
		c := int(float64(int(bpm)-xMin)/float64(xMax-xMin)*float64(width-1) + 0.5)
		if c < 0 {
			return 0
		}
		if c >= width {
			return width - 1
		}
		return c
	}
	speedToRow := func(speed int) int {
		if speed < 0 {
			speed = 0
		} else if speed > 100 {
			speed = 100
		}
		r := height - 1 - int(float64(speed)/100.0*float64(height-1)+0.5)
		if r < 0 {
			return 0
		}
		if r >= height {
			return height - 1
		}
		return r
	}

	// Plot curve points.
	for c := 0; c < width; c++ {
		bpm := uint8(xMin + int(float64(c)/float64(width-1)*float64(xMax-xMin)+0.5))
		spd := mapper.Map(bpm)
		r := speedToRow(spd)
		if bpm <= mapper.MinHR || bpm >= mapper.MaxHR {
			grid[r][c] = '─'
		} else {
			grid[r][c] = '·'
		}
	}

	// Overlay live HR marker (●) and percentile HR marker (◆).
	if liveBPM > 0 {
		c := bpmToCol(liveBPM)
		r := speedToRow(mapper.Map(liveBPM))
		grid[r][c] = '●'
	}
	if effBPM > 0 {
		c := bpmToCol(effBPM)
		r := speedToRow(mapper.Map(effBPM))
		grid[r][c] = '◆'
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("Fan Curve (%d-%d bpm → %d-%d%%)", mapper.MinHR, mapper.MaxHR, mapper.MinSpeed, mapper.MaxSpeed))
	for r := 0; r < height; r++ {
		var label string
		switch r {
		case 0:
			label = "100% ┤"
		case height - 1:
			label = "  0% ┤"
		default:
			label = "     │"
		}
		lines = append(lines, label+string(grid[r]))
	}
	lines = append(lines, "     └"+strings.Repeat("─", width))
	leftLbl := fmt.Sprintf("%d bpm", xMin)
	rightLbl := fmt.Sprintf("%d bpm", xMax)
	gap := maxInt(1, width-len(leftLbl)-len(rightLbl))
	lines = append(lines, "      "+leftLbl+strings.Repeat(" ", gap)+rightLbl)
	if effBPM > 0 {
		lines = append(lines, fmt.Sprintf("      ● Live: %d bpm   ◆ P%d: %d bpm (%d%%)", liveBPM, percentile, effBPM, mapper.Map(effBPM)))
	} else {
		lines = append(lines, fmt.Sprintf("      ● Live: --       ◆ P%d: --", percentile))
	}
	return strings.Join(lines, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
