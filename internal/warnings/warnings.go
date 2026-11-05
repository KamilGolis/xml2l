// Package warnings collects the diagnostic lines a command emits. Human runs
// stream them to a writer as they happen; a machine run can keep them in the
// collector and carry them in its document instead.
package warnings

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Prefix opens every line the commands write.
const Prefix = "warning: "

// Collector is an io.Writer that keeps every warning line and, when out is not
// nil, passes the text through so a human sees it immediately. It is safe for
// concurrent use, which the normalizer needs: profiles are written from one
// goroutine each. The zero value is not usable; call New.
type Collector struct {
	mu  sync.Mutex
	out io.Writer
	buf bytes.Buffer
}

// New returns a collector that streams to out. Pass nil to collect silently.
func New(out io.Writer) *Collector {
	return &Collector{out: out}
}

// Write keeps p and streams it to the destination, if any.
func (c *Collector) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.buf.Write(p)

	if c.out == nil {
		return len(p), nil
	}

	return c.out.Write(p)
}

// Warnf writes one prefixed warning line.
func (c *Collector) Warnf(format string, args ...any) {
	fmt.Fprintf(c, Prefix+format+"\n", args...)
}

// Lines returns the collected warnings without their prefix or trailing
// newline, in the order they were written.
func (c *Collector) Lines() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var lines []string

	for _, line := range strings.Split(c.buf.String(), "\n") {
		line = strings.TrimSuffix(line, "\r")

		if line == "" {
			continue
		}

		lines = append(lines, strings.TrimPrefix(line, Prefix))
	}

	return lines
}

// Len reports how many warning lines were collected.
func (c *Collector) Len() int {
	return len(c.Lines())
}
