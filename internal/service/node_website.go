package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"proxy-panel/internal/kernel"
	"proxy-panel/internal/model"
	"proxy-panel/internal/website"
)

// ConfigureManagedWebsite connects node saves to the same kernel synchronizer
// used by handlers and scheduled jobs. Call once before serving requests.
func (s *NodeService) ConfigureManagedWebsite(syncSvc *KernelSyncService, manager *website.Manager, xrayConfig string) {
	s.syncSvc, s.website, s.xrayConfig = syncSvc, manager, xrayConfig
}

// UpdateAndSync reports whether a managed website change was applied immediately;
// other updates retain the handler's normal debounced kernel synchronization.
func (s *NodeService) UpdateAndSync(id int64, req *UpdateNodeReq) (*model.Node, bool, error) {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.syncSvc != nil {
		s.syncSvc.syncMu.Lock()
		defer s.syncSvc.syncMu.Unlock()
	}
	if err := s.recoverManagedWebsiteLocked(); err != nil {
		return nil, false, err
	}
	old, err := s.GetByID(id)
	if err != nil || old == nil {
		return nil, false, err
	}
	candidate, err := s.nodeCandidate(old, req)
	if err != nil {
		return nil, false, err
	}
	if s.website == nil {
		node, err := s.updateStored(old, candidate, req)
		return node, false, err
	}
	change, err := s.website.Prepare(*old, candidate)
	if err != nil {
		return nil, false, fmt.Errorf("托管网站检查失败: %w", err)
	}
	if change == nil {
		node, err := s.updateStored(old, candidate, req)
		return node, false, err
	}
	if s.syncSvc == nil {
		return nil, false, errors.New("托管网站未连接内核同步服务")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := change.CheckOld(ctx); err != nil {
		return nil, false, fmt.Errorf("旧路径检查失败，节点未修改: %w", err)
	}
	previous, err := os.ReadFile(s.xrayConfig)
	if err != nil {
		return nil, false, fmt.Errorf("备份 Xray 配置失败: %w", err)
	}
	engine, err := s.syncSvc.mgr.Get("xray")
	if err != nil {
		return nil, false, err
	}
	boundedEngine, ok := engine.(interface {
		ApplyConfigContext(context.Context, []byte) error
		RestartContext(context.Context) error
	})
	if !ok {
		return nil, false, errors.New("Xray 引擎不支持带超时的托管网站同步")
	}
	next, err := s.syncSvc.xrayConfigWithNode(candidate)
	if err != nil {
		return nil, false, err
	}
	if err := change.Begin(*old, candidate); err != nil {
		return nil, false, fmt.Errorf("保存路径恢复记录失败，节点未修改: %w", err)
	}
	rollback := func(cause error) (*model.Node, bool, error) {
		rollbackCtx, stop := context.WithTimeout(context.Background(), 45*time.Second)
		defer stop()
		var failures []error
		// Do not use ApplyConfig here: if restarting the old config fails, its
		// automatic rollback would write the rejected new config back to disk.
		if err := engine.WriteConfig(previous); err != nil {
			failures = append(failures, fmt.Errorf("恢复 Xray 文件失败: %w", err))
		} else if err := boundedEngine.RestartContext(rollbackCtx); err != nil {
			failures = append(failures, fmt.Errorf("恢复 Xray 服务失败: %w", err))
		}
		if err := change.Rollback(rollbackCtx); err != nil {
			failures = append(failures, fmt.Errorf("恢复 Nginx 失败: %w", err))
		} else if err := change.CheckOld(rollbackCtx); err != nil {
			failures = append(failures, fmt.Errorf("旧路径恢复检查失败: %w", err))
		}
		if len(failures) > 0 {
			return nil, false, fmt.Errorf("节点未保存；%v；回退未完成: %w", cause, errors.Join(failures...))
		}
		if err := change.Complete(); err != nil {
			return nil, false, fmt.Errorf("已恢复旧配置，但恢复记录清理失败: %v；原错误: %w", err, cause)
		}
		return nil, false, fmt.Errorf("节点未保存，已恢复旧配置: %w", cause)
	}
	if err := boundedEngine.ApplyConfigContext(ctx, next); err != nil {
		return rollback(fmt.Errorf("应用 Xray 配置失败: %w", err))
	}
	if err := change.Apply(ctx); err != nil {
		return rollback(fmt.Errorf("同步 Nginx 失败: %w", err))
	}
	// Persist only after both runtimes accept the new path. Failed operations
	// never publish a new path to subscriptions or overwrite a concurrent row.
	node, err := s.updateStored(old, candidate, req)
	if err != nil {
		return rollback(err)
	}
	// Once the DB commit succeeds the new path is authoritative. A failed
	// journal cleanup must never turn a successful save into an old-path rollback.
	if err := change.Complete(); err != nil {
		log.Printf("[网站同步] 节点 %d 已保存，恢复记录待下次启动或保存时清理: %v", node.ID, err)
	}
	return node, true, nil
}

func (s *NodeService) nodeCandidate(old *model.Node, req *UpdateNodeReq) (model.Node, error) {
	candidate := *old
	if req.Name != nil {
		candidate.Name = *req.Name
	}
	if req.Host != nil {
		candidate.Host = *req.Host
	}
	if req.Port != nil {
		candidate.Port = *req.Port
	}
	if req.Protocol != nil {
		candidate.Protocol = *req.Protocol
	}
	if req.Transport != nil {
		candidate.Transport = *req.Transport
	}
	if req.KernelType != nil {
		candidate.KernelType = *req.KernelType
	}
	if req.Settings != nil {
		candidate.Settings = *req.Settings
	}
	if req.Enable != nil {
		candidate.Enable = *req.Enable
	}
	if req.SortOrder != nil {
		candidate.SortOrder = *req.SortOrder
	}
	// Allow disabling legacy invalid nodes without repairing their settings.
	if req.Protocol != nil || req.Transport != nil || req.KernelType != nil || req.Port != nil || req.Settings != nil || (req.Enable != nil && *req.Enable) {
		if err := validateNodeSettings(candidate.Protocol, candidate.Transport, candidate.KernelType, candidate.Port, candidate.Settings); err != nil {
			return candidate, err
		}
	}
	if req.Settings != nil || (req.Enable != nil && *req.Enable) {
		if err := s.validateRealityChange(old.Settings, candidate.Settings, req.Enable != nil && *req.Enable && !old.Enable); err != nil {
			return candidate, err
		}
	}
	return candidate, nil
}

func (s *KernelSyncService) xrayConfigWithNode(candidate model.Node) ([]byte, error) {
	nodes, err := s.loadNodes()
	if err != nil {
		return nil, err
	}
	users, err := s.loadUsers()
	if err != nil {
		return nil, err
	}
	var configs []kernel.NodeConfig
	for _, node := range nodes {
		if node.kernelType == "singbox" {
			continue
		}
		if node.id == candidate.ID {
			node.port, node.protocol, node.transport = candidate.Port, candidate.Protocol, candidate.Transport
			node.settings = nil
			if err := json.Unmarshal([]byte(candidate.Settings), &node.settings); err != nil {
				return nil, err
			}
		}
		configs = append(configs, kernel.NodeConfig{ID: node.id, Tag: fmt.Sprintf("node-%d", node.id), Port: node.port,
			Protocol: node.protocol, Transport: node.transport, Settings: node.settings})
	}
	engine, err := s.mgr.Get("xray")
	if err != nil {
		return nil, err
	}
	return engine.GenerateConfig(configs, users)
}
