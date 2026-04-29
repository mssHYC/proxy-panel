package kernel

import (
	"errors"
	"fmt"
	"os"
)

// ErrConfigRolledBack 表明 ApplyConfig 已把内核回滚到旧配置；调用方据此区分
// "新配置失败 + 旧配置仍在跑"和"新旧都跑不起来"两种场景。
var ErrConfigRolledBack = errors.New("kernel config rolled back to previous version")

// ApplyConfigForTest 暴露 applyConfigWithRollback 给跨包测试用，业务代码勿用。
func ApplyConfigForTest(configPath string, restart func() error, data []byte) error {
	return applyConfigWithRollback(configPath, restart, data)
}

// applyConfigWithRollback 写入 + 重启 + 失败回滚的通用实现。
// 返回的 error 在已经成功回滚时会 Wrap ErrConfigRolledBack，便于日志/指标识别。
// hadBackup=false（首次同步）时无可回滚——直接把原始 restart 错误返回。
func applyConfigWithRollback(configPath string, restart func() error, data []byte) error {
	var backup []byte
	hadBackup := false
	if old, err := os.ReadFile(configPath); err == nil {
		backup = old
		hadBackup = true
	}
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("写入配置失败: %w", err)
	}
	if err := restart(); err != nil {
		if !hadBackup {
			return fmt.Errorf("重启失败（无旧配置可回滚）: %w", err)
		}
		if rbErr := os.WriteFile(configPath, backup, 0600); rbErr != nil {
			return fmt.Errorf("重启失败且回滚写入也失败: restart=%v, rollback=%w", err, rbErr)
		}
		// 尝试用旧配置重启；失败也不再覆盖原始错误信息。
		_ = restart()
		return fmt.Errorf("%w: %v", ErrConfigRolledBack, err)
	}
	return nil
}

// UserTraffic 用户流量统计
type UserTraffic struct {
	Upload   int64
	Download int64
}

// TrafficStat 单条 (节点 tag, 用户名) 维度的流量增量。
//
// NodeTag 为空表示引擎无法把流量归属到具体节点（例如老配置升级后仍存在的纯
// username 形式 stats key），此时上层会按 node_id=0 记录，可观测但无法做节点
// 维度统计。Username 与 user_nodes.users.username 一致。
type TrafficStat struct {
	NodeTag  string
	Username string
	Upload   int64
	Download int64
}

// NodeConfig 节点配置
type NodeConfig struct {
	ID        int64
	Tag       string
	Port      int
	Protocol  string
	Transport string
	Settings  map[string]interface{}
}

// UserConfig 用户配置
//
// NodeIDs 为该用户通过 user_nodes 关联的节点 ID 列表。内核生成 inbound.clients 时
// 按 NodeIDs 过滤用户（见 userLinkedToNode）：
//   - NodeIDs 非空：只把该用户注入到关联的节点 inbound
//   - NodeIDs 为空：回退到"可用全部节点"，与订阅 handler 的 ListByUserID→
//     ListEnabled 兜底语义对齐，避免老部署（从未勾选节点）升级后所有 inbound
//     的 clients 全空导致所有协议超时。
type UserConfig struct {
	UUID       string
	Email      string
	Protocol   string
	SpeedLimit int64
	NodeIDs    []int64
}

// userLinkedToNode 判断用户是否关联了给定节点。
//
// 对齐订阅侧 subscription.Subscribe 的降级语义：ListByUserID 查到空时会回退到
// ListEnabled 返回全部启用节点。inbound 侧必须跟上这个兜底 —— 即 NodeIDs 为空
// 代表"该用户未设置节点白名单，可用全部节点"，否则只接受显式关联的节点。
//
// nodeID == 0 视为测试桩（没有明确节点身份），也按"兜底 true"处理以保持旧用例可跑。
func userLinkedToNode(u UserConfig, nodeID int64) bool {
	if nodeID == 0 {
		return true
	}
	if len(u.NodeIDs) == 0 {
		return true
	}
	for _, id := range u.NodeIDs {
		if id == nodeID {
			return true
		}
	}
	return false
}

// Engine 内核引擎接口，抽象 Xray / Sing-box 等代理内核的统一操作
type Engine interface {
	// Name 返回引擎名称
	Name() string
	// Start 启动内核
	Start() error
	// Stop 停止内核
	Stop() error
	// Restart 重启内核
	Restart() error
	// IsRunning 检查内核是否正在运行
	IsRunning() bool
	// GetTrafficStats 获取按 (节点, 用户) 维度的流量增量
	GetTrafficStats() ([]TrafficStat, error)
	// AddUser 热添加用户
	AddUser(tag, uuid, email, protocol string) error
	// RemoveUser 热移除用户
	RemoveUser(tag, uuid, email string) error
	// GenerateConfig 根据节点和用户列表生成配置文件内容
	GenerateConfig(nodes []NodeConfig, users []UserConfig) ([]byte, error)
	// WriteConfig 将配置写入文件
	WriteConfig(data []byte) error
	// ApplyConfig 事务式更新内核：备份旧配置 → 写入新配置 → Restart；
	// Restart 失败时把旧配置回写并尝试再次 Restart（best-effort），返回带回滚标记的错误。
	// 与 WriteConfig + Restart 的差别在于失败语义——单独失败的写入或重启不会留下损坏的运行态。
	ApplyConfig(data []byte) error
}
