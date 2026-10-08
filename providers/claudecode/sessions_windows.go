//go:build windows

package claudecode

import (
	"errors"
	"time"
)

// psStart is not supported on Windows, where there is no portable way to read
// a process's start time using only the standard library. Returning an error
// makes the reader treat every session as not running, which is the safe
// answer: a session is never reported as live without the start-time check.
func psStart(int) (time.Time, error) {
	return time.Time{}, errors.New("process start time is not supported on windows")
}
