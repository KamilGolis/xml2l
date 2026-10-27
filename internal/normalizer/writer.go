package normalizer

import (
	"fmt"
	"os"
	"path/filepath"
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
	unknown := newUnknownReport()

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

			xmlBytes, err := NormalizeProfile(profile, g, unknown)
			if err != nil {
				errs <- fmt.Errorf("Normalize %s: %w", profile.SourcePath, err)
				return
			}

			if xmlBytes == nil {
				errs <- fmt.Errorf("No XML generated for %s", profile.Name)
				return
			}

			if err := writeFileAtomic(profile.SourcePath, xmlBytes); err != nil {
				errs <- fmt.Errorf("Write %s: %w", profile.SourcePath, err)
			} else {
				fmt.Fprintf(os.Stderr, "Normalized: %s\n", profile.Name)
			}
		}(p)
	}

	wg.Wait()
	close(errs)

	unknown.print(os.Stderr)

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

// writeFileAtomic writes data to path through a temporary file in the same
// directory followed by a rename. A crash or interrupt mid-write leaves the
// original profile intact instead of truncating it, and the existing file
// mode is preserved.
func writeFileAtomic(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}

	tmpName := tmp.Name()
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	if err := replaceFile(tmpName, path); err != nil {
		return err
	}

	tmpName = "" // renamed into place; nothing left to clean up
	return nil
}

// replaceFile renames tmpName over path. Windows refuses to replace a target
// that carries the read-only attribute, so a failed rename is retried once with
// the attribute cleared; the target keeps its mode either way. The original
// error is returned when the retry does not apply, so a real failure still
// names its cause.
func replaceFile(tmpName, path string) error {
	err := os.Rename(tmpName, path)
	if err == nil {
		return nil
	}

	info, statErr := os.Stat(path)
	if statErr != nil {
		return err
	}

	mode := info.Mode().Perm()
	if mode&0o200 != 0 {
		return err
	}

	if chmodErr := os.Chmod(path, mode|0o200); chmodErr != nil {
		return err
	}

	renameErr := os.Rename(tmpName, path)

	// The new file was created with the old mode, so this only guards against
	// a platform that carried the writable bit across the rename.
	if chmodErr := os.Chmod(path, mode); chmodErr != nil && renameErr == nil {
		return chmodErr
	}

	return renameErr
}
