//go:build e2e

package e2e

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

type TUIDriver struct {
	cmd  *exec.Cmd
	ptmx *os.File

	mu     sync.Mutex
	buf    bytes.Buffer
	closed bool

	done chan error
}

func StartTUI(t *testing.T, bin string, env []string, width, height int) *TUIDriver {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Env = env
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
	if err != nil {
		t.Fatalf("start TUI under PTY: %v", err)
	}
	d := &TUIDriver{cmd: cmd, ptmx: ptmx, done: make(chan error, 1)}
	go d.readLoop()
	go func() { d.done <- cmd.Wait() }()
	return d
}

func (d *TUIDriver) readLoop() {
	tmp := make([]byte, 4096)
	for {
		n, err := d.ptmx.Read(tmp)
		if n > 0 {
			d.mu.Lock()
			_, _ = d.buf.Write(tmp[:n])
			d.mu.Unlock()
		}
		if err != nil {
			if err != io.EOF {
				return
			}
			return
		}
	}
}

func (d *TUIDriver) Send(t *testing.T, s string) {
	t.Helper()
	if _, err := d.ptmx.WriteString(s); err != nil {
		t.Fatalf("send to TUI: %v", err)
	}
}

func (d *TUIDriver) SendKeys(t *testing.T, keys ...string) {
	t.Helper()
	for _, key := range keys {
		d.Send(t, key)
		time.Sleep(75 * time.Millisecond)
	}
}

func (d *TUIDriver) Resize(t *testing.T, width, height int) {
	t.Helper()
	if err := pty.Setsize(d.ptmx, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)}); err != nil {
		t.Fatalf("resize TUI: %v", err)
	}
}

func (d *TUIDriver) Screen() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return stripANSI(d.buf.String())
}

func (d *TUIDriver) WaitForText(t *testing.T, text string) {
	t.Helper()
	AssertEventually(t, defaultTimeout, func() bool {
		return strings.Contains(d.Screen(), text)
	}, func() string {
		return "screen did not contain " + text + ":\n" + d.Screen()
	})
}

func (d *TUIDriver) AssertStillRunning(t *testing.T) {
	t.Helper()
	select {
	case err := <-d.done:
		t.Fatalf("TUI exited unexpectedly: %v\nscreen:\n%s", err, d.Screen())
	default:
	}
}

func (d *TUIDriver) Close(t *testing.T) {
	t.Helper()
	if d == nil || d.cmd == nil || d.cmd.Process == nil {
		return
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	d.mu.Unlock()
	select {
	case <-d.done:
		_ = d.ptmx.Close()
		return
	default:
	}
	_, _ = d.ptmx.WriteString("q")
	select {
	case <-d.done:
		_ = d.ptmx.Close()
		return
	case <-time.After(1500 * time.Millisecond):
		_ = d.cmd.Process.Kill()
		<-d.done
		_ = d.ptmx.Close()
	}
}

var ansiRE = regexp.MustCompile(`\x1b(?:[@-Z\\-_]|\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\))`)

func stripANSI(s string) string {
	return ansiRE.ReplaceAllString(s, "")
}
