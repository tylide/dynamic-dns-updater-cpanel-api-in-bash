package lock

import (
	"fmt"
	"os"
	"time"
)

// Acquire creates path exclusively. Stale files older than staleAfter are
// removed and retried once. Returns release func.
func Acquire(path string, staleAfter time.Duration) (func(), error) {
	if err := tryCreate(path); err == nil {
		return func() { _ = os.Remove(path) }, nil
	} else if !os.IsExist(err) {
		return nil, err
	}
	st, statErr := os.Stat(path)
	if statErr == nil && time.Since(st.ModTime()) > staleAfter {
		_ = os.Remove(path)
		if err := tryCreate(path); err == nil {
			return func() { _ = os.Remove(path) }, nil
		}
	}
	return nil, fmt.Errorf("locked (%s existe, remova se for stale)", path)
}

func tryCreate(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	fmt.Fprintf(f, "%d %s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	f.Close()
	return nil
}
