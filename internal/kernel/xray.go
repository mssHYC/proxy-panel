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

// xrayClientEmail 把 username + 节点 tag 编码为 xray inbound.client.email。
//
// Xray Stats 仅按 email 聚合 user>>> 流量；要拿到 (用户, 节点) 维度的增量，唯一
// 可靠的办法是给每个 inbound 的同一用户设置不同 email。约定的格式 "<username>|<tag>"
// 让采集时能 split 还原。tag 为空时退化成纯 username，与老行为兼容。
func xrayClientEmail(username, nodeTag string) string {
	if nodeTag == "" {
		return username
	}
	return username + "|" + nodeTag
}

// parseXrayClientEmail 反解 xrayClientEmail 编码的 email。
//
// 仅当后缀形如 "|node-<tag>" 时才认为是复合 email——避免把用户名里恰好含 "|" 的
// 历史 email 误拆。tag 找不到则返回 (email, "")，上层会按 node_id=0 记录。
func parseXrayClientEmail(email string) (username, nodeTag string) {
	idx := strings.LastIndex(email, "|node-")
	if idx < 0 {
		return email, ""
	}
	return email[:idx], email[idx+1:]
}

// GetTrafficStats 通过 xray api 获取按 (用户, 节点) 维度的流量增量
func (e *XrayEngine) GetTrafficStats() ([]TrafficStat, error) {
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
func parseXrayStats(out []byte) []TrafficStat {
	type accKey struct {
		username, nodeTag string
	}
	acc := make(map[accKey]*TrafficStat)
	add := func(name string, value int64) {
		parts := strings.Split(name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[2] != "traffic" {
			return
		}
		username, nodeTag := parseXrayClientEmail(parts[1])
		k := accKey{username, nodeTag}
		st, ok := acc[k]
		if !ok {
			st = &TrafficStat{Username: username, NodeTag: nodeTag}
			acc[k] = st
		}
		switch parts[3] {
		case "uplink":
			st.Upload += value
		case "downlink":
			st.Download += value
		}
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" {
		return nil
	}

	// 优先尝试 JSON 格式
	var resp xrayStatsResponse
	if err := json.Unmarshal([]byte(raw), &resp); err == nil && len(resp.Stat) > 0 {
		for _, s := range resp.Stat {
			add(s.Name, s.Value)
		}
	} else {
		// 回退：protobuf 文本格式 name 与 value 分两行
		var curName string
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "name:"):
				curName = extractQuoted(line)
			case strings.HasPrefix(line, "value:") && curName != "":
				valStr := strings.TrimSpace(strings.TrimPrefix(line, "value:"))
				if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
					add(curName, val)
				}
				curName = ""
			}
		}
	}

	result := make([]TrafficStat, 0, len(acc))
	for _, st := range acc {
		result = append(result, *st)
	}
	return result
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

// AddUser 通过 xray api 热添加用户。
//
// email 必须是用户原始 username；这里统一用 xrayClientEmail(email, tag) 编码成
// 与 GenerateConfig 一致的 "<username>|<tag>"，否则 stats key 会落回纯 username，
// 采集时归不到具体节点（node_id=0）。
func (e *XrayEngine) AddUser(tag, uuid, email, protocol string) error {
	server := fmt.Sprintf("127.0.0.1:%d", e.apiPort)
	encodedEmail := xrayClientEmail(email, tag)
	args := []string{"api", "adi", "--server=" + server, "-tag", tag, "-email", encodedEmail}

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

// RemoveUser 通过 xray api 热移除用户。email 同 AddUser 必须做相同编码——
// 否则 xray 配置里登记的 client 是 "<username>|<tag>"，rmi 传纯 username 找不到。
func (e *XrayEngine) RemoveUser(tag, uuid, email string) error {
	server := fmt.Sprintf("127.0.0.1:%d", e.apiPort)
	encodedEmail := xrayClientEmail(email, tag)
	out, err := exec.Command("xray", "api", "rmi",
		"--server="+server, "-tag", tag, "-email", encodedEmail).CombinedOutput()
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

	// 构建用户列表：只注入通过 user_nodes 关联到本节点的用户。
	// 历史版本按 u.Protocol == node.Protocol 过滤，但 users 表 protocol 是单值，
	// 导致用户只能出现在一种协议节点上，其他协议节点 clients=null → 客户端超时。
	// 改为按节点关联过滤，与订阅侧 ListByUserID 可见性保持一致。
	var clientList []interface{}
	for _, u := range users {
		if !userLinkedToNode(u, node.ID) {
			continue
		}
		client := map[string]interface{}{
			// 流量采集需要 (用户, 节点) 维度，而 xray Stats 只能按 email 聚合，
			// 因此这里把节点 tag 编码进 email；采集时由 parseXrayClientEmail 还原。
			"email": xrayClientEmail(u.Email, node.Tag),
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
		// 前端存 snake_case (sni / cert_path / key_path)，同时兼容 camelCase
		tlsSettings := map[string]interface{}{
			"serverName": getSettingStrAny(node.Settings, "", "sni", "serverName"),
		}
		if certPath := getSettingStrAny(node.Settings, "", "cert_path", "certPath"); certPath != "" {
			tlsSettings["certificates"] = []interface{}{
				map[string]interface{}{
					"certificateFile": certPath,
					"keyFile":         getSettingStrAny(node.Settings, "", "key_path", "keyPath"),
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

// ApplyConfig 事务式更新 xray 配置（写入 + Restart + 失败回滚）。详见接口定义。
func (e *XrayEngine) ApplyConfig(data []byte) error {
	return applyConfigWithRollback(e.configPath, e.Restart, data)
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

// getSettingBool 从 Settings 中安全获取布尔值，支持 true/false 和 "true"/"false"/"1"/"0"
func getSettingBool(m map[string]interface{}, key string, defaultVal bool) bool {
	if m == nil {
		return defaultVal
	}
	v, ok := m[key]
	if !ok {
		return defaultVal
	}
	switch val := v.(type) {
	case bool:
		return val
	case string:
		switch strings.ToLower(val) {
		case "true", "1", "yes":
			return true
		case "false", "0", "no", "":
			return false
		}
	case float64:
		return val != 0
	case int:
		return val != 0
	}
	return defaultVal
}

// isSS2022Method 判定 Shadowsocks 加密方法是否属于 2022 系列（多用户协议，使用 users 数组）
func isSS2022Method(method string) bool {
	return strings.HasPrefix(method, "2022-blake3-")
}
