package website

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"proxy-panel/internal/model"
)

type recoveryRecord struct {
	Version     int        `json:"version"`
	Database    string     `json:"database"`
	KernelPath  string     `json:"kernel_path"`
	OldNode     model.Node `json:"old_node"`
	NewNode     model.Node `json:"new_node"`
	OldManifest []byte     `json:"old_manifest"`
	NewManifest []byte     `json:"new_manifest"`
	OldConfig   []byte     `json:"old_config"`
	NewConfig   []byte     `json:"new_config"`
}

// Recovery is an unfinished path save. The database determines whether its old
// or new route was committed; no separate commit flag can race the DB commit.
type Recovery struct {
	manager *Manager
	record  recoveryRecord
	data    []byte
}

func (m *Manager) recoveryPath() string {
	return filepath.Join(filepath.Dir(m.configPath), "path-update.json")
}

// Begin must finish before the first runtime mutation. Publishing an immutable
// synced file with Link refuses to overwrite another unfinished transaction.
func (c *Change) Begin(old, candidate model.Node) error {
	if old.ID != c.nodeID || candidate.ID != old.ID || sameWebsiteRoute(old, candidate) {
		return errors.New("托管网站恢复记录的节点不匹配")
	}
	record := recoveryRecord{Version: 1, Database: c.manager.databasePath, KernelPath: c.manager.kernelPath,
		OldNode: old, NewNode: candidate, OldManifest: c.oldManifest, NewManifest: c.newManifest,
		OldConfig: c.oldConfig, NewConfig: c.newConfig}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(c.manager.configPath), ".path-journal-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Link(file.Name(), c.manager.recoveryPath()); err != nil {
		return fmt.Errorf("无法创建路径恢复记录（已有记录时须先恢复）: %w", err)
	}
	return syncRecoveryDirectory(c.manager.recoveryPath())
}

// Pending reads only the journal, so a half-written pair of managed files never
// prevents startup from discovering its recovery instructions.
func (m *Manager) Pending() (*Recovery, error) {
	if err := regularFile(m.recoveryPath()); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(m.recoveryPath())
	if err != nil {
		return nil, err
	}
	var record recoveryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("路径恢复记录损坏: %w", err)
	}
	if record.Version != 1 || !sameFilePath(record.Database, m.databasePath) || !sameFilePath(record.KernelPath, m.kernelPath) || record.OldNode.ID <= 0 || record.NewNode.ID != record.OldNode.ID || sameWebsiteRoute(record.OldNode, record.NewNode) {
		return nil, errors.New("路径恢复记录与当前数据库或节点不一致")
	}
	var oldState manifest
	for i, snapshot := range []struct {
		node             model.Node
		manifest, config []byte
	}{
		{record.OldNode, record.OldManifest, record.OldConfig}, {record.NewNode, record.NewManifest, record.NewConfig},
	} {
		var state manifest
		var settings map[string]any
		if json.Unmarshal(snapshot.manifest, &state) != nil || json.Unmarshal([]byte(snapshot.node.Settings), &settings) != nil || settings == nil ||
			!sameFilePath(state.Database, m.databasePath) || !sameFilePath(state.KernelPath, m.kernelPath) ||
			(state.Mode != "" && state.Mode != "proxy") || state.NodeID <= 0 || !routePath.MatchString(state.Path) ||
			len(state.Domains) == 0 || len(state.Listens) == 0 || state.CA == "" || len(snapshot.config) == 0 {
			return nil, errors.New("恢复记录缺少有效的网站快照")
		}
		if state.NodeID == snapshot.node.ID {
			if state.Path != firstText(settings, "path", "path", "/") {
				return nil, errors.New("节点路径与恢复快照不符")
			}
		} else if _, err := realityEndpoint(state, snapshot.node, settings); err != nil {
			return nil, err
		}
		if i == 0 {
			oldState = state
		} else if state.NodeID != oldState.NodeID || (state.NodeID != snapshot.node.ID && (state.Path != oldState.Path || (state.RealityFrontend == nil && oldState.RealityFrontend == nil))) {
			return nil, errors.New("恢复记录的网站绑定不一致")
		}
	}
	return &Recovery{manager: m, record: record, data: data}, nil
}

func (r *Recovery) NodeID() int64 { return r.record.OldNode.ID }

func (r *Recovery) CheckNode(node model.Node) error {
	if !sameWebsiteRoute(node, r.record.OldNode) && !sameWebsiteRoute(node, r.record.NewNode) {
		return errors.New("数据库节点既不匹配旧路径也不匹配新路径，保留恢复记录供检查")
	}
	return nil
}

// Restore follows the committed database route. The coordinator must first
// regenerate and start Xray from that database, then verify this website side.
// A failed restore retains the journal, making recovery safe to retry.
func (r *Recovery) Restore(ctx context.Context, node model.Node) error {
	if err := r.CheckNode(node); err != nil {
		return err
	}
	manifestData, configData := r.record.OldManifest, r.record.OldConfig
	if sameWebsiteRoute(node, r.record.NewNode) {
		manifestData, configData = r.record.NewManifest, r.record.NewConfig
	}
	for _, file := range []struct {
		name      string
		old, next []byte
	}{
		{r.manager.manifestPath, r.record.OldManifest, r.record.NewManifest},
		{r.manager.configPath, r.record.OldConfig, r.record.NewConfig},
	} {
		if err := regularFile(file.name); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		}
		data, err := os.ReadFile(file.name)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, file.old) && !bytes.Equal(data, file.next) {
			return fmt.Errorf("网站文件 %s 已被其他操作修改，保留恢复记录且不覆盖该文件", file.name)
		}
	}
	if err := atomicWrite(r.manager.configPath, configData); err != nil {
		return err
	}
	if err := atomicWrite(r.manager.manifestPath, manifestData); err != nil {
		return err
	}
	// At boot the website may not be active yet; recovery must also start it.
	if err := command(ctx, "systemctl", "reload-or-restart", "proxy-panel-web.service"); err != nil {
		return err
	}
	var state manifest
	if err := json.Unmarshal(manifestData, &state); err != nil {
		return err
	}
	// The live save verified the new fallback before the database commit.
	// Boot recovery only depends on the restored local website and HTTPUpgrade.
	return (&Change{state: state, nodeID: node.ID}).health(ctx, state.Path, nil)
}

// Complete only removes the exact record that was read, after successful
// runtime verification and DB commit (or verified rollback).
func (r *Recovery) Complete() error {
	if err := regularFile(r.manager.recoveryPath()); err != nil {
		return err
	}
	data, err := os.ReadFile(r.manager.recoveryPath())
	if err != nil {
		return err
	}
	if !bytes.Equal(data, r.data) {
		return errors.New("路径恢复记录已改变，不能清除")
	}
	if err := os.Remove(r.manager.recoveryPath()); err != nil {
		return err
	}
	return syncRecoveryDirectory(r.manager.recoveryPath())
}

func (c *Change) Complete() error {
	r, err := c.manager.Pending()
	if err != nil || r == nil {
		return err
	}
	if r.NodeID() != c.nodeID || !bytes.Equal(r.record.OldManifest, c.oldManifest) || !bytes.Equal(r.record.NewManifest, c.newManifest) || !bytes.Equal(r.record.OldConfig, c.oldConfig) || !bytes.Equal(r.record.NewConfig, c.newConfig) {
		return errors.New("路径恢复记录不属于当前保存，不能清除")
	}
	return r.Complete()
}

func syncRecoveryDirectory(filename string) error {
	directory, err := os.Open(filepath.Dir(filename))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
