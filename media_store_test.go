package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magiconair/properties/assert"
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

func TestMediaStore(t *testing.T) {
	tmp := createTempDir(t)
	defer tmp.cleanup(t)

	store, err := InitMediaStore(tmp.RootDir)
	if err != nil {
		t.Errorf("InitMediaStore(): error %s", err)
	}

	id1 := store.GetUniqueID()
	fd1, err := store.AddFile(id1 + ".jpeg")
	if err != nil {
		t.Errorf("AddFile(): error %s", err)
	}
	fd1.WriteString("JPEG File")
	fd1.Close()

	err = store.CommitPhoto(id1, time.Now(), "This is a test")
	if err != nil {
		t.Errorf("CommitPhoto(): error %s", err)
	}

	id2 := store.GetUniqueID()
	fd2, err := store.AddFile(id2 + ".jpeg")
	if err != nil {
		t.Errorf("AddFile(): error %s", err)
	}
	fd2.WriteString("JPEG File")
	fd2.Close()

	fd3, err := store.AddFile(id2 + ".mp4")
	if err != nil {
		t.Errorf("AddFile(): error %s", err)
	}
	fd3.WriteString("MP4 File")
	fd3.Close()

	err = store.CommitVideo(id2, time.Now(), "This is another test")
	if err != nil {
		t.Errorf("CommitVideo(): error %s", err)
	}

	album, err := store.GetAlbum("", false)
	if err != nil {
		t.Errorf("GetAlbum(): error %s", err)
	}
	assert.Equal(t, album.Title, "", "current album title is empty")
	assert.Equal(t, len(album.Media), 2, "current album has two media")
	assert.Equal(t, len(album.Media[0].Files), 1, "current album, first media has one file")
	assert.Equal(t, len(album.Media[1].Files), 2, "current album, second media has two files")

	now := time.Now()
	err = store.NewAlbum("My album")
	if err != nil {
		t.Errorf("NewAlbum(): error %s", err)
	}
	err = store.CloseAlbum()
	if err != nil {
		t.Errorf("CloseAlbum(): error %s", err)
	}
	albumId := now.Format("2006-01-02") + "-my-album"
	album, err = store.GetAlbum(albumId, false)
	if err != nil {
		t.Errorf("GetAlbum(): error %s", err)
	}
	assert.Equal(t, album.Title, "My album", "saved album title")
	assert.Equal(t, len(album.Media), 2, "saved album has two media")
	assert.Equal(t, len(album.Media[0].Files), 1, "saved album, first media has one file")
	assert.Equal(t, len(album.Media[1].Files), 2, "saved album, second media has two files")
	assert.Equal(t, album.ID, albumId, "saved album ID")

	albumList, err := store.ListAlbums()
	if err != nil {
		t.Errorf("ListAlbums(): error %s", err)
	}
	assert.Equal(t, len(albumList), 2, "album list has two items")
	assert.Equal(t, albumList[0].ID, "", "album number one is the current album")
	assert.Equal(t, albumList[1].ID, albumId, "album number two is 'My Album'")
}
