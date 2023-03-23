package logger

import (
	"bytes"
	"sync"
)

// Util struct for concurrent buffer for testing logging concurrency
// Taken from: https://stackoverflow.com/questions/19646717/is-the-go-bytes-buffer-thread-safe
type Buffer struct {
	sync.Mutex
	b bytes.Buffer
}

func (b *Buffer) Read(p []byte) (n int, err error) {
	b.Lock()
	defer b.Unlock()
	return b.b.Read(p)
}

func (b *Buffer) Write(p []byte) (n int, err error) {
	b.Lock()
	defer b.Unlock()
	return b.b.Write(p)
}

func (b *Buffer) String() string {
	b.Lock()
	defer b.Unlock()
	return b.b.String()
}

func (b *Buffer) Reset() {
	b.Lock()
	defer b.Unlock()
	b.b.Reset()
}
