package cmd

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The fatal-refusal branch (--name combined with an explicit --file/
// --set-file/--images-dir) isn't covered here - it calls log.Fatal, which
// exits the process, not something a normal in-process test can assert on.

func TestResolveProcessSetPaths_NameEmptyPassesThroughUnchanged(t *testing.T) {
	file, setFile, imagesDir := resolveProcessSetPaths("", "a.csv", "b.json", "c")
	require.Equal(t, "a.csv", file)
	require.Equal(t, "b.json", setFile)
	require.Equal(t, "c", imagesDir)
}

func TestResolveProcessSetPaths_NameDerivesStandardLayout(t *testing.T) {
	file, setFile, imagesDir := resolveProcessSetPaths("some-set", "", "", "")
	require.Equal(t, filepath.Join("sets", "some-set", "catalog.csv"), file)
	require.Equal(t, filepath.Join("sets", "some-set", "set.json"), setFile)
	require.Equal(t, filepath.Join("sets", "some-set", "images"), imagesDir)
}
