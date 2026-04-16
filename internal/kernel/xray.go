package kernel

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// XrayEngine Xray 内核引擎
type XrayEngine struct {
	configPath string
	apiPort    int
}

// NewXrayEngine 创建 Xray 引擎实例
func NewXrayEngine(configPath string, apiPort int) *XrayEngine {
	return &XrayEngine{configPath: configPath, apiPort: apiPort}
}

func (e *XrayEngine) Name() string {
	return "xray"
}

func (e *XrayEngine) Start() error {
	return exec.Command("systemctl", "start", "xray").Run()
}

func (e *XrayEngine) Stop() error {
	return exec.Command("systemctl", "stop", "xray").Run()
}

func (e *XrayEngine) Restart() error {
	return exec.Command("systemctl", "restart", "xray").Run()
}

func (e *XrayEngine) IsRunning() bool {
	err := exec.Command("systemctl", "is-active", "--quiet", "xray").Run()
	return err == nil
}

// xrayStatEntry xray api statsquery 返回的单条统计
type xrayStatEntry struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// xrayStatsResponse xray api statsquery 的 JSON 响应
type xrayStatsResponse struct {
	Stat []xrayStatEntry `json:"stat"`
}

// GetTrafficStats 通过 xray api 获取用户流量统计
func (e *XrayEngine) GetTrafficStats() (map[string]*UserTraffic, error) {
	server := fmt.Sprintf("127.0.0.1:%d", e.apiPort)
	out, err := exec.Command("xray", "api", "statsquery",
		"--server="+server, "-pattern", "user>>>").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("xray statsquery 失败: %w, output: %s", err, string(out))
	}

	result := make(map[string]*UserTraffic)
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return result, nil
	}

	// 尝试 JSON 格式解析
	var resp xrayStatsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err == nil {
		for _, s := range resp.Stat {
			e.parseStat(result, s.Name, s.Value)
		}
		return result, nil
	}

	// 回退到文本格式解析（每行格式: name: "user>>>email>>>traffic>>>uplink" value: 12345）
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			// 提取 name 和 value
			name := extractQuoted(line)
			// 下一部分可能在同行或下行，这里简化处理同行场景
			parts := strings.SplitN(line, "value:", 2)
			if len(parts) == 2 {
				valStr := strings.TrimSpace(parts[1])
				if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
					e.parseStat(result, name, val)
				}
			}
		} else if strings.HasPrefix(line, "stat:") {
			continue
		} else if strings.Contains(line, "user>>>") {
			// 兼容更多文本格式
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				name := strings.Trim(fields[0], "\"")
				if val, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
					e.parseStat(result, name, val)
				}
			}
		}
	}
	return result, nil
}

// parseStat 解析 "user>>>{email}>>>traffic>>>{uplink|downlink}" 格式
func (e *XrayEngine) parseStat(result map[string]*UserTraffic, name string, value int64) {
	// 格式: user>>>{email}>>>traffic>>>{uplink|downlink}
	parts := strings.Split(name, ">>>")
	if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
		return
	}
	email := parts[1]
	direction := parts[3]

	if _, ok := result[email]; !ok {
		result[email] = &UserTraffic{}
	}
	switch direction {
	case "uplink":
		result[email].Upload += value
	case "downlink":
		result[email].Download += value
	}
}

// extractQuoted 从字符串中提取双引号内的内容
func extractQuoted(s string) string {
	start := strings.Index(s, "\"")
	if start == -1 {
		return ""
	}
	end := strings.Index(s[start+1:], "\"")
	if end == -1 {
		return ""
	}
	return s[start+1 : start+1+end]
}

// AddUser 通过 xray api 热添加用户
func (e *XrayEngine) AddUser(tag, uuid, email, protocol string) error {
	server := fmt.Sprintf("127.0.0.1:%d", e.apiPort)
	args := []string{"api", "adi", "--server=" + server, "-tag", tag, "-email", email}

	switch protocol {
	case "vless", "vmess":
		args = append(args, "-id", uuid)
	case "trojan", "shadowsocks":
		args = append(args, "-password", uuid)
	default:
		args = append(args, "-id", uuid)
	}

	out, err := exec.Command("xray", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xray adi 失败: %w, output: %s", err, string(out))
	}
	return nil
}

// RemoveUser 通过 xray api 热移除用户
func (e *XrayEngine) RemoveUser(tag, uuid, email string) error {
	server := fmt.Sprintf("127.0.0.1:%d", e.apiPort)
	out, err := exec.Command("xray", "api", "rmi",
		"--server="+server, "-tag", tag, "-email", email).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xray rmi 失败: %w, output: %s", err, string(out))
	}
	return nil
}

