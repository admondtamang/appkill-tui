package main

import "github.com/shirou/gopsutil/v3/net"

// getListenPorts maps each PID to the local ports it holds in LISTEN state.
func getListenPorts() map[int32][]uint32 {
	conns, err := net.Connections("inet")
	if err != nil {
		return nil
	}
	out := map[int32][]uint32{}
	for _, c := range conns {
		if c.Status != "LISTEN" || c.Pid <= 0 {
			continue
		}
		out[c.Pid] = append(out[c.Pid], c.Laddr.Port)
	}
	return out
}
