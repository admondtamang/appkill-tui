package app

import (
	"fmt"
	"math"
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
	barSlots               = 16
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
	footerLabel = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	selectedRow = lipgloss.NewStyle().Background(colorSelBg).Bold(true)
	statusStyle = lipgloss.NewStyle().Foreground(colorGood).Bold(true)
	deniedStyle = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
	groupStyle  = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	colHeader   = lipgloss.NewStyle().Bold(true).Underline(true)
	mutedGlyph  = lipgloss.NewStyle().Foreground(colorMuted)
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

// padDisplay pads/truncates by terminal column width (not byte/rune count),
// so emoji and wide glyphs don't throw off fixed-width table alignment.
func padDisplay(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		r := []rune(s)
		for len(r) > 0 && lipgloss.Width(string(r)) > width {
			r = r[:len(r)-1]
		}
		return string(r)
	}
	return s + strings.Repeat(" ", width-w)
}

func progressBar(pct float64, warnAt, badAt float64) string {
	filled := min(max(int(math.Round(pct/100*barSlots)), 0), barSlots)
	color := levelColor(pct, warnAt, badAt)
	return lipgloss.NewStyle().Foreground(color).Render(strings.Repeat("|", filled)) +
		mutedGlyph.Render(strings.Repeat("·", barSlots-filled))
}

