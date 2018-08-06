package ziputil_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/EngineerBetter/concourse-up/tf/internal/ziputil"

	"github.com/stretchr/testify/require"
)

//go:generate bash -c "cd testdata/foo && zip --filesync -r ../foo.zip *"

func TestUnzip_success(t *testing.T) {
	zipData, err := ioutil.ReadFile("testdata/foo.zip")
	require.NoError(t, err)
	dst, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(dst)
	err = ziputil.Unzip(dst, zipData)
	require.NoError(t, err)
	require.FileExists(t, filepath.Join(dst, "bar"))
	require.DirExists(t, filepath.Join(dst, "baz"))
}
