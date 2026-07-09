package deploy

import (
	"os/exec"
	"syscall"
	"time"
)

// killGrace bounds how long Wait blocks on lingering pipe holders after the
// process group is killed on context cancellation.
const killGrace = 10 * time.Second

// configureProcessGroup makes cmd the leader of a new process group and, when
// its context is cancelled (deploy timeout or deploq shutdown), SIGKILLs the
// WHOLE group instead of just the direct child.
//
// Without this, cancelling `sh -c "./rolling-deploy.sh"` kills only the `sh`;
// the script and the `docker build` / `docker compose` it spawned are reparented
// to init and keep running detached. deploq then thinks the deploy failed,
// releases the lock, and the next webhook starts a SECOND build that races the
// orphan — two concurrent compiles fighting for CPU (the load-15 incident).
// Killing the process group tears the whole tree down so a timeout can't leak a
// zombie build.
//
// WaitDelay ensures cmd.Wait returns promptly even if a descendant still holds
// the stdout/stderr pipe after the kill.
func configureProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID = the whole process group led by this child.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = killGrace
}
