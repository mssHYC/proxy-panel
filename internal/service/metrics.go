package service

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 面板暴露的 Prometheus 指标。使用包级变量 + promauto 自动注册到 DefaultRegisterer，
// 保证 /metrics 可直接通过 promhttp.Handler() 暴露而无需显式 Register 调用。
var (
	// HTTPRequestsTotal 按 method/path/status 维度累计 API 请求量
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_panel_http_requests_total",
		Help: "面板 HTTP 请求总数",
	}, []string{"method", "path", "status"})

	// AlertsSentTotal 已发送的告警条数（按 type 维度）
	AlertsSentTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_panel_alerts_sent_total",
		Help: "告警发送计数",
	}, []string{"type"})

	// NodeHealth 节点健康状态（1=在线，0=离线/未知）
	NodeHealth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proxy_panel_node_health",
		Help: "节点健康状态，1 表示在线，0 表示离线",
	}, []string{"node", "protocol"})

	// NodeFailCount 节点累计探测失败次数
	NodeFailCount = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "proxy_panel_node_fail_count",
		Help: "节点当前连续失败次数",
	}, []string{"node"})

	// SubscriptionRequestsTotal 按客户端类型与结果分类记订阅请求数。
	// client 限定为已知的 5 种格式（clash/sing-box/surge/v2ray/shadowrocket）+ unknown，
	// 避免 User-Agent 不可控带来的标签基数爆炸。
	SubscriptionRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_panel_subscription_requests_total",
		Help: "订阅请求总数",
	}, []string{"client", "status"})

	// KernelSyncDuration 内核同步（生成 + 写盘 + 重启）耗时分布
	KernelSyncDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "proxy_panel_kernel_sync_duration_seconds",
		Help:    "内核同步耗时（生成配置 + ApplyConfig）",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30},
	}, []string{"kernel"})

	// KernelSyncFailuresTotal 内核同步失败计数。reason ∈ generate/apply/rolled_back
	KernelSyncFailuresTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_panel_kernel_sync_failures_total",
		Help: "内核同步失败计数",
	}, []string{"kernel", "reason"})

	// NodeTrafficBytes 节点累计上下行字节。
	// 不导出 username 维度——username 可能是邮箱/手机号/真名，
	// 暴露到 /metrics 等于把敏感数据扩散到 Prometheus / Grafana；
	// 同时用户数会随增删改名长期累积，制造高基数 time series。
	// 节点维度用 node_id 数字，基数与节点数同阶且无敏感信息。
	NodeTrafficBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_panel_node_traffic_bytes_total",
		Help: "节点累计流量字节",
	}, []string{"node", "direction"})

	// ServerTrafficBytes 全局上下行字节，用作单机/小集群场景下的总量观测
	ServerTrafficBytes = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "proxy_panel_server_traffic_bytes_total",
		Help: "面板累计流量字节（所有用户/节点求和）",
	}, []string{"direction"})
)

// known subscription client labels（白名单，未识别归一为 unknown 防止标签基数爆炸）。
// 与 subscription.GetGenerator / SniffFormat 返回值保持一致（注意是 singbox 而非 sing-box）。
var knownSubscriptionClients = map[string]struct{}{
	"clash":        {},
	"singbox":      {},
	"surge":        {},
	"v2ray":        {},
	"shadowrocket": {},
}

// NormalizeSubscriptionClient 将订阅 format 字符串归一化为标签值。
// 大小写不敏感、自动 trim，未识别归一为 unknown。
func NormalizeSubscriptionClient(format string) string {
	f := strings.ToLower(strings.TrimSpace(format))
	if _, ok := knownSubscriptionClients[f]; ok {
		return f
	}
	return "unknown"
}
