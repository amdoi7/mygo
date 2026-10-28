package terminal

import (
	"bytes"
	"io"
	"net"
	"runtime"
	"strings"
	"testing"

	"github.com/egoist/mygo/internal/wait"
)

func TestOnOutputReceivesFinalBytesReturnedWithEOF(t *testing.T) {
	loadLib(t)
	var output []byte
	term, err := New(Options{Conn: outputEOFConn{}, OnOutput: func(data []byte) {
		output = append(output, data...)
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = term.Close() })
	wait.For(t, "the final output", term.Done())
	if got, want := string(output), "final"; got != want {
		t.Fatalf("raw output=%q, want %q", got, want)
	}
	if text := term.Text(); !strings.Contains(text, "final") {
		t.Fatalf("screen=%q, want final output", text)
	}
}

func TestOnOutputReceivesRawProgramBytes(t *testing.T) {
	loadLib(t)
	command := []string{"/bin/sh", "-c", "printf 'terminal-output'"}
	if runtime.GOOS == "windows" {
		command = []string{"cmd.exe", "/c", "echo terminal-output"}
	}
	var output []byte
	term, err := New(Options{Command: command, OnOutput: func(data []byte) {
		output = append(output, data...)
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = term.Close() })
	term.Resize(80, 24)
	wait.For(t, "the program to finish", term.Done())
	if !bytes.Contains(output, []byte("terminal-output")) || !strings.Contains(term.Text(), "terminal-output") {
		t.Fatalf("raw output=%q, screen=%q", output, term.Text())
	}
}

func TestOnOutputReceivesRawConnectionBytes(t *testing.T) {
	loadLib(t)
	reader, writer := net.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	var output []byte
	term, err := New(Options{Conn: reader, OnOutput: func(data []byte) {
		output = append(output, data...)
	}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = term.Close() })
	term.Feed([]byte("local\r\n"))
	want := []byte("\x1b[31mremote\x1b[0m\r\n")
	if _, err := writer.Write(want); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	wait.For(t, "the connection to finish", term.Done())
	if !bytes.Equal(output, want) {
		t.Fatalf("raw output=%q, want %q", output, want)
	}
	if text := term.Text(); !strings.Contains(text, "local") || !strings.Contains(text, "remote") {
		t.Fatalf("screen=%q, want local and remote text", text)
	}
}

type outputEOFConn struct{}

func (outputEOFConn) Read(data []byte) (int, error)  { return copy(data, "final"), io.EOF }
func (outputEOFConn) Write(data []byte) (int, error) { return len(data), nil }
func (outputEOFConn) Close() error                   { return nil }
