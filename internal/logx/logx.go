package logx

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger appends to a file and mirrors warnings/errors to stderr.
type Logger struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	verbose bool
}

const maxSize = 10 << 20 // 10MB rotation

// New opens path (MkdirAll). Empty path = stderr only.
// If open fails (ex: sem permissão), returns stderr-only logger + open error.
func New(path string, verbose bool) (*Logger, error) {
	l := &Logger{path: path, verbose: verbose}
	if stringsTrim(path) == "" {
		return l, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return l, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return l, err
	}
	l.file = f
	return l, nil
}

// LocalFallbackPath devolve exe-dir/dnsupdater.log (ou ./dnsupdater.log).
// Usado quando o path principal (ex: /var/log) não tem permissão ou não
// existe (Windows).
func LocalFallbackPath() string {
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			return filepath.Join(dir, "dnsupdater.log")
		}
	}
	return "dnsupdater.log"
}

// IsSystemLogPath indica log fora da pasta local (ex: /var/log/...).
func IsSystemLogPath(p string) bool {
	if p == "" {
		return false
	}
	return filepath.IsAbs(p)
}

// EnsureLocalLink cria link pasta-atual/dnsupdater.log -> target (best-effort).
// Não sobrescreve ficheiro regular existente; substitui symlink stale.
// Em Windows sem privilégio falha silenciosamente (retorna err, caller ignora).
func EnsureLocalLink(target string) error {
	if target == "" {
		return fmt.Errorf("target vazio")
	}
	dirs := map[string]bool{}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		dirs[cwd] = true
	}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" {
			dirs[dir] = true
		}
	}
	var firstErr error
	for dir := range dirs {
		link := filepath.Join(dir, "dnsupdater.log")
		if samePath(link, target) {
			continue
		}
		if fi, err := os.Lstat(link); err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				if dest, err := os.Readlink(link); err == nil && samePath(dest, target) {
					continue
				}
				_ = os.Remove(link)
			} else {
				continue // ficheiro regular: não tocar
			}
		}
		if err := os.Symlink(target, link); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func samePath(a, b string) bool {
	if a == b {
		return true
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	return errA == nil && errB == nil && aa == bb
}
func stringsTrim(s string) string {
	// tiny trim to avoid importing strings in signature area
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n' || s[0] == '\r') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// Close releases the file handle.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

func (l *Logger) rotateLocked() {
	if l.file == nil || l.path == "" {
		return
	}
	st, err := l.file.Stat()
	if err != nil || st.Size() < maxSize {
		return
	}
	l.file.Close()
	_ = os.Remove(l.path + ".1")
	_ = os.Rename(l.path, l.path+".1")
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		l.file = nil
		fmt.Fprintf(os.Stderr, "dnsupdater: rotação falhou: %v\n", err)
		return
	}
	l.file = f
}

func (l *Logger) log(level, msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rotateLocked()
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format(time.RFC3339), level, msg)
	if l.file != nil {
		fmt.Fprint(l.file, line)
	}
	if l.verbose || level == "ERROR" || level == "WARN" {
		fmt.Fprint(os.Stderr, line)
	}
}

// Infof logs informational messages.
func (l *Logger) Infof(f string, a ...interface{}) { l.log("INFO", fmt.Sprintf(f, a...)) }

// Warnf logs warnings.
func (l *Logger) Warnf(f string, a ...interface{}) { l.log("WARN", fmt.Sprintf(f, a...)) }

// Errorf logs errors.
func (l *Logger) Errorf(f string, a ...interface{}) { l.log("ERROR", fmt.Sprintf(f, a...)) }
