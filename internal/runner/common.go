package runner

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

func runCommandWithContext(ctx context.Context, cmd *exec.Cmd) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	prepareProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	select {
	case err := <-waitCh:
		return out.Bytes(), err
	case <-ctx.Done():
		_ = killProcessGroup(cmd)
		select {
		case <-waitCh:
		case <-time.After(4 * time.Second):
			_ = killProcessGroup(cmd)
		}
		return out.Bytes(), ctx.Err()
	}
}
