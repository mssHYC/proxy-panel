package firewall

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// runner 抽象 exec 调用，便于测试注入
type runner func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error)

// defaultRunner 使用 exec.CommandContext，stdout 和 stderr 分别收集
var defaultRunner runner = func(ctx context.Context, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		return out.Bytes(), errBuf.Bytes(), fmt.Errorf("%s %v: %w (stderr=%s)",
			name, args, err, errBuf.String())
	}
	return out.Bytes(), errBuf.Bytes(), nil
}
