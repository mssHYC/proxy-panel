package subscription

import "proxy-panel/internal/model"

// Generator 订阅内容生成器接口
type Generator interface {
	Generate(nodes []model.Node, user *model.User, baseURL string) (content string, contentType string, err error)
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
