//go:build darwin

package hostinfo

import (
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// macOS exposes the load average through sysctl, but reading it as a binary
// struct needs golang.org/x/sys/unix: the standard library's syscall package
// exports Sysctl (string-truncating, useless for a struct) and not SysctlRaw.
//
// Adding a dependency to the agent for a development-only platform is the
// wrong trade -- community probes run Linux, where /proc/loadavg is a file
// read -- so this path shells out instead, and caches so a measurement round
// costs one fork rather than one per target.
var (
	mu       sync.Mutex
	cached   float64
	cachedOK bool
	cachedAt time.Time
)

const cacheTTL = 5 * time.Second

func load1() (float64, bool) {
	mu.Lock()
	defer mu.Unlock()

	if time.Since(cachedAt) < cacheTTL {
		return cached, cachedOK
	}

	out, err := exec.Command("sysctl", "-n", "vm.loadavg").Output()
	cachedAt = time.Now()
	if err != nil {
		cached, cachedOK = 0, false
		return cached, cachedOK
	}

	// Output looks like: { 2.15 2.44 2.60 }
	fields := strings.Fields(strings.Trim(strings.TrimSpace(string(out)), "{}"))
	if len(fields) == 0 {
		cached, cachedOK = 0, false
		return cached, cachedOK
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		cached, cachedOK = 0, false
		return cached, cachedOK
	}
	cached, cachedOK = v, true
	return cached, cachedOK
}
