package service

import (
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
)
