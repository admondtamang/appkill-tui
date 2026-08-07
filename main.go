package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultRefreshInterval = 10 * time.Second
	minRefreshInterval     = 1 * time.Second
	maxRefreshInterval     = 60 * time.Second
	refreshStep            = 1 * time.Second
)

type sortMode int

const (
	sortCPU sortMode = iota
	sortRAM
	sortDisk
)

func (s sortMode) String() string {
	switch s {
	case sortRAM:
		return "RAM"
	case sortDisk:
		return "DISK"
	default:
		return "CPU"
	}
}

var (
	colorAccent = lipgloss.Color("#7dd3fc") // soft cyan
	colorGood   = lipgloss.Color("#4ade80") // green
	colorWarn   = lipgloss.Color("#fbbf24") // amber
	colorBad    = lipgloss.Color("#f87171") // red
	colorMuted  = lipgloss.Color("#6b7280") // gray
	colorSelBg  = lipgloss.Color("#1e293b")
	headerStyle = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	footerStyle = lipgloss.NewStyle().Foreground(colorMuted)
	selectedRow = lipgloss.NewStyle().Background(colorSelBg).Bold(true)
	statusStyle = lipgloss.NewStyle().Foreground(colorGood).Bold(true)
	deniedStyle = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	groupStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	colHeader   = lipgloss.NewStyle().Bold(true).Underline(true)
)

func levelColor(v, warnAt, badAt float64) lipgloss.Color {
	switch {
	case v >= badAt:
		return colorBad
	case v >= warnAt:
		return colorWarn
	default:
		return colorGood
	}
}

// pieGlyph renders a percentage as a partial-circle "pie" character.
func pieGlyph(pct float64) string {
	switch {
	case pct >= 87.5:
		return "●"
	case pct >= 62.5:
		return "◕"
	case pct >= 37.5:
		return "◑"
	case pct >= 12.5:
		return "◔"
	default:
		return "○"
	}
}

func runtimeEmoji(rt string) string {
	switch rt {
	case "docker":
		return "🐳"
	case "k8s":
		return "☸️"
	default:
		return "💻"
	}
}

func formatPorts(ports []uint32) string {
	if len(ports) == 0 {
		return "-"
	}
	const shown = 3
	parts := make([]string, 0, shown+1)
	for i, p := range ports {
		if i >= shown {
			parts = append(parts, fmt.Sprintf("+%d", len(ports)-shown))
			break
		}
		parts = append(parts, fmt.Sprintf(":%d", p))
	}
	return strings.Join(parts, ",")
}

func statCells(cpu, ram, disk float64) (string, string, string) {
	cpuStr := lipgloss.NewStyle().Foreground(levelColor(cpu, 20, 60)).Render(fmt.Sprintf("%6.1f", cpu))
	ramStr := lipgloss.NewStyle().Foreground(levelColor(ram, 200, 800)).Render(fmt.Sprintf("%8.1f", ram))
	diskStr := fmt.Sprintf("%9.1f", disk)
	return cpuStr, ramStr, diskStr
}

func renderTabs(active filterMode) string {
	labels := []string{"All", "🔌 Ports", "🧩 Apps"}
	parts := make([]string, len(labels))
	for i, l := range labels {
		if filterMode(i) == active {
			parts[i] = groupStyle.Render("[" + l + "]")
		} else {
			parts[i] = footerStyle.Render(l)
		}
	}
	return strings.Join(parts, "  ")
}

func stateEmoji(p ProcInfo) string {
	if p.Denied {
		return "🔒"
	}
	switch p.State {
	case "Z":
		return "🧟"
	case "R":
		return "🚀"
	case "T":
		return "⏸️"
	default:
		return "💤"
	}
}

type tickMsg time.Time
type procsMsg []ProcInfo
type overviewMsg Overview

