package normalizer

import (
	"fmt"
	"os"
	"runtime"
	"sync"

	"xml2l/internal/graph"
)

// WriteProfiles normalizes all profiles in the graph and writes them to disk
// concurrently. One goroutine per profile — no mutex required since each file
// path is strictly isolated.
func WriteProfiles(g NormalizeGraph) error {
	profiles := g.Profiles()
	maxWorkers := runtime.NumCPU()
	if maxWorkers > len(profiles) {
		maxWorkers = len(profiles)
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	sem := make(chan struct{}, maxWorkers)

	var wg sync.WaitGroup
	errs := make(chan error, len(profiles))

	for _, p := range profiles {
		if p.SourcePath == "" {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(profile *graph.ProfileNode) {
			defer wg.Done()
			defer func() { <-sem }()

			xmlBytes := NormalizeProfile(profile, g)
			if xmlBytes == nil {
				errs <- fmt.Errorf("No XML generated for %s", profile.Name)
				return
			}

			if err := os.WriteFile(profile.SourcePath, xmlBytes, 0644); err != nil {
				errs <- fmt.Errorf("Write %s: %w", profile.SourcePath, err)
			}
		}(p)
	}

	wg.Wait()
	close(errs)

	// Collect errors (report first error).
	var firstErr error
	for err := range errs {
		if firstErr == nil {
			firstErr = err
		}
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	return firstErr
}
