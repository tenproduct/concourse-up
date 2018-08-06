package ziputil

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
)

// Unzip extracts the contents of zipData into a directory
func Unzip(dir string, zipData []byte) error {
	r := bytes.NewReader(zipData)
	zr, err := zip.NewReader(r, r.Size())
	if err != nil {
		return err
	}
	for _, f := range zr.File {
		err = func(f *zip.File) error {
			fullPath := filepath.Join(dir, f.Name)
			if f.FileInfo().IsDir() {
				return os.MkdirAll(fullPath, 0700)
			}
			var fr io.ReadCloser
			fr, err = f.Open()
			if err != nil {
				return err
			}
			defer func() { _ = fr.Close() }()
			var tf io.WriteCloser
			tf, err = os.OpenFile(fullPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, f.Mode())
			if err != nil {
				return err
			}
			defer func() { _ = tf.Close() }()
			_, err = io.Copy(tf, fr)
			if err != nil {
				return err
			}
			return tf.Close()
		}(f)
		if err != nil {
			return err
		}
	}
	return nil
}
