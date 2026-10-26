package orgschema

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// These tests drive Fetch through a fake `sf` executable placed on PATH. They
// are skipped on Windows because the fake is a POSIX shell script.

func TestFetchHardFailsWhenOrgUnreachable(t *testing.T) {
	skipWithoutShell(t)

	writeFakeSF(t, "#!/bin/sh\necho boom 1>&2\nexit 1\n")

	if _, err := Fetch(""); err == nil {
		t.Fatal("expected Fetch to fail when the org cannot be reached")
	}
}

func TestFetchDoesNotRegisterFailedTypes(t *testing.T) {
	skipWithoutShell(t)

	writeFakeSF(t, `#!/bin/sh
for a in "$@"; do
  if [ "$a" = "metadata-types" ]; then
    echo '{"status":0,"result":{"metadataObjects":[{"xmlName":"ApexClass"}]}}'
    exit 0
  fi
done
echo boom 1>&2
exit 1
`)

	o, err := Fetch("")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if o == nil {
		t.Fatal("Fetch returned a nil schema")
	}

	// A type whose member query failed must stay unregistered, so the
	// normalizer skips filtering for it instead of deleting every entry.
	if o.HasType("ApexClass") {
		t.Error("a type whose query failed must not be registered")
	}

	if o.Has("ApexClass", "AnyClass") {
		t.Error("a failed type must not report members")
	}
}

func skipWithoutShell(t *testing.T) {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake sf is a POSIX shell script")
	}
}

func writeFakeSF(t *testing.T, script string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "sf")

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
