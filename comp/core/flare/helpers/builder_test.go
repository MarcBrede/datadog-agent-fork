// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helpers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	"github.com/DataDog/datadog-agent/pkg/util/archive"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/hostname/validate"
)

var FromSlash = filepath.FromSlash

func setupDirWithData(t *testing.T) string {
	root := filepath.Join(t.TempDir(), "root")
	require.NoError(t, os.MkdirAll(filepath.Join(root, "depth1", "depth2"), os.ModePerm))

	f1 := filepath.Join(root, "test1")
	f2 := filepath.Join(root, "test2")
	f3 := filepath.Join(root, "depth1", "test3")
	f4 := filepath.Join(root, "depth1", "depth2", "test4")

	require.NoError(t, os.WriteFile(f1, []byte("some data"), os.ModePerm))
	require.NoError(t, os.WriteFile(f2, []byte("some data\napi_key: 123456789006789009"), os.ModePerm))
	require.NoError(t, os.WriteFile(f3, []byte("some data"), os.ModePerm))
	require.NoError(t, os.WriteFile(f4, []byte("some data"), os.ModePerm))

	return root
}

func assertFileContent(t *testing.T, fb *builder, expected string, path string) {
	path = filepath.Join(fb.flareDir, FromSlash(path))

	require.FileExists(t, path)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, expected, string(content))
}

func getNewBuilder(t *testing.T) *builder {
	f, err := NewFlareBuilder(false, flarebuilder.FlareArgs{})
	require.NotNil(t, f)
	require.NoError(t, err)

	fb, success := f.(*builder)
	require.True(t, success, "FlareBuilder returned by FlareBuilder is not a *builder type")
	return fb
}

func TestNewFlareBuilder(t *testing.T) {
	fb := getNewBuilder(t)

	require.DirExists(t, fb.tmpDir)
	require.DirExists(t, fb.flareDir)
	require.FileExists(t, filepath.Join(fb.flareDir, "flare_creation.log"))

	archive, err := fb.Save()
	assert.NoError(t, err)
	assert.FileExists(t, archive)
	os.RemoveAll(archive)

	assert.NoDirExists(t, fb.tmpDir)
	assert.NoDirExists(t, fb.flareDir)
}

func TestSave(t *testing.T) {
	fb := getNewBuilder(t)

	root := setupDirWithData(t)
	fb.CopyDirTo(root, "test", func(string) bool { return true })
	fb.AddFile("test.data", []byte("some data"))

	archivePath, err := fb.Save()
	require.NoError(t, err)
	assert.NoDirExists(t, fb.tmpDir)
	require.FileExists(t, archivePath)

	defer os.RemoveAll(archivePath)

	tmpDir := t.TempDir()

	hostname, err := hostname.Get(context.TODO())
	if err != nil {
		hostname = "unknown"
	}
	hostname = validate.CleanHostnameDir(hostname)

	err = archive.Unzip(archivePath, tmpDir)
	assert.Nil(t, err)
	assert.FileExists(t, filepath.Join(tmpDir, hostname, "test.data"))
	assert.FileExists(t, filepath.Join(tmpDir, hostname, "test/depth1/depth2/test4"))
}

func TestAddFileFromFunc(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	fb.AddFileFromFunc(FromSlash("test/AddFileFromFunc"), func() ([]byte, error) {
		return []byte("some data"), nil
	})
	assertFileContent(t, fb, "some data", "test/AddFileFromFunc")

	fb.AddFileFromFunc(FromSlash("test/AddFileFromFunc_nil"), func() ([]byte, error) {
		return nil, nil
	})
	assertFileContent(t, fb, "", "test/AddFileFromFunc_nil")

	err := fb.AddFileFromFunc(FromSlash("test/AddFileFromFunc_error"), func() ([]byte, error) {
		return nil, fmt.Errorf("some error")
	})
	assert.Error(t, err)
	assert.Equal(t, FromSlash("error collecting data for 'test/AddFileFromFunc_error': some error"), err.Error())
	assert.NoFileExists(t, filepath.Join(fb.flareDir, "test", "AddFileFromFunc_error"))
}

