package main

import (
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

type Overview struct {
	CPUPercent  float64
	MemPercent  float64
	DiskPercent float64
}

func getOverview() Overview {
	var o Overview
	if pct, err := cpu.Percent(0, false); err == nil && len(pct) > 0 {
		o.CPUPercent = pct[0]
	}
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		o.MemPercent = vm.UsedPercent
	}
	if du, err := disk.Usage("/"); err == nil && du != nil {
		o.DiskPercent = du.UsedPercent
	}
	return o
}
