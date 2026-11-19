package website_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"proxy-panel/internal/model"
	"proxy-panel/internal/website"
)

func TestPendingPathChangeRecoversCommittedOldNodeAfterRestart(t *testing.T) {
	f := newFixture(t)
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	change, err := f.manager.Prepare(f.node, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Begin(f.node, candidate); err != nil {
		t.Fatal(err)
	}
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Recreate the manager without retaining any in-memory Change state.
	dir := filepath.Dir(f.config)
	restarted := website.New(f.manifest, f.config, filepath.Join(dir, "panel.db"), filepath.Join(dir, "xray.json"))
	pending, err := restarted.Pending()
	if err != nil || pending == nil {
		t.Fatalf("recovery record unavailable: %v", err)
	}
	if pending.NodeID() != f.node.ID {
		t.Fatal("recovery lost the managed node")
	}
	if err := pending.Restore(context.Background(), f.node); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Prepare(f.node, candidate); err != nil {
		t.Fatalf("old route not restored: %v", err)
	}
	if err := pending.Complete(); err != nil {
		t.Fatal(err)
	}
	if remaining, err := restarted.Pending(); err != nil || remaining != nil {
		t.Fatalf("completed recovery retained: %v", err)
	}
	if info, err := os.Stat(filepath.Join(dir, "path-update.json")); err == nil || info != nil {
		t.Fatal("recovery record was not removed")
	}
}

func TestPendingRecoveryRetainsCommittedNewPathAcrossPartialFileRestore(t *testing.T) {
	f := newFixture(t)
	oldConfig, _ := os.ReadFile(f.config)
	change, candidate := beginPathChange(t, f)
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A restart may find either side of an interrupted pair of atomic renames.
	write(t, f.config, oldConfig)
	pending, err := f.manager.Pending()
	if err != nil {
		t.Fatal(err)
	}
	candidate.Name, candidate.SortOrder = "renamed after commit", 123
	if err := pending.Restore(context.Background(), candidate); err != nil {
		t.Fatal(err)
	}
	next := candidate
	next.Settings = strings.Replace(candidate.Settings, `"/new"`, `"/next"`, 1)
	if _, err := f.manager.Prepare(candidate, next); err != nil {
		t.Fatalf("committed new path was lost: %v", err)
	}
	if err := pending.Restore(context.Background(), candidate); err != nil {
		t.Fatalf("repeated recovery is not idempotent: %v", err)
	}
	if err := pending.Complete(); err != nil {
		t.Fatal(err)
	}
}

func TestFailedRecoveryRetainsRecordForNextRestart(t *testing.T) {
	f := newFixture(t)
	change, _ := beginPathChange(t, f)
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	pending, err := f.manager.Pending()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_RELOAD_FAIL", "1")
	if err := pending.Restore(context.Background(), f.node); err == nil {
		t.Fatal("failed Nginx reload reported recovery success")
	}
	t.Setenv("FAKE_RELOAD_FAIL", "")
	// Missing files are also recoverable from the durable record.
	if err := os.Remove(f.manifest); err != nil {
		t.Fatal(err)
	}
	retry, err := f.manager.Pending()
	if err != nil || retry == nil {
		t.Fatalf("failed recovery lost its record: %v", err)
	}
	if err := retry.Restore(context.Background(), f.node); err != nil {
		t.Fatal(err)
	}
	if err := retry.Complete(); err != nil {
		t.Fatal(err)
	}
}

func TestBeginNeverReplacesAnUnfinishedPathSave(t *testing.T) {
	f := newFixture(t)
	change, candidate := beginPathChange(t, f)
	file := filepath.Join(filepath.Dir(f.config), "path-update.json")
	before, _ := os.ReadFile(file)
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("journal must be private: %v", err)
	}
	if err := change.Begin(f.node, candidate); err == nil {
		t.Fatal("unfinished journal was overwritten")
	}
	after, _ := os.ReadFile(file)
	if string(before) != string(after) {
		t.Fatal("failed second save changed pending recovery")
	}
}

func TestRecoveryStartsTheWebsiteWhenItsServiceIsInactive(t *testing.T) {
	f := newFixture(t)
	change, _ := beginPathChange(t, f)
	if err := change.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	control := filepath.Join(filepath.Dir(f.config), "bin", "systemctl")
	write(t, control, []byte("#!/bin/sh\nif [ \"$1\" = reload ]; then echo 'unit is inactive' >&2; exit 1; fi\n[ \"$1\" = reload-or-restart ]\n"))
	pending, err := f.manager.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if err := pending.Restore(context.Background(), f.node); err != nil {
		t.Fatal(err)
	}
	if err := pending.Complete(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryRefusesUnknownNodeAndManualFileChanges(t *testing.T) {
	for _, scenario := range []string{"node", "manual config", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			_, candidate := beginPathChange(t, f)
			pending, err := f.manager.Pending()
			if err != nil {
				t.Fatal(err)
			}
			selected := f.node
			switch scenario {
			case "node":
				selected.Settings = strings.Replace(selected.Settings, `"/old"`, `"/manual"`, 1)
			case "manual config":
				write(t, f.config, []byte("manual Nginx configuration"))
			case "symlink":
				target := filepath.Join(filepath.Dir(f.config), "manual.conf")
				write(t, target, []byte("manual Nginx configuration"))
				if err := os.Remove(f.config); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, f.config); err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(f.config)
			if err := pending.Restore(context.Background(), selected); err == nil {
				t.Fatal("unrecognized state was overwritten")
			}
			after, _ := os.ReadFile(f.config)
			if string(before) != string(after) {
				t.Fatal("failed recovery changed a manual config")
			}
			if existing, err := f.manager.Pending(); err != nil || existing == nil {
				t.Fatalf("failed recovery lost record: %v", err)
			}
			if err := pending.CheckNode(candidate); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPendingRejectsDamagedAndForeignRecords(t *testing.T) {
	for _, scenario := range []string{"invalid JSON", "version", "database", "snapshot"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFixture(t)
			beginPathChange(t, f)
			file := filepath.Join(filepath.Dir(f.config), "path-update.json")
			data, _ := os.ReadFile(file)
			var fields map[string]any
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "invalid JSON":
				data = []byte("{")
			case "version":
				fields["version"] = 99
			case "database":
				fields["database"] = "/another/panel.db"
			case "snapshot":
				fields["new_manifest"] = nil
			}
			if scenario != "invalid JSON" {
				data, _ = json.Marshal(fields)
			}
			write(t, file, data)
			if _, err := f.manager.Pending(); err == nil {
				t.Fatal("damaged/foreign record was accepted")
			}
		})
	}
}

func beginPathChange(t *testing.T, f *fixture) (*website.Change, model.Node) {
	t.Helper()
	candidate := f.node
	candidate.Settings = strings.Replace(candidate.Settings, `"/old"`, `"/new"`, 1)
	change, err := f.manager.Prepare(f.node, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if err := change.Begin(f.node, candidate); err != nil {
		t.Fatal(err)
	}
	return change, candidate
}
