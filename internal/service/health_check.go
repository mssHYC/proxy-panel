package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"proxy-panel/internal/database"
	"proxy-panel/internal/model"
	notify "proxy-panel/internal/service/notify"
)

const healthFailThreshold = 3

// HealthChecker 节点 TCP 健康检查
type HealthChecker struct {
	db        *database.DB
	nodeSvc   *NodeService
	notifySvc *notify.NotifyService
	timeout   time.Duration
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(db *database.DB, nodeSvc *NodeService, notifySvc *notify.NotifyService) *HealthChecker {
	return &HealthChecker{
		db:        db,
		nodeSvc:   nodeSvc,
		notifySvc: notifySvc,
		timeout:   3 * time.Second,
	}
}

// CheckAll 并发探测所有启用节点，写回状态并在首次超阈值时发告警
func (h *HealthChecker) CheckAll(ctx context.Context) error {
	nodes, err := h.nodeSvc.ListEnabled()
	if err != nil {
		return fmt.Errorf("列出启用节点失败: %w", err)
	}

	var wg sync.WaitGroup
	for i := range nodes {
		wg.Add(1)
		go func(n model.Node) {
			defer wg.Done()
			h.checkOne(ctx, &n)
		}(nodes[i])
	}
	wg.Wait()
	return nil
}

func isUDPProtocol(p string) bool {
	switch p {
	case "hysteria2", "hy2", "tuic":
		return true
	}
	return false
}

// dialOne 按协议执行一次探测：UDP 协议走 QUIC 握手，其余走 TCP connect。
func (h *HealthChecker) dialOne(ctx context.Context, n *model.Node) error {
	dctx, cancel := context.WithTimeout(ctx, h.timeout)
	defer cancel()
	if isUDPProtocol(n.Protocol) {
		return probeQUIC(dctx, n.Host, n.Port)
	}
	dialer := &net.Dialer{Timeout: h.timeout}
	addr := net.JoinHostPort(n.Host, strconv.Itoa(n.Port))
	conn, err := dialer.DialContext(dctx, "tcp", addr)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

func (h *HealthChecker) checkOne(ctx context.Context, n *model.Node) {
	err := h.dialOne(ctx, n)
	if err == nil {
		if _, uerr := h.db.Exec(`UPDATE nodes
			SET last_check_at = ?, last_check_ok = 1, last_check_err = '', fail_count = 0
			WHERE id = ?`, time.Now(), n.ID); uerr != nil {
			log.Printf("[健康检查] 更新节点 %d 状态失败: %v", n.ID, uerr)
		}
		NodeHealth.WithLabelValues(n.Name, n.Protocol).Set(1)
		NodeFailCount.WithLabelValues(n.Name).Set(0)
		return
	}

	errMsg := err.Error()
	newFail := n.FailCount + 1
	if _, uerr := h.db.Exec(`UPDATE nodes
		SET last_check_at = ?, last_check_ok = 0, last_check_err = ?, fail_count = ?
		WHERE id = ?`, time.Now(), errMsg, newFail, n.ID); uerr != nil {
		log.Printf("[健康检查] 更新节点 %d 状态失败: %v", n.ID, uerr)
	}
	NodeHealth.WithLabelValues(n.Name, n.Protocol).Set(0)
	NodeFailCount.WithLabelValues(n.Name).Set(float64(newFail))

	// 仅在首次跨越阈值时发送，避免重复告警
	if n.FailCount < healthFailThreshold && newFail >= healthFailThreshold && h.notifySvc != nil {
		msg := fmt.Sprintf("🚫 节点「%s」(%s:%d) 连续 %d 次探测失败：%s",
			n.Name, n.Host, n.Port, newFail, errMsg)
		h.notifySvc.SendAll(msg)
		AlertsSentTotal.WithLabelValues("node_offline").Inc()
	}
}
