package tf_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/EngineerBetter/concourse-up/internal/fakeexec"
	"github.com/EngineerBetter/concourse-up/tf"
)

func TestExecCommandHelper(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	fmt.Print(os.Getenv("STDOUT"))
	i, _ := strconv.Atoi(os.Getenv("EXIT_STATUS"))
	os.Exit(i)
}

func TestClient_Apply_parses_output(t *testing.T) {
	e := fakeexec.New(t)
	defer e.Finish()

	e.Expect("terraform", "apply", "-input=false", "-auto-approve")
	e.Expect("terraform", "output", "--json").Outputs(`{
		"foo": {
			"sensitive": false,
			"type": "string",
			"value": "bar"
		},
		"baz": {
			"sensitive": false,
			"type": "list",
			"value": [
				"ほげ",
				"ぴよ"
			]
		},
		"spam": {
			"sensitive": false,
			"type": "map",
			"value": {
				"ham": "ham",
				"eggs": "eggs"
			}
		}
	}`)

	c, err := tf.New(tf.FakeExec(e.Cmd()))
	require.NoError(t, err)
	out, err := c.Apply(nil)
	require.NoError(t, err)

	require.Equal(t, tf.OutputTypeString, out["foo"].Type())
	s, err := out["foo"].String()
	require.NoError(t, err)
	require.Equal(t, "bar", s)

	require.Equal(t, tf.OutputTypeList, out["baz"].Type())
	l, err := out["baz"].List()
	require.NoError(t, err)
	require.Equal(t, []string{"ほげ", "ぴよ"}, l)

	require.Equal(t, tf.OutputTypeMap, out["spam"].Type())
	m, err := out["spam"].Map()
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"ham":  "ham",
		"eggs": "eggs",
	}, m)
}
func TestClient_Apply_with_vars(t *testing.T) {
	e := fakeexec.New(t)
	defer e.Finish()

	e.Expect("terraform", "apply", "-input=false", "-auto-approve", "-var=foo=bar")
	e.Expect("terraform", "output", "--json").Outputs(`{}`)

	c, err := tf.New(tf.FakeExec(e.Cmd()))
	require.NoError(t, err)
	_, err = c.Apply(map[string]string{"foo": "bar"})
	require.NoError(t, err)
}
func TestClient_Destroy(t *testing.T) {
	e := fakeexec.New(t)
	defer e.Finish()

	e.Expect("terraform", "destroy", "-auto-approve")

	c, err := tf.New(tf.FakeExec(e.Cmd()))
	require.NoError(t, err)
	err = c.Destroy()
	require.NoError(t, err)
}

func TestClient_FromURL(t *testing.T) {
	const fakeTerraform = `
#!/bin/bash
exit 1
`
	fakeTerraformSum := str2sum(fakeTerraform)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "terraform", time.Now(), strings.NewReader(fakeTerraform))
	}))
	defer s.Close()
	c, err := tf.New(
		tf.FromURL(s.URL, fakeTerraformSum),
	)
	require.NoError(t, err)
	defer c.Close()
	_ = c
}

func str2sum(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

//go:generate bash -c "cd testdata/tf && zip --filesync -r ../tf.zip *"
func TestClient_Conf(t *testing.T) {
	zipData, err := ioutil.ReadFile("testdata/tf.zip")
	require.NoError(t, err)
	c, err := tf.New(
		tf.Conf(zipData),
	)
	require.NoError(t, err)
	defer c.Close()
	_ = c
}
