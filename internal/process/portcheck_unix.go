//go:build !windows

package process

import (
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// listenersOnPort returns the PIDs that hold a TCP LISTEN socket on the given
// local port. It prefers lsof and falls back to ss so it works across macOS
// and Linux distros. An empty slice means nothing is listening.
func listenersOnPort(port int) []int {
	if pids := lsofListeners(port); pids != nil {
		return pids
	}
	return ssListeners(port)
}

func lsofListeners(port int) []int {
	path, err := exec.LookPath("lsof")
	if err != nil {
		return nil
	}
	// -t: terse (PIDs only), -nP: no name/port resolution, listening TCP sockets only.
	out, err := exec.Command(path, "-nP", "-iTCP:"+strconv.Itoa(port), "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		// lsof exits non-zero when there are no matches; treat as "none".
		if _, ok := err.(*exec.ExitError); ok {
			return []int{}
		}
		return nil
	}
	return parsePIDLines(string(out))
}

func ssListeners(port int) []int {
	path, err := exec.LookPath("ss")
	if err != nil {
		return []int{}
	}
	out, err := exec.Command(path, "-ltnH", "sport = :"+strconv.Itoa(port)).Output()
	if err != nil {
		return []int{}
	}
	var pids []int
	for _, line := range strings.Split(string(out), "\n") {
		// users:(("ssh",pid=3782,fd=6))
		idx := strings.Index(line, "pid=")
		for idx != -1 {
			rest := line[idx+4:]
			end := strings.IndexAny(rest, ",)")
			if end == -1 {
				break
			}
			if pid, err := strconv.Atoi(rest[:end]); err == nil {
				pids = append(pids, pid)
			}
			next := strings.Index(rest, "pid=")
			if next == -1 {
				break
			}
			idx = idx + 4 + next
		}
	}
	return pids
}

func parsePIDLines(s string) []int {
	var pids []int
	for _, line := range strings.Fields(s) {
		if pid, err := strconv.Atoi(strings.TrimSpace(line)); err == nil {
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

// childMap builds a parent-PID -> child-PIDs map from `ps`.
func childMap() map[int][]int {
	out, err := exec.Command("ps", "-axo", "pid=,ppid=").Output()
	if err != nil {
		return nil
	}
	children := make(map[int][]int)
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		children[ppid] = append(children[ppid], pid)
	}
	return children
}

// killPID forcefully terminates a process by PID.
func killPID(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
