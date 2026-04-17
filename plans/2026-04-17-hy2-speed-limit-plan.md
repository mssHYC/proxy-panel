# Hysteria2 节点限速实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 Hy2 节点编辑页支持 `最大上行/下行 (Mbps)`（1-20，默认 10），下发到 sing-box inbound；单用户独享节点时与 `user.speed_limit` 融合取更严格值。

**Architecture:** 节点 settings JSON 追加 `max_up_mbps`/`max_down_mbps` 两个字段（无 schema 迁移），在 `kernel/singbox.go` hy2 `buildInbound` 里读出并下发成 sing-box inbound 的 `up_mbps`/`down_mbps`。单用户时用 `min(user.speed_limit, node.max)` 覆盖。前端 `Nodes.vue` 加 input、`Users.vue` 的 `speed_limit` 加 tooltip 标明语义。

**Tech Stack:** Go (modernc.org/sqlite)、Vue 3 + Element Plus、sing-box clash_api

**Spec:** [specs/2026-04-17-hy2-speed-limit-design.md](../specs/2026-04-17-hy2-speed-limit-design.md)

---

### Task 1: 添加 `getSettingInt` 工具函数（TDD）

**Files:**
- Create: `internal/kernel/xray_test.go`
- Modify: `internal/kernel/xray.go`（文件末尾，`getSettingSliceAny` 之后）

- [ ] **Step 1: 写失败测试**

Create `internal/kernel/xray_test.go`:

```go
package kernel

import "testing"

func TestGetSettingInt(t *testing.T) {
	cases := []struct {
		name     string
		m        map[string]interface{}
		key      string
		def      int
		expected int
	}{
		{"nil map returns default", nil, "x", 7, 7},
		{"missing key returns default", map[string]interface{}{}, "x", 7, 7},
		{"int value", map[string]interface{}{"x": 5}, "x", 7, 5},
		{"float64 from json", map[string]interface{}{"x": 5.0}, "x", 7, 5},
		{"int64 value", map[string]interface{}{"x": int64(5)}, "x", 7, 5},
		{"numeric string", map[string]interface{}{"x": "12"}, "x", 7, 12},
		{"invalid string falls back", map[string]interface{}{"x": "abc"}, "x", 7, 7},
		{"empty string falls back", map[string]interface{}{"x": ""}, "x", 7, 7},
		{"wrong type falls back", map[string]interface{}{"x": []int{1}}, "x", 7, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getSettingInt(c.m, c.key, c.def); got != c.expected {
				t.Errorf("%s: want %d, got %d", c.name, c.expected, got)
			}
		})
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/kernel/ -run TestGetSettingInt -v`
Expected: 编译失败，`undefined: getSettingInt`

- [ ] **Step 3: 实现 `getSettingInt`**

In `internal/kernel/xray.go`, append after `getSettingSliceAny`（文件末尾）:

```go
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
```

（`strconv` 已在 xray.go 的 import 块里，不用新增。）

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/kernel/ -run TestGetSettingInt -v`
Expected: 9 个 sub-case 全部 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/kernel/xray_test.go internal/kernel/xray.go
git commit -m "feat(kernel): add getSettingInt helper

与 getSettingStr 风格一致，兼容 JSON 反序列化产生的
float64/int64/string 多种类型。后续 hy2 限速字段读取会用到。

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 2: Hy2 buildInbound 节点级限速下发（TDD）

**Files:**
- Create: `internal/kernel/singbox_test.go`
- Modify: `internal/kernel/singbox.go:156-207`（`buildInbound` hy2 分支）

- [ ] **Step 1: 写失败测试**

Create `internal/kernel/singbox_test.go`:

```go
package kernel

import (
	"encoding/json"
	"testing"
)

