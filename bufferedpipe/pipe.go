package bufferedpipe

import (
	"bytes"
	"io"
	"sync"
)

// BufferedPipe implements a PipeReadWriteCloser for an in-memory pipe with variable size buffer.
type BufferedPipe struct {
	buf        bytes.Buffer
	c          *sync.Cond
	pipeClosed bool
}

// New creates a new BufferedPipe
func New() *BufferedPipe {
	var m sync.Mutex

	bp := BufferedPipe{
		buf:        bytes.Buffer{},
		c:          sync.NewCond(&m),
		pipeClosed: false,
	}

	return &bp
}

// Read waits until there are len(p) bytes in the buffer, then copies them into p.
func (b *BufferedPipe) Read(p []byte) (n int, err error) {
	b.c.L.Lock()
	defer b.c.L.Unlock()
	defer b.c.Signal()

	for b.buf.Len() <= len(p) && !b.pipeClosed {
		b.c.Wait()
	}

	n, err = b.buf.Read(p)
	return
}

// Write copies len(p) bytes from p into the buffer, then wakes all waiting readers.
// Writing to a closed pipe returns a io.ErrUnexpectedEOF
func (b *BufferedPipe) Write(p []byte) (n int, err error) {
	b.c.L.Lock()
	defer b.c.L.Unlock()
	defer b.c.Signal()

	if b.pipeClosed {
		n, err = 0, io.ErrUnexpectedEOF
		return
	}

	n, err = b.buf.Write(p)
	return
}

// Close closes the pipe, then wakes all waiting readers.
func (b *BufferedPipe) Close() error {
	b.c.L.Lock()
	defer b.c.L.Unlock()
	defer b.c.Signal()

	b.pipeClosed = true

	return nil
}
