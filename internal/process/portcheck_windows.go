//go:build windows

package process

import (
	"os/exec"
	"strconv"
	"strings"
)

// listenersOnPort returns the PIDs that hold a TCP LISTEN socket on the given
// local port, parsed from `netstat -ano`. An empty slice means nothing is
// listening.
func listenersOnPort(port int) []int {
	out, err := exec.Command("netstat", "-ano", "-p", "TCP").Output()
	if err != nil {
		return []int{}
	}
	suffix := ":" + strconv.Itoa(port)
	seen := make(map[int]bool)
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		// Proto  LocalAddress  ForeignAddress  State  PID
		if len(fields) < 5 {
			continue
		}
		if !strings.EqualFold(fields[3], "LISTENING") {
			continue
		}
		local := fields[1]
		if !strings.HasSuffix(local, suffix) {
			continue
		}
		if pid, err := strconv.Atoi(fields[len(fields)-1]); err == nil && !seen[pid] {
			seen[pid] = true
			pids = append(pids, pid)
		}
	}
	return pids
}

// processTreePIDs returns the set of PIDs in the process tree rooted at root
// (root included), so a port held by a child process still counts as owned.
func processTreePIDs(root int) map[int]bool {
	tree := map[int]bool{root: true}
	children := childMap()
	if len(children) == 0 {
		return tree
	}
	queue := []int{root}
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		for _, c := range children[pid] {
			if !tree[c] {
				tree[c] = true
				queue = append(queue, c)
			}
		}
	}
	return tree
}

// childMap builds a parent-PID -> child-PIDs map via WMIC.
func childMap() map[int][]int {
	out, err := exec.Command("wmic", "process", "get", "ProcessId,ParentProcessId", "/format:csv").Output()
	if err != nil {
		return nil
	}
	children := make(map[int][]int)
	for _, line := range strings.Split(string(out), "\n") {
		parts := strings.Split(strings.TrimSpace(line), ",")
		if len(parts) < 3 {
			continue
		}
		// Node,ParentProcessId,ProcessId
		ppid, err1 := strconv.Atoi(strings.TrimSpace(parts[len(parts)-2]))
		pid, err2 := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1]))
		if err1 != nil || err2 != nil {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}
	return children
}

// killPID forcefully terminates a process (and its children) by PID.
func killPID(pid int) error {
	return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid)).Run()
}
