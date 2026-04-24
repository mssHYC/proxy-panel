package subscription

import (
	"proxy-panel/internal/model"
	"proxy-panel/internal/service/routing"
)

// Generator 订阅内容生成器接口
type Generator interface {
	Generate(nodes []model.Node, user *model.User, baseURL string) (content string, contentType string, err error)
}

// RoutingAwareGenerator 是能够消费预构建 routing.Plan 的生成器。
// 订阅 handler 在调用 BuildPlan 后，对 Generator 做类型断言以调用此接口。
type RoutingAwareGenerator interface {
	Generator
	GenerateWithPlan(plan *routing.Plan, nodes []model.Node, user *model.User, baseURL string) (content string, contentType string, err error)
}

// GetGenerator 根据格式名称返回对应的生成器
func GetGenerator(format string) Generator {
	switch format {
	case "surge":
		return &SurgeGenerator{}
	case "clash":
		return &ClashGenerator{}
	case "v2ray":
		return &V2RayGenerator{}
	case "shadowrocket":
		return &ShadowrocketGenerator{}
	case "singbox":
		return &SingboxGenerator{}
	default:
		return &V2RayGenerator{}
	}
}
