//go:build windows

package process

import (
	"log"
	"os/exec"
	"time"
)

// setSysProcAttr is a no-op on Windows (no Setpgid).
func setSysProcAttr(cmd *exec.Cmd) {}

// gracefulStop kills the process on Windows (no SIGTERM support).
func gracefulStop(cmd *exec.Cmd, id string) {
	_ = cmd.Process.Kill()

	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[%s] process exited", id)
	case <-time.After(10 * time.Second):
		log.Printf("[%s] process did not exit after 10s", id)
	}
}
