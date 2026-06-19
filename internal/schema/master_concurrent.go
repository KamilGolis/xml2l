package schema

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	"xml2l/internal/profile"
	"xml2l/internal/scanner"
)

// AggregateResult holds the output of one goroutine.
type AggregateResult struct {
	Raw     []byte
	Profile *profile.Profile
	Path    string
}

// RunConcurrent decodes all profiles at paths concurrently, merges their
// entries into a MasterSchema, and returns results in original path order.
func RunConcurrent(paths []string, gt scanner.GroundTruth) ([]AggregateResult, *MasterSchema, []error) {
	if len(paths) == 0 {
		return nil, NewMasterSchema(), nil
	}

	type jobResult struct {
		idx  int
		raw  []byte
		prof *profile.Profile
		err  error
	}

	var wg sync.WaitGroup
	results := make(chan jobResult, len(paths))

	// Fan-out: decode each profile concurrently.
	for i, path := range paths {
		wg.Add(1)
		go func(idx int, filePath string) {
			defer wg.Done()

			raw, err := os.ReadFile(filePath)
			if err != nil {
				results <- jobResult{idx: idx, err: fmt.Errorf("read %s: %w", filePath, err)}
				return
			}

			prof, err := profile.Decode(bytes.NewReader(raw), gt)
			if err != nil {
				results <- jobResult{idx: idx, err: fmt.Errorf("decode %s: %w", filePath, err)}
				return
			}

			results <- jobResult{idx: idx, raw: raw, prof: prof}
		}(i, path)
	}

	wg.Wait()
	close(results)

	// Collect results in a buffer indexed by original position.
	buf := make([]*jobResult, len(paths))
	var errs []error
	for r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		buf[r.idx] = &r
	}

	// Fan-in: build Master Schema sequentially from all successful decodes.
	ms := NewMasterSchema()
	ordered := make([]AggregateResult, 0, len(paths))
	for i, r := range buf {
		if r == nil {
			continue
		}
		if r.prof != nil {
			ms.merge(r.prof)
		}
		ordered = append(ordered, AggregateResult{
			Raw:     r.raw,
			Profile: r.prof,
			Path:    paths[i],
		})
	}

	return ordered, ms, errs
}
