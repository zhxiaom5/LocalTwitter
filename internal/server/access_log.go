package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultAccessLogMaxBytes = int64(50 * 1024 * 1024)
	defaultAccessLogMaxFiles = 3
)

type accessLogger struct {
	path     string
	maxBytes int64
	maxFiles int
	mu       sync.Mutex
}

func newAccessLoggerFromEnv() *accessLogger {
	path := strings.TrimSpace(os.Getenv("LOCALTWITTER_ACCESS_LOG"))
	if path == "" {
		return nil
	}
	return newAccessLogger(path, defaultAccessLogMaxBytes, defaultAccessLogMaxFiles)
}

func newAccessLogger(path string, maxBytes int64, maxFiles int) *accessLogger {
	if maxBytes <= 0 {
		maxBytes = defaultAccessLogMaxBytes
	}
	if maxFiles <= 0 {
		maxFiles = defaultAccessLogMaxFiles
	}
	return &accessLogger{path: path, maxBytes: maxBytes, maxFiles: maxFiles}
}

func (l *accessLogger) middleware(next http.Handler) http.Handler {
	if l == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &accessLogResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		if err := l.write(accessLogLine(r, status, rec.bytesWritten, time.Since(start))); err != nil {
			log.Printf("write access log: %v", err)
		}
	})
}

func (l *accessLogger) write(line string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	if err := l.rotateIfNeeded(int64(len(line))); err != nil {
		return err
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(line); err != nil {
		return err
	}
	return writer.Flush()
}

func (l *accessLogger) rotateIfNeeded(incomingBytes int64) error {
	info, err := os.Stat(l.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size()+incomingBytes <= l.maxBytes {
		return nil
	}
	if l.maxFiles <= 1 {
		return os.Truncate(l.path, 0)
	}
	last := l.path + "." + strconv.Itoa(l.maxFiles-1)
	if err := os.Remove(last); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := l.maxFiles - 2; i >= 1; i-- {
		src := l.path + "." + strconv.Itoa(i)
		dst := l.path + "." + strconv.Itoa(i+1)
		if err := os.Rename(src, dst); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(l.path, l.path+".1"); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func accessLogLine(r *http.Request, status int, bytesWritten int64, elapsed time.Duration) string {
	requestURI := r.URL.RequestURI()
	if requestURI == "" {
		requestURI = r.URL.Path
	}
	requestLine := r.Method + " " + requestURI + " " + r.Proto
	return fmt.Sprintf("%s - - [%s] %q %d %d %q %q %q rt=%.3f request_time=%.3f request_length=%d host=%q range=%q\n",
		remoteHost(r.RemoteAddr),
		time.Now().Format("02/Jan/2006:15:04:05 -0700"),
		requestLine,
		status,
		bytesWritten,
		nginxLogValue(r.Referer()),
		nginxLogValue(r.UserAgent()),
		nginxLogValue(r.Header.Get("X-Forwarded-For")),
		elapsed.Seconds(),
		elapsed.Seconds(),
		approximateRequestLength(r, requestLine),
		nginxLogValue(r.Host),
		nginxLogValue(r.Header.Get("Range")),
	)
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil && host != "" {
		return host
	}
	if remoteAddr == "" {
		return "-"
	}
	return remoteAddr
}

func nginxLogValue(value string) string {
	if value == "" {
		return "-"
	}
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, "\r", `\r`)
	return value
}

func approximateRequestLength(r *http.Request, requestLine string) int64 {
	// Mirrors nginx's request_length closely enough for local diagnostics:
	// request line, headers, terminating CRLF, and the declared body length.
	length := int64(len(requestLine) + len("\r\n"))
	if r.Host != "" {
		length += int64(len("Host") + len(": ") + len(r.Host) + len("\r\n"))
	}
	for name, values := range r.Header {
		for _, value := range values {
			length += int64(len(name) + len(": ") + len(value) + len("\r\n"))
		}
	}
	length += int64(len("\r\n"))
	if r.ContentLength > 0 {
		length += r.ContentLength
	}
	return length
}

type accessLogResponseWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int64
}

func (w *accessLogResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *accessLogResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *accessLogResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *accessLogResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