func TestHy2BuildInbound_NodeLevelLimit(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"max_up_mbps":   float64(8),
			"max_down_mbps": float64(15),
		},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2"},
		{UUID: "u2", Email: "bob", Protocol: "hysteria2"},
	}
	ib := e.buildInbound(node, users)
	if ib["up_mbps"] != 8 {
		t.Errorf("up_mbps: want 8, got %v", ib["up_mbps"])
	}
	if ib["down_mbps"] != 15 {
		t.Errorf("down_mbps: want 15, got %v", ib["down_mbps"])
	}
}

func TestHy2BuildInbound_NoLimitsOmitted(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2"},
	}
	ib := e.buildInbound(node, users)
	if _, ok := ib["up_mbps"]; ok {
		t.Errorf("expected no up_mbps key, got %v", ib["up_mbps"])
	}
	if _, ok := ib["down_mbps"]; ok {
		t.Errorf("expected no down_mbps key")
	}
}

func TestHy2GenerateConfigSerializable(t *testing.T) {
	e := NewSingboxEngine("", "", 9090)
	nodes := []NodeConfig{{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{"max_up_mbps": float64(7)},
	}}
	users := []UserConfig{{UUID: "u1", Email: "a", Protocol: "hysteria2"}}
	data, err := e.GenerateConfig(nodes, users)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/kernel/ -run TestHy2 -v`
Expected: `TestHy2BuildInbound_NodeLevelLimit` FAIL —— `up_mbps: want 8, got <nil>`。另外两个测试可能通过但不代表功能存在。

- [ ] **Step 3: 修改 `buildInbound` hy2 分支**

In `internal/kernel/singbox.go`, 替换 hy2 分支（`case "hysteria2":` 内部）。找到：

```go
		inbound := map[string]interface{}{
			"type":        "hysteria2",
			"tag":         node.Tag,
			"listen":      "::",
			"listen_port": node.Port,
			"users":       userList,
		}

		// TLS 配置
```

在 `inbound := ...` 闭合的 `}` 之后、`// TLS 配置` 之前插入：

```go

		// 带宽上限：节点级 max_up_mbps/max_down_mbps → sing-box inbound up_mbps/down_mbps
		// （单用户场景的用户级 speed_limit 融合逻辑在 Task 3 里追加）
		if upMbps := getSettingInt(s, "max_up_mbps", 0); upMbps > 0 {
			inbound["up_mbps"] = upMbps
		}
		if downMbps := getSettingInt(s, "max_down_mbps", 0); downMbps > 0 {
			inbound["down_mbps"] = downMbps
		}
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/kernel/ -run TestHy2 -v`
Expected: 3 个测试全部 PASS

- [ ] **Step 5: 跑完整包避免回归**

Run: `go test ./internal/kernel/ -v`
Expected: 所有已有测试 + 新测试全部 PASS

- [ ] **Step 6: Commit**

```bash
git add internal/kernel/singbox_test.go internal/kernel/singbox.go
git commit -m "feat(kernel): hy2 节点级带宽上限下发 sing-box

nodes.settings 新增 max_up_mbps/max_down_mbps 字段，
buildInbound 读取后下发 sing-box hy2 inbound 的
up_mbps/down_mbps。0 或缺省为不限速。

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 3: 单用户场景融合 user.speed_limit（TDD）

**Files:**
- Modify: `internal/kernel/singbox_test.go`（追加测试）
- Modify: `internal/kernel/singbox.go`（上一步插入的逻辑处）

- [ ] **Step 1: 追加失败测试**

In `internal/kernel/singbox_test.go`, 追加在文件末尾：

```go
func TestHy2BuildInbound_SingleUserUseMinimum(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"max_up_mbps":   float64(20),
			"max_down_mbps": float64(20),
		},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2", SpeedLimit: 5},
	}
	ib := e.buildInbound(node, users)
	if ib["up_mbps"] != 5 {
		t.Errorf("single-user: up_mbps want 5, got %v", ib["up_mbps"])
	}
	if ib["down_mbps"] != 5 {
		t.Errorf("single-user: down_mbps want 5, got %v", ib["down_mbps"])
	}
}

