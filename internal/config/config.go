package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// 与 config.example.yaml 一致的占位值；运行时若保留这些默认值，说明尚未完成基础初始化
const (
	defaultAdminPass = "admin123"
	defaultJWTSecret = "change-me-to-random-32-bytes"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Traffic  TrafficConfig  `yaml:"traffic"`
	Notify   NotifyConfig   `yaml:"notify"`
	Kernel   KernelConfig   `yaml:"kernel"`
	Firewall FirewallConfig `yaml:"firewall"`
}

type ServerConfig struct {
	Port   int    `yaml:"port"`
	TLS    bool   `yaml:"tls"`
	Cert   string `yaml:"cert"`
	Key    string `yaml:"key"`
	Domain string `yaml:"domain"`
	// 反代部署时的可信上游列表；未配置时禁用 X-Forwarded-For 解析（仅信任直连 RemoteAddr）
	// 例如：["127.0.0.1", "10.0.0.0/8"]
	TrustedProxies []string `yaml:"trusted_proxies"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type AuthConfig struct {
	JWTSecret   string `yaml:"jwt_secret"`
	AdminUser   string `yaml:"admin_user"`
	AdminPass   string `yaml:"admin_pass"`
	TokenExpiry int    `yaml:"token_expiry_hours"`
}

type TrafficConfig struct {
	CollectInterval int    `yaml:"collect_interval_sec"`
	ServerLimitGB   int    `yaml:"server_limit_gb"`
	WarnPercent     int    `yaml:"warn_percent"`
	ResetCron       string `yaml:"reset_cron"`
}

type NotifyConfig struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Wechat   WechatConfig   `yaml:"wechat"`
}

type TelegramConfig struct {
	Enable   bool   `yaml:"enable"`
	BotToken string `yaml:"bot_token"`
	ChatID   string `yaml:"chat_id"`
}

type WechatConfig struct {
	Enable     bool   `yaml:"enable"`
	WebhookURL string `yaml:"webhook_url"`
}

type KernelConfig struct {
	XrayPath       string `yaml:"xray_path"`
	XrayConfig     string `yaml:"xray_config"`
	XrayAPIPort    int    `yaml:"xray_api_port"`
	SingboxPath    string `yaml:"singbox_path"`
	SingboxConfig  string `yaml:"singbox_config"`
	SingboxAPIPort int    `yaml:"singbox_api_port"`
}

type FirewallConfig struct {
	Enable  bool   `yaml:"enable"`
	Backend string `yaml:"backend"`
}

// Load 从指定路径加载 YAML 配置文件，并返回解析后的 Config 结构体
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// 设置默认值
	cfg := &Config{
		Server:   ServerConfig{Port: 8080},
		Database: DatabaseConfig{Path: "data/panel.db"},
		Auth:     AuthConfig{TokenExpiry: 24},
		Traffic:  TrafficConfig{CollectInterval: 60, WarnPercent: 80},
		Kernel:   KernelConfig{XrayAPIPort: 10085, SingboxAPIPort: 9090},
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate 校验关键安全字段是否已脱离示例默认值
// 任一默认凭证/密钥命中即拒绝启动，避免生产裸奔
func (c *Config) Validate() error {
	if c.Auth.JWTSecret == "" || c.Auth.JWTSecret == defaultJWTSecret {
		return fmt.Errorf("auth.jwt_secret 仍为默认占位值，请修改 config.yaml 设为 32+ 字节的随机字符串后重启")
	}
	if len(c.Auth.JWTSecret) < 16 {
		return fmt.Errorf("auth.jwt_secret 长度过短（%d 字节），建议至少 32 字节", len(c.Auth.JWTSecret))
	}
	if c.Auth.AdminPass == "" || c.Auth.AdminPass == defaultAdminPass {
		return fmt.Errorf("auth.admin_pass 仍为默认占位值 %q，请修改 config.yaml 设置强密码后重启", defaultAdminPass)
	}
	return nil
}
