package source

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// Command starts a child process, streams its combined stdout/stderr,
// and terminates the child process group when ctx is cancelled.
//
// Because logwiz owns the process, it can also restart it on demand
// (see Restart). After the child exits on its own, Command keeps waiting
// for a restart request until ctx is cancelled.
type Command struct {
	Args   []string
	Parser logmsg.Parser

	once      sync.Once
	restartCh chan struct{}
}

// RestartingMsg is sent by a Command source after the previous child has
// been stopped and right before the replacement is spawned.
type RestartingMsg struct{}

// Restarter is implemented by *Command and lets the TUI restart the child
// process at runtime.
type Restarter interface {
	Restart()
}

// stopGrace is how long a child gets to exit after SIGINT before its
// process group is SIGKILLed.
const stopGrace = 2 * time.Second

func (c *Command) init() {
	c.once.Do(func() { c.restartCh = make(chan struct{}, 1) })
}

// Restart asks the running child (if any) to stop and spawns a fresh one.
// Non-blocking; multiple calls before the restart is processed coalesce.
func (c *Command) Restart() {
	c.init()
	select {
	case c.restartCh <- struct{}{}:
	default:
	}
}

func (c *Command) Start(ctx context.Context, sender Sender) {
	c.init()
	go func() {
		if len(c.Args) == 0 {
			sender.Send(ErrMsg{Err: fmt.Errorf("empty command")})
			return
		}
		for {
			// Coalesce restart requests made before/while the previous run
			// was shutting down so a double :restart doesn't kill the new child.
			select {
			case <-c.restartCh:
			default:
			}

			if restart := c.runOnce(ctx, sender); !restart {
				// Child finished on its own (or failed to start); wait for a
				// restart request or shutdown.
				select {
				case <-ctx.Done():
					return
				case <-c.restartCh:
				}
			}
			if ctx.Err() != nil {
				return
			}
			sender.Send(RestartingMsg{})
			sender.Send(NewLinesMsg{Lines: []logmsg.LogMessage{{
				Raw:    "[logwiz: restarting: " + strings.Join(c.Args, " ") + "]",
				Notice: true,
			}}})
		}
	}()
}

// runOnce spawns the child and streams its output until it exits, ctx is
// cancelled, or a restart is requested. It returns true when the run ended
// because of a restart request.
func (c *Command) runOnce(ctx context.Context, sender Sender) (restarted bool) {
	cmd := exec.Command(c.Args[0], c.Args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdin = strings.NewReader("")

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		sender.Send(ErrMsg{Err: err})
		return false
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		sender.Send(ErrMsg{Err: err})
		return false
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	// The watcher stops the child on shutdown or restart. waitDone tells it
	// the child has been reaped; watcherDone lets us read `restarted`
	// without racing it.
	waitDone := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-runCtx.Done():
		case <-c.restartCh:
			restarted = true
			cancelRun()
		case <-waitDone:
			return
		}
		interruptProcessGroup(cmd.Process.Pid)
		select {
		case <-waitDone:
		case <-time.After(stopGrace):
			killProcessGroup(cmd.Process.Pid)
		}
	}()

	pumpErr := pumpLines(runCtx, pipe, sender, c.Parser)
	waitErr := cmd.Wait()
	close(waitDone)
	<-watcherDone

	if restarted || ctx.Err() != nil {
		return restarted
	}
	switch {
	case pumpErr != nil:
		sender.Send(ErrMsg{Err: pumpErr})
	case waitErr != nil:
		sender.Send(ErrMsg{Err: waitErr})
	default:
		sender.Send(EOFMsg{})
	}
	return false
}

func interruptProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGINT)
}

func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
