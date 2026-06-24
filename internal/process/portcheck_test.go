package process

import (
	"net"
	"os"
	"testing"
)

func TestProcessTreeIncludesSelf(t *testing.T) {
	tree := processTreePIDs(os.Getpid())
	if !tree[os.Getpid()] {
		t.Fatalf("process tree should include self pid %d: %v", os.Getpid(), tree)
	}
}

func TestListenersOnPortFindsSelf(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	pids := listenersOnPort(port)
	if len(pids) == 0 {
		t.Skipf("no listeners reported for port %d (lsof/ss/netstat unavailable?)", port)
	}

	self := os.Getpid()
	found := false
	for _, pid := range pids {
		if pid == self {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected self pid %d among listeners %v on port %d", self, pids, port)
	}
}

func TestListenersOnPortEmptyWhenClosed(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	if pids := listenersOnPort(port); len(pids) != 0 {
		t.Fatalf("expected no listeners on closed port %d, got %v", port, pids)
	}
}