func tickCmd(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func refreshCmd() tea.Cmd {
	return func() tea.Msg { return procsMsg(listProcesses()) }
}

func overviewCmd() tea.Cmd {
	return func() tea.Msg { return overviewMsg(getOverview()) }
}

type model struct {
	all             []ProcInfo
	overview        Overview
	expanded        map[string]bool
	cursor          int
	sortBy          sortMode
	sortAsc         bool
	filter          filterMode
	refreshInterval time.Duration
	search          textinput.Model
	searching       bool
	spinner         spinner.Model
	loading         bool
	status          string
	statusAt        time.Time
	width           int
	height          int
}

func initialModel() model {
	ti := textinput.New()
	ti.Placeholder = "search by name or pid…"
	ti.Prompt = "🔍 "

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return model{
		search:          ti,
		spinner:         sp,
		sortBy:          sortCPU,
		loading:         true,
		expanded:        map[string]bool{},
		refreshInterval: defaultRefreshInterval,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(refreshCmd(), overviewCmd(), tickCmd(m.refreshInterval), m.spinner.Tick)
}

func (m model) rows() []displayRow {
	items := filterProcs(m.all, m.search.Value())
	items = applyFilter(items, m.filter)
	groups := groupProcesses(items)
	sortGroups(groups, m.sortBy, m.sortAsc)
	return flattenRows(groups, m.expanded)
}

func (m *model) clampCursor() {
	if n := len(m.rows()); m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tickMsg:
		m.loading = true
		return m, tea.Batch(refreshCmd(), overviewCmd(), tickCmd(m.refreshInterval))

	case overviewMsg:
		m.overview = Overview(msg)
		return m, nil

	case procsMsg:
		m.all = msg
		m.loading = false
		m.clampCursor()
		return m, nil

	case tea.KeyMsg:
		if m.searching {
			switch msg.String() {
			case "esc":
				m.searching = false
				m.search.Blur()
			case "enter":
				m.searching = false
				m.search.Blur()
			default:
				var cmd tea.Cmd
				m.search, cmd = m.search.Update(msg)
				m.cursor = 0
				return m, cmd
			}
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "/":
			m.searching = true
			m.search.Focus()
			return m, textinput.Blink
		case "j", "down":
			if rows := m.rows(); m.cursor < len(rows)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "s":
			m.sortBy = (m.sortBy + 1) % 3
		case "S":
			m.sortAsc = !m.sortAsc
		case "tab":
			m.filter = (m.filter + 1) % 3
			m.cursor = 0
		case "+", "=":
			m.refreshInterval = min(m.refreshInterval+refreshStep, maxRefreshInterval)
		case "-", "_":
			m.refreshInterval = max(m.refreshInterval-refreshStep, minRefreshInterval)
		case "enter":
			rows := m.rows()
			if m.cursor < len(rows) && rows[m.cursor].kind == rowGroup {
				name := rows[m.cursor].name
				m.expanded[name] = !m.expanded[name]
				m.clampCursor()
			}
		case "x":
			rows := m.rows()
			if m.cursor < len(rows) {
				r := rows[m.cursor]
				switch r.kind {
				case rowGroup:
					n := 0
					for _, p := range r.groupProcs {
						if syscall.Kill(int(p.PID), syscall.SIGTERM) == nil {
							n++
						}
					}
					m.status = fmt.Sprintf("💀 sent SIGTERM to %d/%d %s processes", n, len(r.groupProcs), r.name)
				default:
					if err := syscall.Kill(int(r.proc.PID), syscall.SIGTERM); err != nil {
						m.status = fmt.Sprintf("⚠️  couldn't kill %s (%d): %v", r.proc.Name, r.proc.PID, err)
					} else {
						m.status = fmt.Sprintf("💀 sent SIGTERM to %s (%d)", r.proc.Name, r.proc.PID)
					}
				}
				m.statusAt = time.Now()
			}
		}
	}
	return m, nil
}

func pad(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}
	return fmt.Sprintf("%-*s", width, s)
}

func (m model) renderOverview() string {
	cpu := pieGlyph(m.overview.CPUPercent)
	ram := pieGlyph(m.overview.MemPercent)
	disk := pieGlyph(m.overview.DiskPercent)
	line := fmt.Sprintf(
		"🖥️  CPU %s %s   🧠 RAM %s %s   💽 DISK %s %s",
		lipgloss.NewStyle().Foreground(levelColor(m.overview.CPUPercent, 50, 85)).Render(cpu),
		fmt.Sprintf("%5.1f%%", m.overview.CPUPercent),
		lipgloss.NewStyle().Foreground(levelColor(m.overview.MemPercent, 60, 90)).Render(ram),
		fmt.Sprintf("%5.1f%%", m.overview.MemPercent),
		lipgloss.NewStyle().Foreground(levelColor(m.overview.DiskPercent, 70, 90)).Render(disk),
		fmt.Sprintf("%5.1f%%", m.overview.DiskPercent),
	)
	return line
}

