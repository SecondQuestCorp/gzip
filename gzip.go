package gzip

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/gin-gonic/gin"
)

const (
	BestCompression    = gzip.BestCompression
	BestSpeed          = gzip.BestSpeed
	DefaultCompression = gzip.DefaultCompression
	NoCompression      = gzip.NoCompression
)

func Gzip(level int, options ...Option) gin.HandlerFunc {
	return newGzipHandler(level, options...).Handle
}

type gzipWriter struct {
	gin.ResponseWriter
	writer *gzip.Writer
}

func (g *gzipWriter) WriteString(s string) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write([]byte(s))
}

func (g *gzipWriter) Write(data []byte) (int, error) {
	g.Header().Del("Content-Length")
	return g.writer.Write(data)
}

// Fix: https://github.com/mholt/caddy/issues/38
func (g *gzipWriter) WriteHeader(code int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

type thresholdWriter struct {
	gin.ResponseWriter
	handler *gzipHandler
	buffer  *bytes.Buffer
	gz      *gzip.Writer
}

func (t *thresholdWriter) WriteString(s string) (int, error) {
	t.Header().Del("Content-Length")
	return t.doWrite([]byte(s))
}

func (t *thresholdWriter) Write(data []byte) (int, error) {
	t.Header().Del("Content-Length")
	return t.doWrite(data)
}

func (t *thresholdWriter) WriteHeader(code int) {
	t.Header().Del("Content-Length")
	t.ResponseWriter.WriteHeader(code)
}

func (t *thresholdWriter) Close() error {
	if t.isCompressed() {
		t.gz.Close()
		t.gz.Reset(io.Discard)
		t.handler.gzPool.Put(t.gz)
		return nil
	} else {
		_, err := t.ResponseWriter.Write(t.buffer.Bytes())
		t.buffer.Reset()
		return err
	}
}

func (t *thresholdWriter) doWrite(data []byte) (int, error) {
	if t.isCompressed() {
		return t.gz.Write(data)
	}

	shouldCompress := t.buffer.Len()+len(data) >= t.handler.CompressionSizeThreshold
	if shouldCompress {
		t.gz = t.handler.gzPool.Get().(*gzip.Writer)
		t.gz.Reset(t.ResponseWriter)
		n, err := t.gz.Write(t.buffer.Bytes())
		if err != nil {
			return n, err
		}
		t.buffer.Reset()
		return t.gz.Write(data)
	} else {
		return t.buffer.Write(data)
	}
}

func (t *thresholdWriter) isCompressed() bool {
	return t.gz != nil
}
