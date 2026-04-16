package kernel

import (
	"errors"
	"os"
	"os/exec"
)

// SingboxEngine sing-box 内核引擎 (v1.0 基础框架，v1.1 完善)
type SingboxEngine struct {
	configPath string
}

// NewSingboxEngine 创建 sing-box 引擎实例
func NewSingboxEngine(configPath string) *SingboxEngine {
	return &SingboxEngine{configPath: configPath}
}

func (e *SingboxEngine) Name() string {
	return "sing-box"
}

func (e *SingboxEngine) Start() error {
	return exec.Command("systemctl", "start", "sing-box").Run()
}

func (e *SingboxEngine) Stop() error {
	return exec.Command("systemctl", "stop", "sing-box").Run()
}

func (e *SingboxEngine) Restart() error {
	return exec.Command("systemctl", "restart", "sing-box").Run()
}

func (e *SingboxEngine) IsRunning() bool {
	err := exec.Command("systemctl", "is-active", "--quiet", "sing-box").Run()
	return err == nil
}

// GetTrafficStats v1.0 返回空 map，v1.1 实现
func (e *SingboxEngine) GetTrafficStats() (map[string]*UserTraffic, error) {
	return make(map[string]*UserTraffic), nil
}

// AddUser sing-box 不支持热加载用户
func (e *SingboxEngine) AddUser(tag, uuid, email, protocol string) error {
	return errors.New("sing-box 不支持热加载用户，请重启内核")
}

// RemoveUser sing-box 不支持热移除用户
func (e *SingboxEngine) RemoveUser(tag, uuid, email string) error {
	return errors.New("sing-box 不支持热加载用户，请重启内核")
}

// GenerateConfig v1.0 返回空 JSON，v1.1 实现
func (e *SingboxEngine) GenerateConfig(nodes []NodeConfig, users []UserConfig) ([]byte, error) {
	return []byte("{}"), nil
}

// WriteConfig 将配置写入文件
func (e *SingboxEngine) WriteConfig(data []byte) error {
	return os.WriteFile(e.configPath, data, 0644)
}
