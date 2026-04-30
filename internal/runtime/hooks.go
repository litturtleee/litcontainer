package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

func RunHooks(hooks []Hook, state *ContainerState) error {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	for _, hook := range hooks {
		if err := runHook(hook, stateJSON); err != nil {
			return err // 任一失败即终止
		}
	}
	return nil
}

func runHook(hook Hook, stateJSON []byte) error {
	// 设置超时
	timeout := 30 * time.Second
	if hook.Timeout != nil && *hook.Timeout > 0 {
		timeout = time.Duration(*hook.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// args[0] 是程序名，其余是参数
	args := hook.Args
	if len(args) == 0 {
		args = []string{hook.Path}
	}

	cmd := exec.CommandContext(ctx, hook.Path, args[1:]...)
	cmd.Env = hook.Env
	cmd.Stdin = bytes.NewReader(stateJSON) // 状态通过 stdin 传入

	// 捕获输出便于调试
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("hook %s timed out after %v", hook.Path, timeout)
		}
		return fmt.Errorf("hook %s failed: %w\noutput: %s", hook.Path, err, out.String())
	}
	return nil
}
