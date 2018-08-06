package tf

import "os/exec"

func FakeExec(f func(command string, args ...string) *exec.Cmd) Option {
	return func(c *Client) error {
		c.exec = f
		return nil
	}
}
