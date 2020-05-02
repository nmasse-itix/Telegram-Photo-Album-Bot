package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeAlbumName(t *testing.T) {
	input := "Mes premières années"
	want := "mes-premieres-annees"
	if got := sanitizeAlbumName(input); got != want {
		t.Errorf("sanitizeAlbumName() = %q, want %q", got, want)
	}
}

func TestNewMediaStore(t *testing.T) {
	tmp := createTempDir(t)
	defer tmp.cleanup(t)

	_, err := InitMediaStore(tmp.RootDir)
	if err != nil {
		t.Errorf("InitMediaStore(): error %s", err)
	}
	stat, err := os.Stat(filepath.Join(tmp.RootDir, ".current"))
	if err != nil || !stat.IsDir() {
		t.Errorf("InitMediaStore(): .current not created (error = %s)", err)
	}
}
