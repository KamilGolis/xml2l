package warnings

import (
	"bytes"
	"strings"
	"sync"
	"testing"
)

func TestCollectorStreamsAndKeepsLines(t *testing.T) {
	var out bytes.Buffer

	c := New(&out)

	if _, err := c.Write([]byte("warning: first\nwarning: second\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if out.String() != "warning: first\nwarning: second\n" {
		t.Errorf("streamed text changed: %q", out.String())
	}

	got := c.Lines()

	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Errorf("lines = %q, want [first second]", got)
	}

	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}
}

func TestCollectorWarnf(t *testing.T) {
	var out bytes.Buffer

	c := New(&out)
	c.Warnf("skip %s: %v", "a.xml", "denied")

	if out.String() != "warning: skip a.xml: denied\n" {
		t.Errorf("streamed = %q", out.String())
	}

	if got := c.Lines(); len(got) != 1 || got[0] != "skip a.xml: denied" {
		t.Errorf("lines = %q", got)
	}
}

func TestCollectorSilent(t *testing.T) {
	c := New(nil)

	if _, err := c.Write([]byte("warning: kept\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := c.Lines(); len(got) != 1 || got[0] != "kept" {
		t.Errorf("lines = %q, want [kept]", got)
	}
}

func TestCollectorConcurrentWrites(t *testing.T) {
	c := New(nil)

	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if _, err := c.Write([]byte("warning: entry\n")); err != nil {
				t.Errorf("write: %v", err)
			}
		}()
	}

	wg.Wait()

	if c.Len() != 50 {
		t.Errorf("Len = %d, want 50", c.Len())
	}

	for _, line := range c.Lines() {
		if strings.TrimSpace(line) != "entry" {
			t.Errorf("unexpected line %q", line)
		}
	}
}
