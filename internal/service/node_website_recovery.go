package service

import (
	"context"
	"fmt"
	"time"
)

// RecoverManagedWebsite must finish before ordinary kernel sync or serving
// requests. Reopening the DB decides whether an interrupted save committed.
func (s *NodeService) RecoverManagedWebsite() error {
	s.mutationMu.Lock()
	defer s.mutationMu.Unlock()
	if s.syncSvc != nil {
		s.syncSvc.syncMu.Lock()
		defer s.syncSvc.syncMu.Unlock()
	}
	return s.recoverManagedWebsiteLocked()
}

func (s *NodeService) recoverManagedWebsiteLocked() error {
	if s.website == nil {
		return nil
	}
	recovery, err := s.website.Pending()
	if err != nil {
		return fmt.Errorf("读取网站路径恢复记录失败: %w", err)
	}
	if recovery == nil {
		return nil
	}
	unlock, err := s.website.LockDeployment()
	if err != nil {
		return err
	}
	defer unlock()
	// Another participant could have finished recovery between the discovery
	// read and acquiring the CLI's deployment lock.
	recovery, err = s.website.Pending()
	if err != nil || recovery == nil {
		return err
	}
	if s.syncSvc == nil {
		return fmt.Errorf("网站路径恢复缺少内核同步服务")
	}
	node, err := s.GetByID(recovery.NodeID())
	if err != nil {
		return err
	}
	if node == nil {
		return fmt.Errorf("网站路径恢复对应的节点 %d 不存在", recovery.NodeID())
	}
	if err := recovery.CheckNode(*node); err != nil {
		return err
	}
	// Generate from current committed users/nodes, never replay an old credential
	// snapshot that could re-enable a user disabled since the interrupted save.
	data, err := s.syncSvc.xrayConfigWithNode(*node)
	if err != nil {
		return err
	}
	engine, err := s.syncSvc.mgr.Get("xray")
	if err != nil {
		return err
	}
	bounded, ok := engine.(interface{ RestartContext(context.Context) error })
	if !ok {
		return fmt.Errorf("Xray 引擎不支持带超时的恢复")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := engine.WriteConfig(data); err != nil {
		return fmt.Errorf("恢复 Xray 配置失败: %w", err)
	}
	if err := bounded.RestartContext(ctx); err != nil {
		return fmt.Errorf("恢复 Xray 服务失败: %w", err)
	}
	if err := recovery.Restore(ctx, *node); err != nil {
		return fmt.Errorf("恢复托管网站失败: %w", err)
	}
	if err := recovery.Complete(); err != nil {
		return fmt.Errorf("服务已恢复，清理恢复记录失败: %w", err)
	}
	return nil
}
