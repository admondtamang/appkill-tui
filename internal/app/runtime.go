package app

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// detectRuntime inspects /proc/<pid>/cgroup to tell whether a process is
// running inside Kubernetes, inside plain Docker, or directly on the host.
func detectRuntime(pid int32) string {
	f, err := os.Open(fmt.Sprintf("/proc/%d/cgroup", pid))
	if err != nil {
		return "raw"
	}
	defer f.Close()

	hasK8s, hasDocker := false, false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "kubepods") {
			hasK8s = true
		}
		if strings.Contains(line, "docker") {
			hasDocker = true
		}
	}
	_ = scanner.Err()
	switch {
	case hasK8s:
		return "k8s"
	case hasDocker:
		return "docker"
	default:
		return "raw"
	}
}
