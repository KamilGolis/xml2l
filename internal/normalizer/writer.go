package normalizer

import (
	"fmt"
	"os"
	"sync"

	"xml2l/internal/graph"
)

// WriteProfiles normalizes all profiles in the graph and writes them to disk
// concurrently. One goroutine per profile — no mutex required since each file
// path is strictly isolated.
func WriteProfiles(g NormalizeGraph) error {
	var wg sync.WaitGroup
	errs := make(chan error, len(g.Profiles()))

	for _, p := range g.Profiles() {
		if p.SourcePath == "" {
			continue
		}

		wg.Add(1)
		go func(profile *graph.ProfileNode) {
			defer wg.Done()

			xmlBytes := NormalizeProfile(profile, g)
			if xmlBytes == nil {
				errs <- fmt.Errorf("no XML generated for %s", profile.Name)
				return
			}

			if err := os.WriteFile(profile.SourcePath, xmlBytes, 0644); err != nil {
				errs <- fmt.Errorf("write %s: %w", profile.SourcePath, err)
			}
		}(p)
	}

	wg.Wait()
	close(errs)

	// Collect errors (report first error).
	// We still attempt all writes before returning.
	var firstErr error
	for err := range errs {
		if firstErr == nil {
			firstErr = err
		}
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	return firstErr
}