func TestAddFile(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	fb.AddFile(FromSlash("test/AddFile"), []byte("some data"))
	assertFileContent(t, fb, "some data", "test/AddFile")

	fb.AddFile(FromSlash("test/AddFile_scrubbed_api_key"), []byte("api_key : 123456789006789009"))
	assertFileContent(t, fb, "api_key: \"********\"", "test/AddFile_scrubbed_api_key")
}

func TestAddNonLocalFileFlare(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	expectedError := "the destination path is not local to the flare root path"

	err := fb.AddFile(FromSlash("../test/AddFile"), []byte{})
	assert.ErrorContains(t, err, expectedError)

	err = fb.AddFileWithoutScrubbing(FromSlash("../test/AddFile"), []byte{})
	assert.ErrorContains(t, err, expectedError)

	err = fb.AddFileFromFunc(FromSlash("../test/AddFile"), func() ([]byte, error) { return []byte{}, nil })
	assert.ErrorContains(t, err, expectedError)

	path := filepath.Join(t.TempDir(), "test.data")
	os.WriteFile(path, []byte("some data"), os.ModePerm)
	err = fb.CopyFileTo(path, FromSlash("../test/AddFile"))
	assert.ErrorContains(t, err, expectedError)

	root := setupDirWithData(t)
	err = fb.CopyDirTo(root, "../test", func(string) bool { return true })
	assert.ErrorContains(t, err, expectedError)

	err = fb.CopyDirToWithoutScrubbing(root, "../test", func(string) bool { return true })
	assert.ErrorContains(t, err, expectedError)

	_, err = fb.PrepareFilePath("../test")
	assert.ErrorContains(t, err, expectedError)
}

func TestAddFileWithoutScrubbing(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	fb.AddFileWithoutScrubbing(FromSlash("test/AddFile"), []byte("some data"))
	assertFileContent(t, fb, "some data", "test/AddFile")

	fb.AddFileWithoutScrubbing(FromSlash("test/AddFile_scrubbed_api_key"), []byte("api_key: 123456789006789009"))
	assertFileContent(t, fb, "api_key: 123456789006789009", "test/AddFile_scrubbed_api_key")
}

// Test that writeScrubbedFile actually scrubs third-party API keys.
func TestRedactingOtherServicesApiKey(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	clear := []byte(`init_config:
instances:
- host: 127.0.0.1
  api_key: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  port: 8082
  api_key: dGhpc2++lzM+XBhc3N3b3JkW113aXRo/c29tZWN]oYXJzMTIzCg==
  version: 4 # omit this line if you're running pdns_recursor version 3.x`)
	redacted := `init_config:
instances:
- host: 127.0.0.1
  api_key: "***************************aaaaa"
  port: 8082
  api_key: "********"
  version: 4 # omit this line if you're running pdns_recursor version 3.x`

	fb.AddFile("test.conf", clear)
	assertFileContent(t, fb, redacted, "test.conf")
}

func TestAddFileYamlDetection(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	clear := []byte(`instances:
- host: 127.0.0.1
  token:
    - abcdef
    - abcdef
    - abcdef`)
	redacted := `instances:
  - host: 127.0.0.1
    token: "********"`

	fb.AddFile("test.yaml", clear)
	assertFileContent(t, fb, redacted, "test.yaml")
}

func TestCopyFileTo(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	path := filepath.Join(t.TempDir(), "test.data")
	os.WriteFile(path, []byte("some data"), os.ModePerm)

	assert.NoError(t, fb.CopyFileTo(path, FromSlash("test/copy/test.data")))
	assertFileContent(t, fb, "some data", "test/copy/test.data")
	assert.NoError(t, fb.CopyFileTo(path, FromSlash("test/copy2/new.data")))
	assertFileContent(t, fb, "some data", "test/copy2/new.data")
	assert.NoError(t, fb.CopyFileTo(path, FromSlash("new.data")))
	assertFileContent(t, fb, "some data", "new.data")
}

