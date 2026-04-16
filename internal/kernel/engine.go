package kernel

// UserTraffic 用户流量统计
type UserTraffic struct {
	Upload   int64
	Download int64
}

// NodeConfig 节点配置
type NodeConfig struct {
	Tag       string
	Port      int
	Protocol  string
	Transport string
	Settings  map[string]interface{}
}

// UserConfig 用户配置
type UserConfig struct {
	UUID       string
	Email      string
	Protocol   string
	SpeedLimit int64
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