// GenerateConfig 生成 Xray 完整配置
func (e *XrayEngine) GenerateConfig(nodes []NodeConfig, users []UserConfig) ([]byte, error) {
	cfg := map[string]interface{}{
		"stats": map[string]interface{}{},
		"policy": map[string]interface{}{
			"levels": map[string]interface{}{
				"0": map[string]interface{}{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
				},
			},
			"system": map[string]interface{}{
				"statsInboundUplink":    true,
				"statsInboundDownlink":  true,
				"statsOutboundUplink":   true,
				"statsOutboundDownlink": true,
			},
		},
		"api": map[string]interface{}{
			"tag":      "api",
			"services": []string{"HandlerService", "StatsService"},
		},
	}

	// API inbound (dokodemo-door)
	apiInbound := map[string]interface{}{
		"tag":      "api-inbound",
		"listen":   "127.0.0.1",
		"port":     e.apiPort,
		"protocol": "dokodemo-door",
		"settings": map[string]interface{}{
			"address": "127.0.0.1",
		},
	}

	inbounds := []interface{}{apiInbound}

	// 为每个节点生成 inbound
	for _, node := range nodes {
		inbound := e.buildInbound(node, users)
		inbounds = append(inbounds, inbound)
	}
	cfg["inbounds"] = inbounds

	// outbounds
	cfg["outbounds"] = []interface{}{
		map[string]interface{}{
			"tag":      "direct",
			"protocol": "freedom",
		},
		map[string]interface{}{
			"tag":      "blocked",
			"protocol": "blackhole",
		},
	}

	// routing
	cfg["routing"] = map[string]interface{}{
		"rules": []interface{}{
			map[string]interface{}{
				"type":        "field",
				"inboundTag":  []string{"api-inbound"},
				"outboundTag": "api",
			},
			map[string]interface{}{
				"type":        "field",
				"protocol":    []string{"bittorrent"},
				"outboundTag": "blocked",
			},
		},
	}

	return json.MarshalIndent(cfg, "", "  ")
}

// buildInbound 根据节点配置构建 inbound
func (e *XrayEngine) buildInbound(node NodeConfig, users []UserConfig) map[string]interface{} {
	inbound := map[string]interface{}{
		"tag":    node.Tag,
		"listen": "0.0.0.0",
		"port":   node.Port,
		"protocol": node.Protocol,
	}

	// 构建用户列表
	var clientList []interface{}
	for _, u := range users {
		if u.Protocol != node.Protocol {
			continue
		}
		client := map[string]interface{}{
			"email": u.Email,
			"level": 0,
		}
		switch node.Protocol {
		case "vless":
			client["id"] = u.UUID
			client["flow"] = getSettingStr(node.Settings, "flow", "")
		case "vmess":
			client["id"] = u.UUID
			client["alterId"] = 0
		case "trojan":
			client["password"] = u.UUID
		case "shadowsocks":
			client["password"] = u.UUID
			client["method"] = getSettingStr(node.Settings, "method", "aes-128-gcm")
		}
		clientList = append(clientList, client)
	}

	// settings
	switch node.Protocol {
	case "vless":
		decryption := "none"
		settings := map[string]interface{}{
			"decryption": decryption,
			"clients":    clientList,
		}
		inbound["settings"] = settings
	case "vmess":
		inbound["settings"] = map[string]interface{}{
			"clients": clientList,
		}
	case "trojan":
		inbound["settings"] = map[string]interface{}{
			"clients": clientList,
		}
	case "shadowsocks":
		inbound["settings"] = map[string]interface{}{
			"clients": clientList,
			"network": "tcp,udp",
		}
	}

	// streamSettings
	stream := e.buildStreamSettings(node)
	if stream != nil {
		inbound["streamSettings"] = stream
	}

	return inbound
}

// buildStreamSettings 根据传输方式构建 streamSettings
func (e *XrayEngine) buildStreamSettings(node NodeConfig) map[string]interface{} {
	transport := node.Transport
	if transport == "" {
		transport = "tcp"
	}

	stream := map[string]interface{}{
		"network": transport,
	}

	switch transport {
	case "ws":
		wsSettings := map[string]interface{}{
			"path": getSettingStr(node.Settings, "path", "/"),
		}
		if host := getSettingStr(node.Settings, "host", ""); host != "" {
			wsSettings["headers"] = map[string]interface{}{
				"Host": host,
			}
		}
		stream["wsSettings"] = wsSettings
	case "grpc":
		stream["grpcSettings"] = map[string]interface{}{
			"serviceName": getSettingStr(node.Settings, "serviceName", ""),
		}
	}

	// TLS / Reality
	security := getSettingStr(node.Settings, "security", "none")
	stream["security"] = security

	switch security {
	case "tls":
		tlsSettings := map[string]interface{}{
			"serverName": getSettingStr(node.Settings, "serverName", ""),
		}
		if certPath := getSettingStr(node.Settings, "certPath", ""); certPath != "" {
			tlsSettings["certificates"] = []interface{}{
				map[string]interface{}{
					"certificateFile": certPath,
					"keyFile":         getSettingStr(node.Settings, "keyPath", ""),
				},
			}
		}
		stream["tlsSettings"] = tlsSettings
	case "reality":
		stream["realitySettings"] = map[string]interface{}{
			"dest":         getSettingStr(node.Settings, "dest", ""),
			"serverNames":  getSettingSlice(node.Settings, "serverNames"),
			"privateKey":   getSettingStr(node.Settings, "privateKey", ""),
			"shortIds":     getSettingSlice(node.Settings, "shortIds"),
		}
	}

	return stream
}

// WriteConfig 将配置写入文件
func (e *XrayEngine) WriteConfig(data []byte) error {
	return os.WriteFile(e.configPath, data, 0644)
}

// getSettingStr 从 Settings 中安全获取字符串值
func getSettingStr(m map[string]interface{}, key, defaultVal string) string {
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultVal
}

// getSettingSlice 从 Settings 中安全获取字符串切片
func getSettingSlice(m map[string]interface{}, key string) []string {
	if m == nil {
		return nil
	}
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []interface{}:
		result := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}