func TestCopyFile(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	path := filepath.Join(t.TempDir(), "test.data")
	os.WriteFile(path, []byte("some data"), os.ModePerm)

	assert.NoError(t, fb.CopyFile(path))
	assertFileContent(t, fb, "some data", "test.data")
}

func TestCopyDirTo(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	root := setupDirWithData(t)

	require.NoError(t, fb.CopyDirTo(
		root,
		"test",
		func(f string) bool {
			return filepath.Base(f) != "test3"
		},
	))

	assertFileContent(t, fb, "some data", filepath.Join("test", "test1"))
	assertFileContent(t, fb, "some data\napi_key: \"********\"", filepath.Join("test", "test2"))
	assert.NoFileExists(t, filepath.Join(fb.flareDir, "test", "depth1", "test3"))
	assertFileContent(t, fb, "some data", filepath.Join("test", "depth1", "depth2", "test4"))
}

func TestCopyDirToWithoutScrubbing(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	root := setupDirWithData(t)

	require.NoError(t, fb.CopyDirToWithoutScrubbing(
		root,
		"test",
		func(f string) bool {
			return filepath.Base(f) != "test3"
		},
	))

	assertFileContent(t, fb, "some data", filepath.Join("test", "test1"))
	assertFileContent(t, fb, "some data\napi_key: 123456789006789009", filepath.Join("test", "test2"))
	assert.NoFileExists(t, filepath.Join(fb.flareDir, "test", "depth1", "test3"))
	assertFileContent(t, fb, "some data", filepath.Join("test", "depth1", "depth2", "test4"))
}

func TestCopyFileYamlDetection(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	input := []byte(`instances:
- host: 127.0.0.1
  token:
    - abcdef
    - abcdef
    - abcdef`)
	redacted := `instances:
  - host: 127.0.0.1
    token: "********"`

	path1 := filepath.Join(t.TempDir(), "test.yaml")
	os.WriteFile(path1, []byte(input), os.ModePerm)
	path2 := filepath.Join(t.TempDir(), "test.data")
	os.WriteFile(path2, []byte(input), os.ModePerm)

	assert.NoError(t, fb.CopyFile(path1))
	assertFileContent(t, fb, redacted, "test.yaml")

	assert.NoError(t, fb.CopyFileTo(path2, "test2.yaml"))
	assertFileContent(t, fb, redacted, "test2.yaml")
}

func TestPrepareFilePath(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	fb.PrepareFilePath("a")
	assert.NoDirExists(t, filepath.Join(fb.flareDir, "a"))

	fb.PrepareFilePath(FromSlash("a/"))
	assert.NoDirExists(t, filepath.Join(fb.flareDir, "a"))

	fb.PrepareFilePath(FromSlash("a/file"))
	assert.DirExists(t, filepath.Join(fb.flareDir, "a"))

	fb.PrepareFilePath(FromSlash("a/b/c/d/file"))
	assert.DirExists(t, filepath.Join(fb.flareDir, "a", "b", "c", "d"))
}

func TestRegisterDirPerm(t *testing.T) {
	fb := getNewBuilder(t)
	defer fb.clean()

	root := setupDirWithData(t)

	fb.RegisterDirPerm(root)

	expectedPaths := []string{
		filepath.Join(root),
		filepath.Join(root, "test1"),
		filepath.Join(root, "test2"),
		filepath.Join(root, "depth1"),
		filepath.Join(root, "depth1", "test3"),
		filepath.Join(root, "depth1", "depth2"),
		filepath.Join(root, "depth1", "depth2", "test4"),
	}

	require.Len(t, fb.permsInfos, len(expectedPaths))
	for _, path := range expectedPaths {
		assert.Contains(t, fb.permsInfos, path)
	}
}
