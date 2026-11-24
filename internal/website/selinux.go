package website

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"time"
)

// getenforce also handles a nonstandard selinuxfs mount. Consult the kernel
// directly on minimal installations that omit the utility.
func selinuxEnforcing(ctx context.Context) (bool, error) {
	getenforce, err := exec.LookPath("getenforce")
	if err != nil && !errors.Is(err, exec.ErrNotFound) {
		return false, err
	}
	var data []byte
	if err == nil {
		data, err = exec.CommandContext(ctx, getenforce).Output()
	} else {
		data, err = os.ReadFile("/sys/fs/selinux/enforce")
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
	}
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(string(data)) {
	case "Enforcing", "1":
		return true, nil
	case "Permissive", "Disabled", "0":
		return false, nil
	default:
		return false, fmt.Errorf("未知的 SELinux 状态 %q", data)
	}
}

func checkRealityWebBind(endpoint netip.AddrPort) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	enforcing, err := selinuxEnforcing(ctx)
	if err != nil {
		return fmt.Errorf("检查 SELinux 状态失败: %w", err)
	}
	if enforcing {
		// Use the installer's labeled probe and transient Nginx service. The
		// panel's own domain can bind ports that httpd_t may not use.
		if err := command(ctx, "bash", "/usr/local/bin/proxy-panel", "website", "check-reality-bind", endpoint.String()); err != nil {
			return fmt.Errorf("%w：SELinux enforcing 下 Nginx 监听 %s 预检失败，请检查 AVC 日志并确认已更新管理脚本: %v", ErrManagedConflict, endpoint, err)
		}
	}
	return nil
}
