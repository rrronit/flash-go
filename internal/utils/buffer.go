package utils

import (
	"bytes"
	"strings"
	"sync"
)

var (
	// BufferPool for reading file contents (stdout, stderr, etc.)
	BufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, 64*1024)) // 64KB initial capacity
		},
	}
	// StringBuilderPool for small string builders (command construction)
	StringBuilderPool = sync.Pool{
		New: func() interface{} {
			return new(strings.Builder)
		},
	}
)

// GetBuffer retrieves a buffer from the pool.
func GetBuffer() *bytes.Buffer {
	buf := BufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// PutBuffer returns a buffer to the pool.
func PutBuffer(buf *bytes.Buffer) {
	if buf.Cap() <= 1024*1024 { // Only pool buffers <= 1MB
		BufferPool.Put(buf)
	}
}

// GetStringBuilder retrieves a string builder from the pool.
func GetStringBuilder() *strings.Builder {
	sb := StringBuilderPool.Get().(*strings.Builder)
	sb.Reset()
	return sb
}

// PutStringBuilder returns a string builder to the pool.
func PutStringBuilder(sb *strings.Builder) {
	StringBuilderPool.Put(sb)
}