func TestHy2BuildInbound_SingleUserNodeMaxWhenUserHigher(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"max_down_mbps": float64(10),
		},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2", SpeedLimit: 100},
	}
	ib := e.buildInbound(node, users)
	if ib["down_mbps"] != 10 {
		t.Errorf("user 100 > node 10: down_mbps want 10, got %v", ib["down_mbps"])
	}
}

func TestHy2BuildInbound_SingleUserNoNodeMax(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2", SpeedLimit: 3},
	}
	ib := e.buildInbound(node, users)
	if ib["up_mbps"] != 3 || ib["down_mbps"] != 3 {
		t.Errorf("node unset, user=3: want both 3, got up=%v down=%v", ib["up_mbps"], ib["down_mbps"])
	}
}

func TestHy2BuildInbound_MultiUserIgnoresUserLimits(t *testing.T) {
	e := NewSingboxEngine("", "", 0)
	node := NodeConfig{
		Tag:      "node-1",
		Port:     443,
		Protocol: "hysteria2",
		Settings: map[string]interface{}{
			"max_up_mbps":   float64(10),
			"max_down_mbps": float64(10),
		},
	}
	users := []UserConfig{
		{UUID: "u1", Email: "alice", Protocol: "hysteria2", SpeedLimit: 3},
		{UUID: "u2", Email: "bob", Protocol: "hysteria2", SpeedLimit: 5},
	}
	ib := e.buildInbound(node, users)
	if ib["up_mbps"] != 10 || ib["down_mbps"] != 10 {
		t.Errorf("multi-user should fallback to node max; got up=%v down=%v", ib["up_mbps"], ib["down_mbps"])
	}
}
```

- [ ] **Step 2: 运行测试验证失败**

Run: `go test ./internal/kernel/ -run TestHy2BuildInbound_Single -v`
Expected: 3 个 single-user 测试 FAIL（down_mbps want 5/3 但 got 20/0），multi-user 测试 PASS（已经在用节点 max）

- [ ] **Step 3: 在 hy2 分支里插入单用户融合逻辑**

In `internal/kernel/singbox.go`, 替换 Task 2 刚插入的那段：

```go
		// 带宽上限：节点级 max_up_mbps/max_down_mbps → sing-box inbound up_mbps/down_mbps
		// （单用户场景的用户级 speed_limit 融合逻辑在 Task 3 里追加）
		if upMbps := getSettingInt(s, "max_up_mbps", 0); upMbps > 0 {
			inbound["up_mbps"] = upMbps
		}
		if downMbps := getSettingInt(s, "max_down_mbps", 0); downMbps > 0 {
			inbound["down_mbps"] = downMbps
		}
