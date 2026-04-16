package kernel

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// SingboxEngine sing-box 内核引擎 (v1.0 基础框架，v1.1 完善)
type SingboxEngine struct {
	binaryPath string
	configPath string
	cmd        *exec.Cmd
}

// NewSingboxEngine 创建 sing-box 引擎实例
func NewSingboxEngine(binaryPath, configPath string) *SingboxEngine {
	return &SingboxEngine{binaryPath: binaryPath, configPath: configPath}
}

func (e *SingboxEngine) Name() string {
	return "sing-box"
}

func (e *SingboxEngine) Start() error {
	if hasSystemctl() {
		return exec.Command("systemctl", "start", "sing-box").Run()
	}
	if e.IsRunning() {
		return nil
	}
	binary, err := exec.LookPath(e.binaryPath)
	if err != nil {
		return fmt.Errorf("未找到 sing-box 二进制: %s", e.binaryPath)
	}
	e.cmd = exec.Command(binary, "run", "-c", e.configPath)
	e.cmd.Stdout = os.Stdout
	e.cmd.Stderr = os.Stderr
	return e.cmd.Start()
}

func (e *SingboxEngine) Stop() error {
	if hasSystemctl() {
		return exec.Command("systemctl", "stop", "sing-box").Run()
	}
	if e.cmd != nil && e.cmd.Process != nil {
		err := e.cmd.Process.Kill()
		e.cmd = nil
		return err
	}
	return nil
}

func (e *SingboxEngine) Restart() error {
	if hasSystemctl() {
		return exec.Command("systemctl", "restart", "sing-box").Run()
	}
	e.Stop()
	return e.Start()
}

func (e *SingboxEngine) IsRunning() bool {
	if hasSystemctl() {
		err := exec.Command("systemctl", "is-active", "--quiet", "sing-box").Run()
		return err == nil
	}
	return e.cmd != nil && e.cmd.Process != nil && e.cmd.ProcessState == nil
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
