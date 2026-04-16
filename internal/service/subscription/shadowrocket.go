package subscription

import (
	"encoding/base64"
	"strings"

	"proxy-panel/internal/model"
)

// ShadowrocketGenerator Shadowrocket 格式订阅生成器（复用 V2Ray URI）
type ShadowrocketGenerator struct{}

func (g *ShadowrocketGenerator) Generate(nodes []model.Node, user *model.User, baseURL string) (string, string, error) {
	uris := buildV2RayURIs(nodes, user)
	raw := strings.Join(uris, "\n")
	encoded := base64.StdEncoding.EncodeToString([]byte(raw))
	return encoded, "text/plain; charset=utf-8", nil
}
