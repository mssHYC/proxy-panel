package kernel

// UserTraffic 用户流量统计
type UserTraffic struct {
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
	// GetTrafficStats 获取所有用户的流量统计
	GetTrafficStats() (map[string]*UserTraffic, error)
	// AddUser 热添加用户
	AddUser(tag, uuid, email, protocol string) error
	// RemoveUser 热移除用户
	RemoveUser(tag, uuid, email string) error
	// GenerateConfig 根据节点和用户列表生成配置文件内容
	GenerateConfig(nodes []NodeConfig, users []UserConfig) ([]byte, error)
	// WriteConfig 将配置写入文件
	WriteConfig(data []byte) error
}
