package app

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

func filterProcs(all []ProcInfo, query string) []ProcInfo {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return all
	}
	out := make([]ProcInfo, 0, len(all))
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Name), q) || strings.Contains(fmt.Sprint(p.PID), q) {
			out = append(out, p)
		}
	}
	return out
}

type filterMode int

const (
	filterAll filterMode = iota
	filterPorts
	filterApps
)

func applyFilter(items []ProcInfo, f filterMode) []ProcInfo {
	if f == filterAll {
		return items
	}
	out := make([]ProcInfo, 0, len(items))
	for _, p := range items {
		hasPorts := len(p.Ports) > 0
		if (f == filterPorts) == hasPorts {
			out = append(out, p)
		}
	}
	return out
}

func uniquePorts(procs []ProcInfo) []uint32 {
	seen := map[uint32]bool{}
	var out []uint32
	for _, p := range procs {
		for _, port := range p.Ports {
			if !seen[port] {
				seen[port] = true
				out = append(out, port)
			}
		}
	}
	slices.Sort(out)
	return out
}

func uniqueRuntimes(procs []ProcInfo) []string {
	seen := map[string]bool{}
	for _, p := range procs {
		seen[p.Runtime] = true
	}
	var out []string
	for _, r := range []string{"k8s", "docker", "raw"} {
		if seen[r] {
			out = append(out, r)
		}
	}
	return out
}

type procGroup struct {
	name  string
	procs []ProcInfo
	cpu   float64
	ram   float64
	disk  float64
}

func groupProcesses(items []ProcInfo) []procGroup {
	idx := map[string]int{}
	groups := []procGroup{}
	for _, p := range items {
		if i, ok := idx[p.Name]; ok {
			g := &groups[i]
			g.procs = append(g.procs, p)
			g.cpu += p.CPU
			g.ram += p.RSSMB
			g.disk += p.DiskKBs
		} else {
			idx[p.Name] = len(groups)
			groups = append(groups, procGroup{name: p.Name, procs: []ProcInfo{p}, cpu: p.CPU, ram: p.RSSMB, disk: p.DiskKBs})
		}
	}
	for i := range groups {
		sort.Slice(groups[i].procs, func(a, b int) bool {
			return groups[i].procs[a].CPU > groups[i].procs[b].CPU
		})
	}
	return groups
}

func sortGroups(groups []procGroup, by sortMode, asc bool) {
	sort.Slice(groups, func(i, j int) bool {
		var a, b float64
		switch by {
		case sortRAM:
			a, b = groups[i].ram, groups[j].ram
		case sortDisk:
			a, b = groups[i].disk, groups[j].disk
		default:
			a, b = groups[i].cpu, groups[j].cpu
		}
		if asc {
			return a < b
		}
		return a > b
	})
}

type rowKind int

const (
	rowSingle rowKind = iota
	rowGroup
	rowChild
)

type displayRow struct {
	kind       rowKind
	name       string
	proc       ProcInfo   // rowSingle, rowChild
	groupProcs []ProcInfo // rowGroup: full member list, for kill-all
	count      int
	cpu        float64
	ram        float64
	disk       float64
	ports      []uint32
	runtimes   []string
	last       bool // rowChild: true if this is the last child of its group (tree connector)
}

func flattenRows(groups []procGroup, expanded map[string]bool) []displayRow {
	rows := make([]displayRow, 0, len(groups))
	for _, g := range groups {
		if len(g.procs) == 1 {
			rows = append(rows, displayRow{kind: rowSingle, name: g.name, proc: g.procs[0]})
			continue
		}
		rows = append(rows, displayRow{
			kind: rowGroup, name: g.name, groupProcs: g.procs,
			count: len(g.procs), cpu: g.cpu, ram: g.ram, disk: g.disk,
			ports: uniquePorts(g.procs), runtimes: uniqueRuntimes(g.procs),
		})
		if expanded[g.name] {
			for i, p := range g.procs {
				rows = append(rows, displayRow{kind: rowChild, name: g.name, proc: p, last: i == len(g.procs)-1})
			}
		}
	}
	return rows
}
