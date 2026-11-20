package service_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/kernel"
	"proxy-panel/internal/service"
	"proxy-panel/internal/website"
)

func TestNodeSaveKilledBeforeCommitRecoversOnRestart(t *testing.T) {
	for _, boundary := range []string{"xray", "nginx"} {
		t.Run(boundary, func(t *testing.T) {
			f := newNodeWebsiteFixture(t)
			if err := os.WriteFile(filepath.Join(f.root, "kill-after-"+boundary), nil, 0600); err != nil {
				t.Fatal(err)
			}
			output, err := runNodeRecoveryProcess(t, f, "save")
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ProcessState.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
				t.Fatalf("save was not killed at %s: %v\n%s", boundary, err, output)
			}
			if f.getNode(t).Settings != f.node.Settings {
				t.Fatal("uncommitted path reached subscriptions")
			}
			if boundary == "nginx" {
				f.requirePath(t, "/new", http.StatusSwitchingProtocols)
				// Interrupt recovery itself after it restores Nginx. The following
				// restart must still find the journal and finish idempotently.
				if err := os.WriteFile(filepath.Join(f.root, "kill-after-nginx"), nil, 0600); err != nil {
					t.Fatal(err)
				}
				output, err := runNodeRecoveryProcess(t, f, "recover")
				exit, ok := err.(*exec.ExitError)
				if !ok || exit.ProcessState.Sys().(syscall.WaitStatus).Signal() != syscall.SIGKILL {
					t.Fatalf("recovery was not killed: %v\n%s", err, output)
				}
			}
			for restart := 0; restart < 2; restart++ {
				if output, err := runNodeRecoveryProcess(t, f, "recover"); err != nil {
					t.Fatalf("restart recovery: %v\n%s", err, output)
				}
				f.requirePath(t, "/old", http.StatusSwitchingProtocols)
				f.requirePath(t, "/new", http.StatusNotFound)
				if f.getNode(t).Settings != f.node.Settings {
					t.Fatal("recovery changed the committed node")
				}
			}
		})
	}
}

func TestNodeMetadataSaveIgnoresDamagedWebsiteManifest(t *testing.T) {
	f := newNodeWebsiteFixture(t)
	if err := os.WriteFile(filepath.Join(f.root, "managed.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	name, order := "renamed", 12
	if _, err := f.nodes.Update(f.node.ID, &service.UpdateNodeReq{Name: &name, SortOrder: &order}); err != nil {
		t.Fatal(err)
	}
	if node := f.getNode(t); node.Name != name || node.SortOrder != order || node.Settings != f.node.Settings {
		t.Fatal("metadata save changed or lost the route")
	}
	response := f.save(t, strings.Replace(f.node.Settings, "/old", "/new", 1))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("route change ignored corrupt manifest: %d", response.Code)
	}
	f.requirePath(t, "/old", http.StatusSwitchingProtocols)
}

func TestCommittedNodeSurvivesFailedJournalCleanupAndRestart(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory write denial requires an unprivileged process")
	}
	f := newNodeWebsiteFixture(t)
	t.Cleanup(func() { os.Chmod(f.root, 0700) })
	f.fail(t, "clear")
	output, err := runNodeRecoveryProcess(t, f, "save")
	// Restore access before checks/cleanup, even when the child failed.
	if permissionErr := os.Chmod(f.root, 0700); permissionErr != nil {
		t.Fatal(permissionErr)
	}
	if err != nil {
		t.Fatalf("a committed save must remain successful: %v\n%s", err, output)
	}
	newSettings := strings.Replace(f.node.Settings, "/old", "/new", 1)
	if f.getNode(t).Settings != newSettings {
		t.Fatal("committed path was rolled back after cleanup failure")
	}
	manager := website.New(filepath.Join(f.root, "managed.json"), filepath.Join(f.root, "nginx.conf"), filepath.Join(f.root, "panel.db"), filepath.Join(f.root, "xray.json"))
	if pending, err := manager.Pending(); err != nil || pending == nil {
		t.Fatalf("cleanup failure lost recovery record: %v", err)
	}
	if err := os.Remove(filepath.Join(f.root, "fail-clear")); err != nil {
		t.Fatal(err)
	}
	for restart := 0; restart < 2; restart++ {
		if output, err := runNodeRecoveryProcess(t, f, "recover"); err != nil {
			t.Fatalf("committed recovery: %v\n%s", err, output)
		}
		f.requirePath(t, "/new", http.StatusSwitchingProtocols)
		f.requirePath(t, "/old", http.StatusNotFound)
		if f.getNode(t).Settings != newSettings {
			t.Fatal("recovery reverted a committed path")
		}
	}
}

func TestDeleteFinishesPendingPathRecoveryBeforeRejectingBoundNode(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory write denial requires an unprivileged process")
	}
	f := newNodeWebsiteFixture(t)
	t.Cleanup(func() { os.Chmod(f.root, 0700) })
	f.fail(t, "clear")
	output, err := runNodeRecoveryProcess(t, f, "save")
	if permissionErr := os.Chmod(f.root, 0700); permissionErr != nil {
		t.Fatal(permissionErr)
	}
	if err != nil {
		t.Fatalf("save: %v\n%s", err, output)
	}
	if err := os.Remove(filepath.Join(f.root, "fail-clear")); err != nil {
		t.Fatal(err)
	}
	if err := f.nodes.Delete(f.node.ID); !errors.Is(err, website.ErrManagedConflict) {
		t.Fatalf("expected managed delete conflict: %v", err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "path-update.json")); !os.IsNotExist(err) {
		t.Fatalf("pending recovery survived rejected delete: %v", err)
	}
	if output, err := runNodeRecoveryProcess(t, f, "recover"); err != nil {
		t.Fatalf("restart after rejected delete: %v\n%s", err, output)
	}
}

func runNodeRecoveryProcess(t *testing.T, f *nodeWebsiteFixture, mode string) ([]byte, error) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, "-test.run=^TestNodeRecoveryProcessHelper$", "-test.v")
	command.Env = append(os.Environ(), "NODE_RECOVERY_MODE="+mode, fmt.Sprintf("NODE_RECOVERY_ID=%d", f.node.ID))
	return command.CombinedOutput()
}

// A new process reopens the real database; no in-memory rollback state survives.
func TestNodeRecoveryProcessHelper(t *testing.T) {
	mode := os.Getenv("NODE_RECOVERY_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	root := os.Getenv("NODE_WEBSITE_TEST_ROOT")
	dbPath, kernelPath := filepath.Join(root, "panel.db"), filepath.Join(root, "xray.json")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	nodes := service.NewNodeService(db, nil)
	mgr := kernel.NewManager()
	mgr.Register(kernel.NewXrayEngine("xray", kernelPath, 10085))
	syncSvc := service.NewKernelSyncService(db, mgr)
	nodes.ConfigureManagedWebsite(syncSvc, website.New(filepath.Join(root, "managed.json"), filepath.Join(root, "nginx.conf"), dbPath, kernelPath), kernelPath)
	if mode == "save" {
		id, err := strconv.ParseInt(os.Getenv("NODE_RECOVERY_ID"), 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		node, err := nodes.GetByID(id)
		if err != nil || node == nil {
			t.Fatalf("node: %v", err)
		}
		settings := strings.Replace(node.Settings, "/old", "/new", 1)
		if _, _, err := nodes.UpdateAndSync(id, &service.UpdateNodeReq{Settings: &settings}); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := nodes.RecoverManagedWebsite(); err != nil {
		t.Fatal(err)
	}
	if err := syncSvc.Sync(); err != nil {
		t.Fatal(err)
	}
}
