package website_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"proxy-panel/internal/website"
)

func TestDeploymentLockHonorsCLIAndOrphanedRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".update.lock")
	manager := website.New("", "", "", "").WithDeploymentLock(path)
	cli, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()
	if err := syscall.Flock(int(cli.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LockDeployment(); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("concurrent CLI not rejected: %v", err)
	}
	journal := filepath.Join(dir, ".website-transaction")
	if err := os.Symlink("pending-backup", journal); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(cli.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.LockDeployment(); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("unfinished CLI transaction accepted: %v", err)
	}
	if err := os.Remove(journal); err != nil {
		t.Fatal(err)
	}
	unlock, err := manager.LockDeployment()
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(cli.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); !errors.Is(err, syscall.EWOULDBLOCK) {
		t.Fatalf("shell-style flock during API transaction: %v", err)
	}
	unlock()
	if err := syscall.Flock(int(cli.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("CLI could not reacquire the original lock: %v", err)
	}
}
