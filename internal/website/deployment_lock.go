package website

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// WithDeploymentLock selects the installer's shared lock. The installed panel
// uses its WorkingDirectory (INSTALL_DIR), independent of DB and Nginx paths.
func (m *Manager) WithDeploymentLock(path string) *Manager {
	m.deploymentLock = path
	return m
}

// LockDeployment excludes the shell installer's website, recovery, and upgrade
// transactions. The inode must remain in place after closing the descriptor.
func (m *Manager) LockDeployment() (func(), error) {
	file, err := os.OpenFile(m.deploymentLock, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, fmt.Errorf("无法打开部署锁: %w", err)
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w：升级或网站部署正在运行，请稍后重试", ErrManagedConflict)
		}
		return nil, fmt.Errorf("获取部署锁失败: %w", err)
	}
	// A killed CLI releases flock before its systemd recovery worker runs.
	// Refuse the intervening window too, including dangling journal symlinks.
	if _, err := os.Lstat(filepath.Join(filepath.Dir(m.deploymentLock), ".website-transaction")); !errors.Is(err, os.ErrNotExist) {
		file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w：网站部署恢复尚未完成", ErrManagedConflict)
	}
	return func() { file.Close() }, nil
}

// CheckSnapshot verifies that a CLI transaction which finished while the API
// was preflighting did not replace the configuration used to plan this save.
func (c *Change) CheckSnapshot() error {
	for filename, expected := range map[string][]byte{c.manager.manifestPath: c.oldManifest, c.manager.configPath: c.oldConfig} {
		if err := regularFile(filename); err != nil {
			return err
		}
		actual, err := os.ReadFile(filename)
		if err != nil || !bytes.Equal(actual, expected) {
			return fmt.Errorf("%w：托管网站配置在预检后发生变化，请重试", ErrManagedConflict)
		}
	}
	return nil
}
