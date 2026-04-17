package kernel

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// connSnapshot 记录单条连接上次采集到的累计字节数
type connSnapshot struct {
	upload   int64
	download int64
}

// SingboxEngine sing-box 内核引擎
type SingboxEngine struct {
	binaryPath string
	configPath string
	apiPort    int
	cmd        *exec.Cmd
	// statsEnabled 标记最近一次生成的配置是否启用了 clash_api；
	// 未启用时跳过采集，避免反复请求失败刷屏
	statsEnabled atomic.Bool
	// connSnap 缓存上一次 /connections 快照，用于计算各连接的字节数增量
	connMu     sync.Mutex
	connSnap   map[string]connSnapshot
	httpClient *http.Client
	// inboundUser 记录 inbound tag 到唯一用户的映射，用于 sing-box 1.13 clash_api
	// 不提供 inboundUser 字段时的回退归属（仅当该 inbound 仅登记 1 个用户时写入）
	inboundUserMu sync.Mutex
	inboundUser   map[string]string
}

// NewSingboxEngine 创建 sing-box 引擎实例
func NewSingboxEngine(binaryPath, configPath string, apiPort int) *SingboxEngine {
	return &SingboxEngine{
		binaryPath:  binaryPath,
		configPath:  configPath,
		apiPort:     apiPort,
		connSnap:    make(map[string]connSnapshot),
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		inboundUser: make(map[string]string),
	}
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

// clashSnapshot 对应 sing-box clash_api /connections 的返回结构
type clashSnapshot struct {
	Connections []clashConnection `json:"connections"`
}

type clashConnection struct {
	ID       string        `json:"id"`
	Metadata clashMetadata `json:"metadata"`
	Upload   int64         `json:"upload"`
	Download int64         `json:"download"`
}

type clashMetadata struct {
	InboundUser string `json:"inboundUser"`
	Type        string `json:"type"`
}

// GetTrafficStats 通过 sing-box 的 clash_api /connections 接口按用户聚合流量
// sing-box 官方二进制通常不带 with_v2ray_api tag，因此改用带 with_clash_api 的通用方案。
// 注意：短连接在关闭瞬间会从 /connections 列表消失，最后一次增量可能丢失；
// 对于 hy2 这种长连接为主的协议影响较小。
func (e *SingboxEngine) GetTrafficStats() (map[string]*UserTraffic, error) {
	if e.apiPort == 0 || !e.statsEnabled.Load() {
		return make(map[string]*UserTraffic), nil
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/connections", e.apiPort)
	resp, err := e.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("sing-box clash_api 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sing-box clash_api 状态码 %d", resp.StatusCode)
	}
	var snap clashSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, fmt.Errorf("解析 clash_api 响应失败: %w", err)
	}

	result := make(map[string]*UserTraffic)
	e.connMu.Lock()
	defer e.connMu.Unlock()

	seen := make(map[string]struct{}, len(snap.Connections))
	for _, c := range snap.Connections {
		user := c.Metadata.InboundUser
		// sing-box 1.13 的 clash_api 不再暴露 inboundUser；回退按 inbound tag 归属
		// type 格式形如 "hysteria2/node-3"，切片后半部分即 inbound tag
		if user == "" {
			if _, tag, ok := strings.Cut(c.Metadata.Type, "/"); ok && tag != "" {
				e.inboundUserMu.Lock()
				user = e.inboundUser[tag]
				e.inboundUserMu.Unlock()
			}
		}
		if user == "" {
			continue
		}
		seen[c.ID] = struct{}{}
		prev := e.connSnap[c.ID]
		deltaUp := c.Upload - prev.upload
		deltaDown := c.Download - prev.download
		// 防御：sing-box 重启或计数异常时把增量当全量处理
		if deltaUp < 0 {
			deltaUp = c.Upload
		}
		if deltaDown < 0 {
			deltaDown = c.Download
		}
		if _, ok := result[user]; !ok {
			result[user] = &UserTraffic{}
		}
		result[user].Upload += deltaUp
		result[user].Download += deltaDown
		e.connSnap[c.ID] = connSnapshot{upload: c.Upload, download: c.Download}
	}
	// 清理已关闭的连接
	for id := range e.connSnap {
		if _, ok := seen[id]; !ok {
			delete(e.connSnap, id)
		}
	}
	return result, nil
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
	// 收集 inbound tag -> 唯一用户映射，供采集时回退归属
	inboundUserMap := make(map[string]string)
	for _, node := range nodes {
		inbound := e.buildInbound(node, users)
		if inbound == nil {
			continue
		}
		inbounds = append(inbounds, inbound)
		if len(users) == 1 {
			inboundUserMap[node.Tag] = users[0].Email
		}
	}
	e.inboundUserMu.Lock()
	e.inboundUser = inboundUserMap
	e.inboundUserMu.Unlock()

	cfg["inbounds"] = inbounds
	cfg["outbounds"] = []interface{}{
		map[string]interface{}{"type": "direct", "tag": "direct"},
		map[string]interface{}{"type": "block", "tag": "block"},
	}

	// 启用 clash_api：只要有 inbound 就开启，panel 通过 /connections 按用户采集流量
	enableStats := e.apiPort > 0 && len(inbounds) > 0
	if enableStats {
		cfg["experimental"] = map[string]interface{}{
			"clash_api": map[string]interface{}{
				"external_controller": fmt.Sprintf("127.0.0.1:%d", e.apiPort),
			},
		}
	}
	e.statsEnabled.Store(enableStats)

	return json.MarshalIndent(cfg, "", "  ")
}

func (e *SingboxEngine) buildInbound(node NodeConfig, users []UserConfig) map[string]interface{} {
	s := node.Settings

	switch node.Protocol {
	case "hysteria2":
		// 只注入通过 user_nodes 关联到本节点的用户（与订阅侧 ListByUserID 对齐），
		// 避免未关联用户的 UUID 也被列进 inbound.users 造成跨节点误用。
		userList := make([]map[string]interface{}, 0)
		linkedUsers := make([]UserConfig, 0, len(users))
		for _, u := range users {
			if !userLinkedToNode(u, node.ID) {
				continue
			}
			userList = append(userList, map[string]interface{}{
				"name":     u.Email,
				"password": u.UUID,
			})
			linkedUsers = append(linkedUsers, u)
		}

		inbound := map[string]interface{}{
			"type":        "hysteria2",
			"tag":         node.Tag,
			"listen":      "::",
			"listen_port": node.Port,
			"users":       userList,
		}

		// 带宽上限：节点级 max_up_mbps/max_down_mbps → sing-box inbound up_mbps/down_mbps
		// 单用户独享时与用户 speed_limit 取更严格值（sing-box hy2 无 per-user 带宽字段）。
		// 判定依据必须是"真正能用该节点的用户"即 linkedUsers，而非系统全量 users —
		// 否则多用户系统里别人即便没关联该节点，也会把这里判成多用户从而丢掉 speed_limit。
		upMbps := getSettingInt(s, "max_up_mbps", 0)
		downMbps := getSettingInt(s, "max_down_mbps", 0)
		if len(linkedUsers) == 1 && linkedUsers[0].SpeedLimit > 0 {
			userLim := int(linkedUsers[0].SpeedLimit)
			if upMbps == 0 || userLim < upMbps {
				upMbps = userLim
			}
			if downMbps == 0 || userLim < downMbps {
				downMbps = userLim
			}
		}
		if upMbps > 0 {
			inbound["up_mbps"] = upMbps
		}
		if downMbps > 0 {
			inbound["down_mbps"] = downMbps
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