func pctColored(v, warnAt, badAt float64) string {
	return lipgloss.NewStyle().Foreground(levelColor(v, warnAt, badAt)).Render(fmt.Sprintf("%5.1f%%", v))
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

// renderDashboard draws the bordered header box: CPU/RAM/DISK progress bars
// on the left, Sort/View/Filter state on the right.
func (m model) renderDashboard(width int) string {
	const leftWidth = 34
	rightWidth := max(width-7-leftWidth, 10)
	innerWidth := leftWidth + rightWidth + 3 // " │ " divider

	title := "── appkill "
	top := "╭" + title + strings.Repeat("─", max(innerWidth+2-lipgloss.Width(title), 0)) + "╮"
	bottom := "╰" + strings.Repeat("─", innerWidth+2) + "╯"

	leftLines := [3]string{
		fmt.Sprintf("💻 CPU [%s] %s", progressBar(m.overview.CPUPercent, 50, 85), pctColored(m.overview.CPUPercent, 50, 85)),
		fmt.Sprintf("🧠 RAM [%s] %s", progressBar(m.overview.MemPercent, 60, 90), pctColored(m.overview.MemPercent, 60, 90)),
		fmt.Sprintf("💿 DSK [%s] %s", progressBar(m.overview.DiskPercent, 70, 90), pctColored(m.overview.DiskPercent, 70, 90)),
	}

	labels := []string{"All", "🔌 Ports", "🧩 Apps"}
	var others []string
	for i, l := range labels {
		if filterMode(i) != m.filter {
			others = append(others, l)
		}
	}
	rightLines := [3]string{
		fmt.Sprintf("Sort: %s%s   ⏱ %ds", m.sortBy, map[bool]string{true: "▲", false: "▼"}[m.sortAsc], int(m.refreshInterval.Seconds())),
		fmt.Sprintf("View: [%s]", labels[m.filter]),
		fmt.Sprintf("Filter: %s", strings.Join(others, " │ ")),
	}

	var b strings.Builder
	b.WriteString(top)
	b.WriteString("\n")
	for i := range 3 {
		fmt.Fprintf(&b, "│ %s │ %s │\n", padDisplay(leftLines[i], leftWidth), padDisplay(rightLines[i], rightWidth))
	}
	b.WriteString(bottom)
	return b.String()
}

// renderFooter draws the categorized Nav/Act/Sort keybinding block with a
// right-aligned pagination indicator on the first line.
func (m model) renderFooter(width, start, end, total int) string {
	nav := footerLabel.Render("Nav:") + footerStyle.Render("  ↑/↓ Move │ / Search │ Enter Expand/Collapse │ Tab Filter")
	act := footerLabel.Render("Act:") + footerStyle.Render("  x Kill │ +/- Refresh Rate │ q Quit")
	sortLine := footerLabel.Render("Sort:") + footerStyle.Render(" s Sort │ S Reverse")

	showing := ""
	if total > 0 {
		showing = footerStyle.Render(fmt.Sprintf("Showing: %d-%d of %d", start+1, end, total))
	}
	gap := max(width-lipgloss.Width(nav)-lipgloss.Width(showing), 2)
	nav += strings.Repeat(" ", gap) + showing

	return strings.Join([]string{nav, act, sortLine}, "\n")
}

func (m model) View() string {
	width := m.width
	if width <= 0 {
		width = 100
	}

	var b strings.Builder
	b.WriteString(m.renderDashboard(width))
	b.WriteString("\n")

	spin := ""
	if m.loading {
		spin = " " + m.spinner.View()
	}
	if m.searching || m.search.Value() != "" {
		b.WriteString("  ")
		b.WriteString(m.search.View())
		b.WriteString(spin)
	} else {
		b.WriteString(footerStyle.Render("  / Search..."))
		b.WriteString(spin)
	}
	b.WriteString("\n\n")

	// reserved width: pid(9) icons(6) cpu(7) ram(9) disk(10) ports(22) + spacing(6)
	reserved := 9 + 6 + 7 + 9 + 10 + 22 + 6
	nameWidth := max(width-reserved, 12)

	b.WriteString(colHeader.Render(fmt.Sprintf("  %s %s %s %7s %9s %10s  %s",
		padDisplay("PID", 8), padDisplay("", 5), padDisplay("NAME", nameWidth), "CPU%", "RAM MB", "DISK KB", "PORTS")))
	b.WriteString("\n")
	rule := strings.Repeat("─", width)
	b.WriteString(footerStyle.Render(rule))
	b.WriteString("\n")

	rows := m.rows()
	maxRows := max(m.height-18, 5)
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
		var pidStr, icons, namePart string
		var cpu, ram, disk float64
		var ports []uint32
		switch r.kind {
		case rowGroup:
			arrow := "▸"
			if m.expanded[r.name] {
				arrow = "▾"
			}
			rt := make([]string, len(r.runtimes))
			for i, x := range r.runtimes {
				rt[i] = runtimeEmoji(x)
			}
			pidStr = "---"
			icons = "📦" + strings.Join(rt, "")
			namePart = groupStyle.Render(fmt.Sprintf("%s %s (%d)", arrow, r.name, r.count))
			cpu, ram, disk, ports = r.cpu, r.ram, r.disk, r.ports
		case rowChild:
			p := r.proc
			connector := "├─"
			if r.last {
				connector = "└─"
			}
			pidStr = fmt.Sprint(p.PID)
			icons = stateEmoji(p) + runtimeEmoji(p.Runtime)
			name := connector + " " + p.Name
			if p.Denied {
				name = deniedStyle.Render(name)
			}
			namePart = name
			cpu, ram, disk, ports = p.CPU, p.RSSMB, p.DiskKBs, p.Ports
		default:
			p := r.proc
			pidStr = fmt.Sprint(p.PID)
			icons = stateEmoji(p) + runtimeEmoji(p.Runtime)
			name := p.Name
			if p.Denied {
				name = deniedStyle.Render(name)
			}
			namePart = name
			cpu, ram, disk, ports = p.CPU, p.RSSMB, p.DiskKBs, p.Ports
		}

		cpuStr, ramStr, diskStr := statCells(cpu, ram, disk)
		line := fmt.Sprintf("  %s %s %s %s %s %s  %s",
			padDisplay(pidStr, 8), padDisplay(icons, 5), padDisplay(namePart, nameWidth),
			cpuStr, ramStr, diskStr, formatPorts(ports))

		if i == m.cursor {
			line = selectedRow.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString(footerStyle.Render(rule))
	b.WriteString("\n")
	if m.status != "" && time.Since(m.statusAt) < 4*time.Second {
		b.WriteString(statusStyle.Render(m.status))
		b.WriteString("\n")
	}
	b.WriteString(m.renderFooter(width, start, end, len(rows)))

	return b.String()
}

// Run starts the TUI. Both the appkill and appkill-tui commands call this.
func Run() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
