package source

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// Command starts a child process, streams its combined stdout/stderr,
// and terminates the child process group when ctx is cancelled.
type Command struct {
	Args   []string
	Parser logmsg.Parser
}

func (c *Command) Start(ctx context.Context, sender Sender) {
	go func() {
		if len(c.Args) == 0 {
			sender.Send(ErrMsg{Err: fmt.Errorf("empty command")})
			return
		}

		cmd := exec.CommandContext(context.Background(), c.Args[0], c.Args[1:]...)
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		cmd.Stdin = strings.NewReader("")

		pipe, err := cmd.StdoutPipe()
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}

		waitDone := make(chan struct{})
		defer close(waitDone)

		go func() {
			select {
			case <-ctx.Done():
				interruptProcessGroup(cmd.Process.Pid)
				select {
				case <-waitDone:
				case <-time.After(2 * time.Second):
					killProcessGroup(cmd.Process.Pid)
				}
			case <-waitDone:
			}
		}()

		err = pumpLines(ctx, pipe, sender, c.Parser)
		if ctx.Err() != nil {
			_ = cmd.Wait()
			return
		}
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			_ = cmd.Wait()
			return
		}
		if err := cmd.Wait(); err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}
		sender.Send(EOFMsg{})
	}()
}

func interruptProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGINT)
}

func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
