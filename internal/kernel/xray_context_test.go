package kernel

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestXrayManagedApplyCancellationPreservesOldConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "systemctl"), []byte("#!/bin/sh\nif [ \"$XRAY_TEST_HANG\" = 1 ]; then exec sleep 30; fi\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", root+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("XRAY_TEST_HANG", "1")
	config := filepath.Join(root, "xray.json")
	old := []byte(`{"inbounds":[]}`)
	if err := os.WriteFile(config, old, 0600); err != nil {
		t.Fatal(err)
	}
	engine := NewXrayEngine("xray", config, 10085)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := engine.ApplyConfigContext(ctx, []byte(`{"inbounds":[{}]}`)); err == nil {
		t.Fatal("timed out restart was reported as success")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("restart ignored the save deadline")
	}
	got, err := os.ReadFile(config)
	if err != nil || string(got) != string(old) {
		t.Fatalf("previous config was not preserved: %s %v", got, err)
	}
	// Recovery gets its own deadline; it must not inherit the failed save's context.
	t.Setenv("XRAY_TEST_HANG", "0")
	recovery, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := engine.RestartContext(recovery); err != nil {
		t.Fatalf("fresh recovery failed: %v", err)
	}
}
