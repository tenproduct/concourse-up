package dl_test

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/EngineerBetter/concourse-up/dl"
)

func TestDownload_success(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "fly_linux_amd64", time.Now(), strings.NewReader("not so fly"))
	}))
	defer s.Close()
	p, err := dl.Download(s.URL, "F842D6D8B2DF801F00E4417B69F0742C4F2140285EEEC38FCA99FFC529F602A5")
	defer os.Remove(p)
	require.NoError(t, err)
	fi, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0700), fi.Mode().Perm())
	data, err := ioutil.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "not so fly", string(data))
}

func TestDownload_caches_files(t *testing.T) {
	var count int
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "fly_linux_amd64", time.Now(), strings.NewReader("not so fly"))
		count++
	}))
	defer s.Close()
	p1, err := dl.Download(s.URL, "F842D6D8B2DF801F00E4417B69F0742C4F2140285EEEC38FCA99FFC529F602A5")
	defer os.Remove(p1)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	p2, err := dl.Download(s.URL, "F842D6D8B2DF801F00E4417B69F0742C4F2140285EEEC38FCA99FFC529F602A5")
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Equal(t, p1, p2)
}

func TestDownload_bad_checksum(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "fly_linux_amd64", time.Now(), strings.NewReader("not so fly"))
	}))
	defer s.Close()
	_, err := dl.Download(s.URL, "badf")
	require.EqualError(t, err, "downloading file: validation failed")
}
