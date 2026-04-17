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
	binaryPath string
	configPath string
	apiPort    int
	cmd        *exec.Cmd
}

// NewXrayEngine 创建 Xray 引擎实例
func NewXrayEngine(binaryPath, configPath string, apiPort int) *XrayEngine {
	return &XrayEngine{binaryPath: binaryPath, configPath: configPath, apiPort: apiPort}
}

func (e *XrayEngine) Name() string {
	return "xray"
}

// hasSystemctl 检测是否有 systemctl (Linux)
func hasSystemctl() bool {
	_, err := exec.LookPath("systemctl")
	return err == nil
}

func (e *XrayEngine) Start() error {
	if hasSystemctl() {
		return exec.Command("systemctl", "start", "xray").Run()
	}
	// 本地开发模式：直接启动进程
	if e.IsRunning() {
		return nil
	}
	binary, err := exec.LookPath(e.binaryPath)
	if err != nil {
		return fmt.Errorf("未找到 xray 二进制: %s", e.binaryPath)
	}
	e.cmd = exec.Command(binary, "run", "-config", e.configPath)
	e.cmd.Stdout = os.Stdout
	e.cmd.Stderr = os.Stderr
	return e.cmd.Start()
}

func (e *XrayEngine) Stop() error {
	if hasSystemctl() {
		return exec.Command("systemctl", "stop", "xray").Run()
	}
	if e.cmd != nil && e.cmd.Process != nil {
		err := e.cmd.Process.Kill()
		e.cmd = nil
		return err
	}
	return nil
}

func (e *XrayEngine) Restart() error {
	if hasSystemctl() {
		return exec.Command("systemctl", "restart", "xray").Run()
	}
	e.Stop()
	return e.Start()
}

func (e *XrayEngine) IsRunning() bool {
	if hasSystemctl() {
		err := exec.Command("systemctl", "is-active", "--quiet", "xray").Run()
		return err == nil
	}
	return e.cmd != nil && e.cmd.Process != nil && e.cmd.ProcessState == nil
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
	// -reset 让 Xray 返回增量并把内部计数器清零，否则每次采集都是累计值会重复累加
	out, err := exec.Command("xray", "api", "statsquery",
		"--server="+server, "-pattern", "user>>>", "-reset").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("xray statsquery 失败: %w, output: %s", err, string(out))
	}
	return parseXrayStats(out), nil
}

// parseXrayStats 解析 xray api statsquery 输出（兼容 JSON 与 protobuf 文本）
func parseXrayStats(out []byte) map[string]*UserTraffic {
	result := make(map[string]*UserTraffic)
	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return result
	}

	// 优先尝试 JSON 格式
	var resp xrayStatsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err == nil && len(resp.Stat) > 0 {
		for _, s := range resp.Stat {
			parseStatEntry(result, s.Name, s.Value)
		}
		return result
	}

	// 回退：protobuf 文本格式 name 与 value 分两行，例如：
	//   stat: <
	//     name: "user>>>alice>>>traffic>>>uplink"
	//     value: 12345
	//   >
	var curName string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "name:"):
			curName = extractQuoted(line)
		case strings.HasPrefix(line, "value:") && curName != "":
			valStr := strings.TrimSpace(strings.TrimPrefix(line, "value:"))
			if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
				parseStatEntry(result, curName, val)
			}
			curName = ""
		}
	}
	return result
}

// parseStatEntry 解析 "user>>>{email}>>>traffic>>>{uplink|downlink}" 格式并累加
func parseStatEntry(result map[string]*UserTraffic, name string, value int64) {
	parts := strings.Split(name, ">>>")
	if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
		return
	}
	email := parts[1]
	if _, ok := result[email]; !ok {
		result[email] = &UserTraffic{}
	}
	switch parts[3] {
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
	case "httpupgrade":
		// Xray-core 1.8.3+ 支持 httpupgrade，字段名 httpupgradeSettings
		huSettings := map[string]interface{}{
			"path": getSettingStr(node.Settings, "path", "/"),
		}
		if host := getSettingStr(node.Settings, "host", ""); host != "" {
			huSettings["host"] = host
		}
		stream["httpupgradeSettings"] = huSettings
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
		dest := getSettingStrAny(node.Settings, "", "dest")
		// Xray 要求 dest 包含端口，如 "www.tesla.com:443"
		if dest != "" && !strings.Contains(dest, ":") {
			dest = dest + ":443"
		}
		stream["realitySettings"] = map[string]interface{}{
			"dest":        dest,
			"serverNames": getSettingSliceAny(node.Settings, "server_names", "serverNames"),
			"privateKey":  getSettingStrAny(node.Settings, "", "private_key", "privateKey"),
			"shortIds":    getSettingSliceAny(node.Settings, "short_ids", "shortIds"),
		}
	}

	return stream
}

// WriteConfig 将配置写入文件
func (e *XrayEngine) WriteConfig(data []byte) error {
	return os.WriteFile(e.configPath, data, 0600)
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

// getSettingStrAny 尝试多个 key 名，返回第一个匹配的字符串值
func getSettingStrAny(m map[string]interface{}, defaultVal string, keys ...string) string {
	for _, key := range keys {
		if v := getSettingStr(m, key, ""); v != "" {
			return v
		}
	}
	return defaultVal
}

// getSettingSliceAny 尝试多个 key 名，返回第一个匹配的字符串切片
func getSettingSliceAny(m map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		if v := getSettingSlice(m, key); len(v) > 0 {
			return v
		}
	}
	return nil
}

// getSettingInt 从 Settings 中安全获取整数值，兼容 JSON 解析产生的 float64/int/string
func getSettingInt(m map[string]interface{}, key string, defaultVal int) int {
	if m == nil {
		return defaultVal
	}
	v, ok := m[key]
	if !ok {
		return defaultVal
	}
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		if val == "" {
			return defaultVal
		}
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}
