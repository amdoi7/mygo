package terminal

import (
	"bytes"
	"errors"
	"net"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/plugins/terminal/internal/pty"
)

// Contract: Piculet docs/design/35-execution-cards.md.
func TestTaskTerminalRawPTYAndInteractiveInput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh; ConPTY is cross-compiled")
	}
	loadLib(t)
	ready := make(chan struct{})
	var raw []byte
	term, err := New(Options{
		Command:  []string{"/bin/sh", "-c", "stty -echo; test -t 0 && test -t 1 && printf '\033[31mtty\033[0m\033]0;ready\007'; IFS= read -r value; printf '\ninput=%s\n' \"$value\"; exit 7"},
		OnOutput: func(data []byte) { raw = append(raw, data...) },
		OnTitle: func(title string) {
			if title == "ready" {
				close(ready)
			}
		},
	})
	if err != nil {
		t.Fatalf("new PTY: %v", err)
	}
	t.Cleanup(func() { _ = term.Close() })
	term.Resize(80, 24)
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("program did not see a TTY")
	}
	term.Send([]byte("world\r"))
	select {
	case <-term.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("interactive input did not settle")
	}
	if term.ExitCode() != 7 || term.Err() != nil || !bytes.Contains(raw, []byte("\x1b[31mtty\x1b[0m")) || !strings.Contains(term.Text(), "input=world") {
		t.Fatalf("code=%d err=%v raw=%q screen=%q", term.ExitCode(), term.Err(), raw, term.Text())
	}
	before := append([]byte(nil), raw...)
	term.Feed([]byte("manual"))
	if !bytes.Equal(raw, before) {
		t.Fatal("Feed was mistaken for process output")
	}
}

func TestTaskTerminalReportsOutputReadFailure(t *testing.T) {
	loadLib(t)
	want := errors.New("task connection failed")
	term, err := New(Options{Conn: taskFailedConn{err: want}})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	t.Cleanup(func() { _ = term.Close() })
	select {
	case <-term.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("failed read did not settle")
	}
	if !errors.Is(term.Err(), want) {
		t.Fatalf("error=%v, want %v", term.Err(), want)
	}
}

func TestTaskTerminalStopRetainsScreen(t *testing.T) {
	loadLib(t)
	reader, writer := net.Pipe()
	ready := make(chan struct{})
	term, err := New(Options{Conn: reader, OnTitle: func(title string) {
		if title == "ready" {
			close(ready)
		}
	}})
	if err != nil {
		t.Fatalf("new connection: %v", err)
	}
	t.Cleanup(func() { _ = writer.Close(); _ = term.Close() })
	if _, err := writer.Write([]byte("retained\x1b]0;ready\a")); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("screen not ready")
	}
	if err := term.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case <-term.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not settle")
	}
	if !strings.Contains(term.Text(), "retained") || len(term.Snapshot()) == 0 {
		t.Fatalf("screen was destroyed: %q", term.Text())
	}
	if err := term.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if term.Text() != "" || len(term.Snapshot()) != 0 {
		t.Fatal("close retained screen")
	}
	if err := term.Close(); err != nil {
		t.Fatalf("repeat close: %v", err)
	}
}

func TestTaskTerminalRejectsScheduledLaunchAfterStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	launch := &pty.Options{Path: "/bin/sh", Args: []string{"sh", "-c", "exit 0"}}
	term := &Terminal{done: make(chan struct{}), launch: launch}
	if err := term.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	term.start(launch)
	if term.proc != nil {
		_ = term.proc.Kill()
		t.Fatal("stopped terminal launched a process")
	}
	select {
	case <-term.Done():
	default:
		t.Fatal("pending launch did not settle")
	}
	if term.ExitCode() != -1 {
		t.Fatalf("exit=%d, want -1", term.ExitCode())
	}
}

func TestTaskTerminalStartupFailureAndConcurrentClose(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	loadLib(t)
	t.Run("startup_failure", func(t *testing.T) {
		term, err := New(Options{Command: []string{"/bin/sh", "-c", "exit 0"}, Dir: t.TempDir() + "/missing"})
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		t.Cleanup(func() { _ = term.Close() })
		term.Resize(80, 24)
		select {
		case <-term.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("failed launch did not settle")
		}
		if term.ExitCode() != -1 || !os.IsNotExist(term.Err()) {
			t.Fatalf("exit=%d err=%v", term.ExitCode(), term.Err())
		}
	})
	t.Run("resize_stop_close", func(t *testing.T) {
		term, err := New(Options{Command: []string{"/bin/sh", "-c", "exec sleep 30"}})
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		t.Cleanup(func() { _ = term.Close() })
		var group sync.WaitGroup
		group.Go(func() { term.Resize(90, 30) })
		group.Go(func() { _ = term.Stop() })
		group.Go(func() { _ = term.Close() })
		group.Wait()
		select {
		case <-term.Done():
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent close did not settle")
		}
	})
}

type taskFailedConn struct{ err error }

func (c taskFailedConn) Read([]byte) (int, error)    { return 0, c.err }
func (c taskFailedConn) Write(p []byte) (int, error) { return len(p), nil }
func (c taskFailedConn) Close() error                { return nil }
