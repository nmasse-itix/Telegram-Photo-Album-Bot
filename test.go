package main

import (
	"io/ioutil"
	"os"
	"testing"
)

type TestCaseTempFile struct {
	RootDir string
}

func createTempDir(t *testing.T) TestCaseTempFile {
	tmp := os.TempDir()
	dir, err := ioutil.TempDir(tmp, "unittest-*")
	if err != nil {
		t.Errorf("newTmpDir: ioutil.TempDir: %s", err)
	}
	return TestCaseTempFile{RootDir: dir}
}

func (tmp *TestCaseTempFile) cleanup(t *testing.T) {
	//os.RemoveAll(tmp.RootDir)
}
