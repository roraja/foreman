//go:build !windows

package process

import (
	"log"
	"os/exec"
	"syscall"
	"time"
)

// setSysProcAttr configures the process to run in its own process group.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// gracefulStop sends SIGTERM to the process group, then SIGKILL after timeout.
func gracefulStop(cmd *exec.Cmd, id string) {
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[%s] process exited gracefully", id)
	case <-time.After(10 * time.Second):
		log.Printf("[%s] process did not exit after 10s, sending SIGKILL", id)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
