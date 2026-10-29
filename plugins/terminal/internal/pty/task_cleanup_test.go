//go:build darwin || linux

package pty

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// Contract: Piculet docs/design/35-execution-cards.md, managed session cleanup.
func TestTaskWaitCleansManagedDescendants(t *testing.T) {
	for _, mode := range []string{"process_group", "job_control"} {
		t.Run(mode, func(t *testing.T) {
			release := filepath.Join(t.TempDir(), "release")
			script := "/bin/sh -c 'trap \"\" HUP; printf \"child=%s\\n\" \"$$\"; exec sleep 30' & while [ ! -f \"$1\" ]; do :; done; exit 0"
			if mode == "job_control" {
				script = "set -m; " + script
			}
			p, err := Start(Options{Path: "/bin/sh", Args: []string{"sh", "-c", script, "sh", release}})
			if err != nil {
				t.Fatalf("start: %v", err)
			}
			reader := bufio.NewReader(p)
			drained := make(chan error, 1)
			var child atomic.Int64
			t.Cleanup(func() {
				_ = p.Kill()
				_ = p.Close()
				if pid := child.Load(); pid > 0 {
					_ = syscall.Kill(int(pid), syscall.SIGKILL)
				}
				<-drained
			})
			go func() {
				defer close(drained)
				line, err := reader.ReadString('\n')
				if err == nil && !strings.HasPrefix(line, "child=") {
					t.Errorf("readiness=%q", line)
				}
				if err == nil {
					var pid int64
					pid, err = strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(line, "child=")), 10, 64)
					child.Store(pid)
				}
				if err == nil {
					err = os.WriteFile(release, nil, 0o600)
				}
				if err == nil {
					_, err = p.Wait()
				}
				if err == nil {
					_, err = io.Copy(io.Discard, reader)
				}
				drained <- err
			}()
			select {
			case err := <-drained:
				if err != nil {
					t.Fatalf("drain: %v", err)
				}
				state, err := exec.Command("/bin/ps", "-p", strconv.FormatInt(child.Load(), 10), "-o", "stat=").Output()
				if strings.ContainsAny(string(state), "SRDTE") {
					t.Fatalf("wait returned with live child: %q", state)
				}
				if err != nil {
					if exited, ok := err.(*exec.ExitError); !ok || exited.ExitCode() != 1 {
						t.Fatalf("child state: %v", err)
					}
				}
			case <-time.After(5 * time.Second):
				t.Fatal("leader exited but a managed descendant retained the PTY")
			}
		})
	}
}