```

改为：

```go
		// 带宽上限：节点级 max_up_mbps/max_down_mbps → sing-box inbound up_mbps/down_mbps
		// 单用户独享时与用户 speed_limit 取更严格值（sing-box hy2 无 per-user 带宽字段）
		upMbps := getSettingInt(s, "max_up_mbps", 0)
		downMbps := getSettingInt(s, "max_down_mbps", 0)
		if len(users) == 1 && users[0].SpeedLimit > 0 {
			userLim := int(users[0].SpeedLimit)
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
```

- [ ] **Step 4: 运行测试验证通过**

Run: `go test ./internal/kernel/ -v`
Expected: 全部 PASS（Task 1 + Task 2 + Task 3 的所有测试）

- [ ] **Step 5: Commit**

```bash
git add internal/kernel/singbox_test.go internal/kernel/singbox.go
git commit -m "feat(kernel): 单用户独享 hy2 节点时融合 user.speed_limit

sing-box hysteria2 inbound 无 per-user 带宽字段，架构上不能严格
按人限速。当 inbound 只登记 1 个用户时，把用户的 speed_limit 与
节点 max 取 min 下发到 inbound up_mbps/down_mbps，此时等价于
精确 per-user；多用户共享时退化为节点级总带宽限制。

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 4: Nodes.vue 前端添加最大上行/下行字段

**Files:**
- Modify: `web/src/views/Nodes.vue`（4 处）

- [ ] **Step 1: 在表单 reactive 里加默认值**

Find in `web/src/views/Nodes.vue` (≈ line 428-430)：

```js
  // Hysteria2
  hy2_obfs_type: '',
  hy2_obfs_password: '',
```

改为：

```js
  // Hysteria2
  hy2_obfs_type: '',
  hy2_obfs_password: '',
  hy2_max_up_mbps: 10,
  hy2_max_down_mbps: 10,
```

- [ ] **Step 2: 在 Hy2 配置块加两个 input**

Find（≈ line 273-275, Hy2 混淆密码那项之后）：

```vue
          <el-form-item v-if="form.hy2_obfs_type" label="混淆密码">
            <el-input v-model="form.hy2_obfs_password" placeholder="混淆密码" />
          </el-form-item>
```

在这一 `</el-form-item>` 之后插入：

```vue
          <el-form-item label="最大上行 (Mbps)">
            <el-input-number v-model="form.hy2_max_up_mbps" :min="0" :max="20" controls-position="right" />
            <span class="ml-2 text-xs text-gray-400">0 = 不限速；节点总带宽上限，多用户共享</span>
          </el-form-item>
          <el-form-item label="最大下行 (Mbps)">
            <el-input-number v-model="form.hy2_max_down_mbps" :min="0" :max="20" controls-position="right" />
            <span class="ml-2 text-xs text-gray-400">0 = 不限速</span>
          </el-form-item>
```

- [ ] **Step 3: 提交 form 时把字段写入 settings**

Find（≈ line 499-506, 提交处 hy2 分支）：

```js
  // Hysteria2
  if (form.protocol === 'hysteria2') {
    if (form.sni) s.sni = form.sni
    if (form.cert_path) s.cert_path = form.cert_path
    if (form.key_path) s.key_path = form.key_path
    if (form.hy2_obfs_type) { s.obfs = form.hy2_obfs_type; s.obfs_password = form.hy2_obfs_password }
  }
```

改为：

```js
  // Hysteria2
  if (form.protocol === 'hysteria2') {
    if (form.sni) s.sni = form.sni
    if (form.cert_path) s.cert_path = form.cert_path
    if (form.key_path) s.key_path = form.key_path
    if (form.hy2_obfs_type) { s.obfs = form.hy2_obfs_type; s.obfs_password = form.hy2_obfs_password }
    s.max_up_mbps = Number(form.hy2_max_up_mbps) || 0
    s.max_down_mbps = Number(form.hy2_max_down_mbps) || 0
  }
```

- [ ] **Step 4: 编辑节点时从 settings 回填表单**

Find（≈ line 548-549, 回填 Hy2 部分）：

```js
  form.hy2_obfs_type = s.obfs || ''
  form.hy2_obfs_password = s.obfs_password || ''
```

改为：

```js
  form.hy2_obfs_type = s.obfs || ''
  form.hy2_obfs_password = s.obfs_password || ''
  form.hy2_max_up_mbps = typeof s.max_up_mbps === 'number' ? s.max_up_mbps : 10
  form.hy2_max_down_mbps = typeof s.max_down_mbps === 'number' ? s.max_down_mbps : 10
```

- [ ] **Step 5: 前端构建不报错**

Run: `cd web && npm run build 2>&1 | tail -20`
Expected: `✓ built in ...`，无 TypeScript/Vue 错误

- [ ] **Step 6: Commit**

```bash
git add web/src/views/Nodes.vue
git commit -m "feat(ui): 节点编辑页 Hy2 配置加最大上行/下行

新增 hy2_max_up_mbps / hy2_max_down_mbps 两个数字输入（1-20，
默认 10），提交时序列化为 settings.max_up_mbps / max_down_mbps。

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 5: Users.vue speed_limit 字段加 tooltip

**Files:**
- Modify: `web/src/views/Users.vue:122-126`

- [ ] **Step 1: 给 speed_limit 输入加 tooltip + 调整说明文字**

Find（≈ line 122-126）：

```vue
        <el-form-item label="限速 Mbps">
          <el-input-number v-model="form.speed_limit" :min="0" controls-position="right" />
          <span class="ml-2 text-xs text-gray-400">0 = 无限制</span>
        </el-form-item>
```

改为：

```vue
        <el-form-item label="限速 Mbps">
          <el-tooltip placement="top" content="仅在该用户独享某 hy2 节点时严格生效；多用户共用同一节点时退化为节点级总带宽限制。">
            <el-input-number v-model="form.speed_limit" :min="0" controls-position="right" />
          </el-tooltip>
          <span class="ml-2 text-xs text-gray-400">0 = 无限制（仅对 hy2 协议，且仅单用户场景）</span>
        </el-form-item>
```

- [ ] **Step 2: 前端构建**

Run: `cd web && npm run build 2>&1 | tail -10`
Expected: `✓ built`

- [ ] **Step 3: Commit**

```bash
git add web/src/views/Users.vue
git commit -m "docs(ui): 用户 speed_limit 字段加 tooltip 说明生效条件

该字段仅对 hy2 协议且单用户独享节点时严格生效，通过 tooltip
与辅助文字向管理员明确说明避免误期望。

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>"
```

---

### Task 6: 发版 v1.1.9

**Files:**
- 无代码变更，仅构建与打 tag

- [ ] **Step 1: 全量构建验证**

Run: `go build ./... && go vet ./internal/kernel/ ./internal/service/ ./cmd/server/ && cd web && npm run build && cd ..`
Expected: 后端无输出（编译成功），前端 `✓ built`

- [ ] **Step 2: 确认全部 Go 单元测试通过**

Run: `go test ./internal/kernel/ -v`
Expected: 所有 `TestGetSettingInt` + `TestHy2*` 子用例 PASS

- [ ] **Step 3: 本地手工验证**

暂用本地编译替换服务器二进制（与前几次发版流程相同），部署后：

```bash
# 1. 不填任何限速，确认历史行为不变
sqlite3 /opt/proxy-panel/data/panel.db "UPDATE nodes SET settings = '{}' WHERE protocol = 'hysteria2';"
systemctl restart proxy-panel
grep -A 3 hysteria2 /opt/proxy-panel/kernel/singbox.json | grep -E 'up_mbps|down_mbps'
# 期望：无输出

# 2. WebUI 在 Hy2 节点上设最大上行=5、下行=8，保存
#    用户 tttt 的 speed_limit 保持 0
grep -E 'up_mbps|down_mbps' /opt/proxy-panel/kernel/singbox.json
# 期望："up_mbps": 5 / "down_mbps": 8

# 3. 将 tttt 的 speed_limit 设为 3（单用户场景融合）
sqlite3 /opt/proxy-panel/data/panel.db "UPDATE users SET speed_limit = 3 WHERE username='tttt';"
# WebUI 触发节点保存或 systemctl restart proxy-panel
grep -E 'up_mbps|down_mbps' /opt/proxy-panel/kernel/singbox.json
# 期望：up_mbps 和 down_mbps 都是 3（min(3,5)、min(3,8)）

# 4. 客户端实测下载速度（speedtest-cli 或 iperf）
# 期望：≤3 Mbps，允许 ±10% 误差
```

- [ ] **Step 4: Push main + 打 tag 触发 release action**

```bash
git push origin main
git tag -a v1.1.9 -m "$(cat <<'EOF'
v1.1.9

功能：
- Hy2 节点编辑页支持最大上行/下行 Mbps（1-20，默认 10）
- 单用户独享 hy2 节点时，用户 speed_limit 与节点 max 取 min 下发
- UI tooltip 明确 speed_limit 生效条件

详见 specs/2026-04-17-hy2-speed-limit-design.md
EOF
)"
git push origin v1.1.9
```

- [ ] **Step 5: 确认 GitHub Action 构建成功**

Run: `gh run list --workflow=release.yml --limit 1`
Expected: 最新一行状态 `in_progress` → 约 1-2 分钟后再查变 `completed success`
