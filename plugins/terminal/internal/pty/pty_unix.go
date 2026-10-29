//go:build darwin || linux

package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type sys struct {
	killMu sync.Mutex
	master *os.File
	cmd    *exec.Cmd
}

func (p *PTY) start(o Options) error {
	master, slaveName, err := openMaster()
	if err != nil {
		return err
	}
	slave, err := os.OpenFile(slaveName, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return fmt.Errorf("pty: opening %s: %w", slaveName, err)
	}
	defer slave.Close() // the child has its own copies
	if err := setSize(master, o.Cols, o.Rows, o.Width, o.Height); err != nil {
		master.Close()
		return err
	}
	cmd := &exec.Cmd{Path: o.Path, Args: o.Args, Dir: o.Dir, Env: o.Env, Stdin: slave, Stdout: slave, Stderr: slave}
	// A session of its own, whose controlling terminal is the slave (the
	// child's descriptor 0), so that the terminal's signals reach the
	// process group in the foreground.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		master.Close()
		return err
	}
	p.master, p.cmd = master, cmd
	return nil
}

func (p *PTY) wait() (int, error) {
	err := p.cmd.Wait()
	cleanupErr := p.waitSession()
	if e, ok := errors.AsType[*exec.ExitError](err); ok {
		if ws, ok := e.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), cleanupErr
		}
		return e.ExitCode(), cleanupErr
	}
	return 0, errors.Join(err, cleanupErr)
}

// Read reads the output of the process; it returns io.EOF once no process
// has the terminal open any more, or after Close.
func (p *PTY) Read(b []byte) (int, error) {
	n, err := p.master.Read(b)
	if err != nil && n == 0 {
		// Linux reports EIO when the last process with the terminal
		// open closes it.
		if errors.Is(err, syscall.EIO) || errors.Is(err, os.ErrClosed) {
			err = io.EOF
		}
	}
	return n, err
}

// Write types b into the terminal.
func (p *PTY) Write(b []byte) (int, error) {
	n, err := p.master.Write(b)
	if errors.Is(err, os.ErrClosed) {
		err = ErrClosed
	}
	return n, err
}

// Resize sets the size of the terminal, which sends the process SIGWINCH.
func (p *PTY) Resize(cols, rows, width, height int) error {
	return setSize(p.master, cols, rows, width, height)
}

// ClearScreen does nothing: the terminal has no screen of its own to
// clear, unlike ConPTY's console.
func (p *PTY) ClearScreen() error { return nil }

// Pid returns the process's ID.
func (p *PTY) Pid() int { return p.cmd.Process.Pid }

// Close closes the terminal, which hangs up the processes that have it:
// they get SIGHUP. It does not wait for them to exit.
func (p *PTY) Close() error { return p.master.Close() }

// Kill requests termination of live session processes, including job-control groups.
func (p *PTY) Kill() error {
	_, err := p.killSession()
	return err
}

func (p *PTY) waitSession() error {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		live, err := p.killSession()
		if err != nil || live == 0 {
			return err
		}
		<-tick.C
	}
}

func (p *PTY) killSession() (int, error) {
	p.killMu.Lock()
	defer p.killMu.Unlock()
	data, err := exec.Command("/bin/ps", "-axo", "pid=,stat=").Output()
	if err != nil {
		return 0, fmt.Errorf("pty: enumerate session: %w", err)
	}
	live := 0
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 {
			return live, fmt.Errorf("pty: invalid process record %q", line)
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			return live, fmt.Errorf("pty: invalid process ID: %w", err)
		}
		if strings.Contains(fields[1], "Z") {
			continue
		}
		sid, _, errno := syscall.Syscall(syscall.SYS_GETSID, uintptr(pid), 0, 0)
		if errno == syscall.ESRCH {
			continue
		}
		if errno != 0 {
			return live, fmt.Errorf("pty: session of process %d: %w", pid, errno)
		}
		if int(sid) != p.cmd.Process.Pid {
			continue
		}
		live++
		if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return live, fmt.Errorf("pty: kill session process %d: %w", pid, err)
		}
	}
	return live, nil
}

func setSize(f *os.File, cols, rows, width, height int) error {
	ws := struct{ row, col, xpixel, ypixel uint16 }{clamp(rows), clamp(cols), clamp(width), clamp(height)}
	return ioctl(f, syscall.TIOCSWINSZ, unsafe.Pointer(&ws))
}

func clamp(v int) uint16 { return uint16(max(0, min(v, 0xFFFF))) }

// ioctl calls ioctl on f with a pointer, which it converts in the system
// call, so that what it points to stays put meanwhile.
func ioctl(f *os.File, req uintptr, arg unsafe.Pointer) error {
	c, err := f.SyscallConn()
	if err != nil {
		return err
	}
	var errno syscall.Errno
	err = c.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg))
	})
	if err != nil {
		return err
	}
	if errno != 0 {
		return errno
	}
	return nil
}
