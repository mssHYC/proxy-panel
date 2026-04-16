package kernel

import (
	"encoding/json"
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

// GenerateConfig 生成 sing-box 完整配置
func (e *SingboxEngine) GenerateConfig(nodes []NodeConfig, users []UserConfig) ([]byte, error) {
	cfg := map[string]interface{}{
		"log": map[string]interface{}{
			"level": "warn",
		},
	}

	inbounds := make([]interface{}, 0)

	for _, node := range nodes {
		inbound := e.buildInbound(node, users)
		if inbound != nil {
			inbounds = append(inbounds, inbound)
		}
	}

	cfg["inbounds"] = inbounds
	cfg["outbounds"] = []interface{}{
		map[string]interface{}{"type": "direct", "tag": "direct"},
		map[string]interface{}{"type": "block", "tag": "block"},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

func (e *SingboxEngine) buildInbound(node NodeConfig, users []UserConfig) map[string]interface{} {
	s := node.Settings

	switch node.Protocol {
	case "hysteria2":
		userList := make([]map[string]interface{}, 0)
		for _, u := range users {
			userList = append(userList, map[string]interface{}{
				"name":     u.Email,
				"password": u.UUID,
			})
		}

		inbound := map[string]interface{}{
			"type":        "hysteria2",
			"tag":         node.Tag,
			"listen":      "::",
			"listen_port": node.Port,
			"users":       userList,
		}

		// TLS 配置
		certPath := getSettingStr(s, "cert_path", "")
		keyPath := getSettingStr(s, "key_path", "")
		if certPath != "" && keyPath != "" {
			inbound["tls"] = map[string]interface{}{
				"enabled":          true,
				"certificate_path": certPath,
				"key_path":         keyPath,
			}
		}

		// 混淆配置
		obfs := getSettingStr(s, "obfs", "")
		if obfs != "" {
			obfsPassword := getSettingStr(s, "obfs_password", "")
			inbound["obfs"] = map[string]interface{}{
				"type":     obfs,
				"password": obfsPassword,
			}
		}

		return inbound

	case "vless", "vmess", "trojan":
		// TODO: 后续支持
		return nil

	default:
		return nil
	}
}

// WriteConfig 将配置写入文件
func (e *SingboxEngine) WriteConfig(data []byte) error {
	return os.WriteFile(e.configPath, data, 0600)
}
