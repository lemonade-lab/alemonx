package web

import (
	"net"
	"strconv"
)

// portOccupant identifies one process that is listening on a port. CWD is
// empty on platforms whose tooling cannot report it cheaply.
type portOccupant struct {
	PID     int
	Process string
	CWD     string
}

// sniffPort probes whether any TCP listener is bound to the port on any
// interface and, when platform tooling is available, which processes hold it.
// The bind probe is authoritative for occupancy: if it succeeds the port is
// free for a new listener.
func sniffPort(port int) (bool, []portOccupant) {
	if port < 1 || port > 65535 {
		return false, nil
	}
	// macOS/BSD allow a wildcard bind to coexist with a listener on a specific
	// address, so a single probe misses real conflicts. Probe both the loopback
	// address the workbench talks to and the wildcard address the robot may
	// bind; either bind failing means the port is taken.
	occupied := false
	for _, address := range []string{"127.0.0.1:" + strconv.Itoa(port), ":" + strconv.Itoa(port)} {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			occupied = true
			continue
		}
		_ = listener.Close()
	}
	if !occupied {
		return false, nil
	}
	pids := portListenerPIDs(port)
	occupants := make([]portOccupant, 0, len(pids))
	for _, pid := range pids {
		occupants = append(occupants, portOccupant{
			PID:     pid,
			Process: processDescription(pid),
			CWD:     processWorkingDirectory(pid),
		})
	}
	return true, occupants
}
