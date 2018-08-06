package dl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"github.com/pkg/errors"
)

func Download(url, sum string) (string, error) {
	p, err := userCacheDir()
	if err != nil {
		return "", err
	}
	p = filepath.Join(p, "concourse-up")
	err = os.MkdirAll(p, 0700)
	if err != nil {
		return "", err
	}
	p = filepath.Join(p, sum)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if os.IsExist(err) {
		return p, nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	req, err := http.Get(url)
	if err != nil {
		os.Remove(p)
		return "", err
	}
	defer req.Body.Close()
	r, err := newChecksumReader(req.Body, sum)
	if err != nil {
		os.Remove(p)
		return "", err
	}
	_, err = io.Copy(f, r)
	if err != nil {
		os.Remove(p)
		err = errors.Wrap(err, "downloading file")
		return "", err
	}
	err = f.Close()
	if err != nil {
		os.Remove(p)
		return "", err
	}
	return p, nil
}

type checksumReader struct {
	r   io.Reader
	h   hash.Hash
	sum []byte
}

func newChecksumReader(r io.Reader, sum string) (io.Reader, error) {
	h := sha256.New()
	sum_, err := hex.DecodeString(sum)
	if err != nil {
		return nil, err
	}
	return &checksumReader{
		r:   io.TeeReader(r, h),
		h:   h,
		sum: sum_,
	}, nil
}

func (r *checksumReader) Read(p []byte) (n int, err error) {
	n, err = r.r.Read(p)
	if err == io.EOF && bytes.Equal(r.h.Sum(nil), r.sum) == false {
		err = errors.New("validation failed")
		return n, err
	}
	return n, err
}

// UserCacheDir returns the default root directory to use for user-specific
// cached data. Users should create their own application-specific subdirectory
// within this one and use that.
//
// On Unix systems, it returns $XDG_CACHE_HOME as specified by
// https://standards.freedesktop.org/basedir-spec/basedir-spec-latest.html if
// non-empty, else $HOME/.cache.
// On Darwin, it returns $HOME/Library/Caches.
// On Windows, it returns %LocalAppData%.
// On Plan 9, it returns $home/lib/cache.
//
// If the location cannot be determined (for example, $HOME is not defined),
// then it will return an error.
func userCacheDir() (string, error) {
	var dir string

	switch runtime.GOOS {
	case "windows":
		dir = os.Getenv("LocalAppData")
		if dir == "" {
			return "", errors.New("%LocalAppData% is not defined")
		}

	case "darwin":
		dir = os.Getenv("HOME")
		if dir == "" {
			return "", errors.New("$HOME is not defined")
		}
		dir += "/Library/Caches"

	case "plan9":
		dir = os.Getenv("home")
		if dir == "" {
			return "", errors.New("$home is not defined")
		}
		dir += "/lib/cache"

	default: // Unix
		dir = os.Getenv("XDG_CACHE_HOME")
		if dir == "" {
			dir = os.Getenv("HOME")
			if dir == "" {
				return "", errors.New("neither $XDG_CACHE_HOME nor $HOME are defined")
			}
			dir += "/.cache"
		}
	}

	return dir, nil
}
