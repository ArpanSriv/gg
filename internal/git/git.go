// Package git runs the system git binary.
package git

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Error is a failed git command with its captured stderr.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = e.Err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

// Output runs git in dir, captures its output and returns trimmed stdout.
func Output(dir string, args ...string) (string, error) {
	return OutputInput(dir, "", args...)
}

// OutputInput is Output with stdin set to input.
func OutputInput(dir, input string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if input != "" {
		cmd.Stdin = strings.NewReader(input)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", &Error{Args: args, Stderr: stderr.String(), Err: err}
	}
	return strings.TrimSpace(stdout.String()), nil
}

// Run runs git in dir and shows its output to the user. All output goes to
// stderr so that gg's stdout only carries the final `cd` line.
func Run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// RefExists reports whether ref (a full ref name) points at a commit.
func RefExists(dir, ref string) bool {
	_, err := Output(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	return err == nil
}

// FirstLine returns the first line of err's message, for one-line reports.
func FirstLine(err error) string {
	msg := strings.TrimSpace(err.Error())
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return msg
}
