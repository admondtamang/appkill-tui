package app

import "github.com/shirou/gopsutil/v3/process"

type ProcInfo struct {
	PID     int32
	Name    string
	CPU     float64
	RSSMB   float64
	DiskKBs float64
	State   string
	Denied  bool
	Ports   []uint32
	Runtime string // "docker", "k8s", or "raw"
}

func listProcesses() []ProcInfo {
	procs, err := process.Processes()
	if err != nil {
		return nil
	}
	ports := getListenPorts()
	out := make([]ProcInfo, 0, len(procs))
	for _, p := range procs {
		name, nerr := p.Name()
		denied := nerr != nil
		if denied {
			name = "?"
		}

		cpu, _ := p.CPUPercent()

		var rss float64
		if mem, err := p.MemoryInfo(); err == nil && mem != nil {
			rss = float64(mem.RSS) / 1024 / 1024
		}

		state := "S"
		if statuses, err := p.Status(); err == nil && len(statuses) > 0 {
			state = statuses[0]
		}

		var diskKB float64
		if io, err := p.IOCounters(); err == nil && io != nil {
			diskKB = float64(io.ReadBytes+io.WriteBytes) / 1024
		}

		out = append(out, ProcInfo{
			PID:     p.Pid,
			Name:    name,
			CPU:     cpu,
			RSSMB:   rss,
			DiskKBs: diskKB,
			State:   state,
			Denied:  denied,
			Ports:   ports[p.Pid],
			Runtime: detectRuntime(p.Pid),
		})
	}
	return out
}
