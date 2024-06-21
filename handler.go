package gzip

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type gzipHandler struct {
	*Options
	gzPool     sync.Pool
	bufferPool sync.Pool
}

func newGzipHandler(level int, options ...Option) *gzipHandler {
	handler := &gzipHandler{
		Options: DefaultOptions,
		gzPool: sync.Pool{
			New: func() interface{} {
				gz, err := gzip.NewWriterLevel(io.Discard, level)
				if err != nil {
					panic(err)
				}
				return gz
			},
		},
	}
	for _, setter := range options {
		setter(handler.Options)
	}

	handler.bufferPool.New = func() interface{} {
		b := new(bytes.Buffer)
		b.Grow(handler.CompressionSizeThreshold)
		return b
	}

	return handler
}

func (g *gzipHandler) Handle(c *gin.Context) {
	if fn := g.DecompressFn; fn != nil && c.Request.Header.Get("Content-Encoding") == "gzip" {
		fn(c)
	}

	if !g.shouldCompress(c.Request) {
		return
	}

	if g.CompressionSizeThreshold <= 0 {
		gz := g.gzPool.Get().(*gzip.Writer)
		defer g.gzPool.Put(gz)
		defer gz.Reset(io.Discard)
		gz.Reset(c.Writer)

		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		c.Writer = &gzipWriter{c.Writer, gz}
		defer func() {
			gz.Close()
			c.Header("Content-Length", fmt.Sprint(c.Writer.Size()))
		}()
	} else {
		buffer := g.bufferPool.Get().(*bytes.Buffer)
		defer g.bufferPool.Put(buffer)
		defer buffer.Reset()

		tw := &thresholdWriter{
			ResponseWriter: c.Writer,
			handler:        g,
			buffer:         buffer,
		}
		c.Writer = tw
		defer func() {
			tw.Close()
			c.Header("Content-Length", fmt.Sprint(c.Writer.Size()))
		}()
	}

	c.Next()
}

func (g *gzipHandler) shouldCompress(req *http.Request) bool {
	if !strings.Contains(req.Header.Get("Accept-Encoding"), "gzip") ||
		strings.Contains(req.Header.Get("Connection"), "Upgrade") ||
		strings.Contains(req.Header.Get("Accept"), "text/event-stream") {
		return false
	}

	extension := filepath.Ext(req.URL.Path)
	if g.ExcludedExtensions.Contains(extension) {
		return false
	}

	if g.ExcludedPaths.Contains(req.URL.Path) {
		return false
	}
	if g.ExcludedPathesRegexs.Contains(req.URL.Path) {
		return false
	}

	return true
}
