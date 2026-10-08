//go:build !windows

package claudecode

import (
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// psStart returns the start time of a running process via ps. It returns an
// error when the process does not exist.
func psStart(pid int) (time.Time, error) {
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return time.Time{}, err
	}
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output() //nolint:gosec // pid is an int
	if err != nil {
		return time.Time{}, err
	}
	// lstart is local time, e.g. "Wed Oct  7 10:04:54 2026".
	return time.ParseInLocation("Mon Jan _2 15:04:05 2006", strings.TrimSpace(string(out)), time.Local)
}
