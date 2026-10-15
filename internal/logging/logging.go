// Package logging 提供本地日志（带大小轮转）。
//
// 日志只写本地文件，绝不包含合同字段值、合同正文、银行账号等敏感信息。
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

const (
	maxFileBytes = 5 << 20 // 单个日志文件 5 MB
	maxFiles     = 3
)

type rotatingWriter struct {
	mu      sync.Mutex
	dir     string
	base    string
	file    *os.File
	size    int64
	current int
}

func newRotatingWriter(dir, base string) (*rotatingWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &rotatingWriter{dir: dir, base: base}
	if err := w.open(0); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) open(index int) error {
	if w.file != nil {
		_ = w.file.Close()
	}
	path := filepath.Join(w.dir, fmt.Sprintf("%s.%d.log", w.base, index))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	w.current = index
	return nil
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size+int64(len(p)) > maxFileBytes {
		next := (w.current + 1) % maxFiles
		_ = os.Remove(filepath.Join(w.dir, fmt.Sprintf("%s.%d.log", w.base, next)))
		if err := w.open(next); err != nil {
			return 0, err
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	return w.file.Close()
}

// New 创建日志记录器。debug=true 时同时输出到标准错误（仅开发使用）。
func New(logDir string, debug bool) (*slog.Logger, io.Closer, error) {
	w, err := newRotatingWriter(logDir, "inspiration")
	if err != nil {
		return nil, nil, err
	}
	var out io.Writer = w
	if debug {
		out = io.MultiWriter(w, os.Stderr)
	}
	handler := slog.NewTextHandler(out, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler), w, nil
}