func (m model) View() string {
	width := m.width
	if width <= 0 {
		width = 100
	}

	var b strings.Builder

	spin := ""
	if m.loading {
		spin = " " + m.spinner.View()
	}
	b.WriteString(headerStyle.Render("⚡ appkill"))
	b.WriteString(spin)
	b.WriteString("  ")
	b.WriteString(footerStyle.Render(fmt.Sprintf("sort: %s%s", m.sortBy, map[bool]string{true: "▲", false: "▼"}[m.sortAsc])))
	b.WriteString("   ")
	b.WriteString(footerStyle.Render(fmt.Sprintf("⏱ %ds", int(m.refreshInterval.Seconds()))))
	b.WriteString("   ")
	b.WriteString(renderTabs(m.filter))
	b.WriteString("\n")
	b.WriteString(m.renderOverview())
	b.WriteString("\n\n")

	if m.searching || m.search.Value() != "" {
		b.WriteString(m.search.View())
	} else {
		b.WriteString(footerStyle.Render("press / to search"))
	}
	b.WriteString("\n\n")

	// reserved width: state emoji(2) runtime emoji(2) pid(9) cpu(8) ram(10) disk(11) ports(14) + spacing(7)
	reserved := 2 + 2 + 9 + 8 + 10 + 11 + 14 + 7
	nameWidth := max(width-reserved, 12)

	b.WriteString(colHeader.Render(fmt.Sprintf("      %-8s %s %7s %9s %10s  %s", "PID", pad("NAME", nameWidth), "CPU%", "RAM MB", "DISK KB", "PORTS")))
	b.WriteString("\n")

	rows := m.rows()
	maxRows := 20
	if m.height > 14 {
		maxRows = m.height - 14
	}
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}
	if start > 0 && start+maxRows > len(rows) {
		start = max(0, len(rows)-maxRows)
	}
	end := min(start+maxRows, len(rows))

	for i := start; i < end; i++ {
		r := rows[i]
		var line string
		switch r.kind {
		case rowGroup:
			arrow := "▸"
			if m.expanded[r.name] {
				arrow = "▾"
			}
			label := pad(fmt.Sprintf("%s %s ×%d", arrow, r.name, r.count), nameWidth)
			cpuStr, ramStr, diskStr := statCells(r.cpu, r.ram, r.disk)
			rt := make([]string, len(r.runtimes))
			for i, x := range r.runtimes {
				rt[i] = runtimeEmoji(x)
			}
			line = fmt.Sprintf("📦 %-2s %-8s %s %s %s %s  %s", strings.Join(rt, ""), "", groupStyle.Render(label), cpuStr, ramStr, diskStr, formatPorts(r.ports))
		case rowChild:
			p := r.proc
			cpuStr, ramStr, diskStr := statCells(p.CPU, p.RSSMB, p.DiskKBs)
			label := pad("  ↳", nameWidth)
			line = fmt.Sprintf("%s %-2s %-8d %s %s %s %s  %s", stateEmoji(p), runtimeEmoji(p.Runtime), p.PID, label, cpuStr, ramStr, diskStr, formatPorts(p.Ports))
		default:
			p := r.proc
			cpuStr, ramStr, diskStr := statCells(p.CPU, p.RSSMB, p.DiskKBs)
			name := p.Name
			if p.Denied {
				name = deniedStyle.Render(pad(name, nameWidth))
			} else {
				name = pad(name, nameWidth)
			}
			line = fmt.Sprintf("%s %-2s %-8d %s %s %s %s  %s", stateEmoji(p), runtimeEmoji(p.Runtime), p.PID, name, cpuStr, ramStr, diskStr, formatPorts(p.Ports))
		}
		if i == m.cursor {
			line = selectedRow.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.status != "" && time.Since(m.statusAt) < 4*time.Second {
		b.WriteString(statusStyle.Render(m.status))
		b.WriteString("\n")
	}
	scrollInfo := ""
	if len(rows) > maxRows {
		scrollInfo = fmt.Sprintf("  (%d-%d of %d)", start+1, end, len(rows))
	}
	b.WriteString(footerStyle.Render("↑/↓ move · / search · s sort · S reverse · tab filter · +/- refresh rate · enter expand/collapse · x kill · q quit" + scrollInfo))

	return b.String()
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
