// Package secrets 承载百度智能云等第三方凭据。
package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	localConfigDirName  = "config"
	localConfigFileName = "config.json"
)

// BaiduConfig 百度智能云应用 AK/SK（控制台「API Key」与「Secret Key」）。
// 获取 access_token 时分别对应 client_id、client_secret。
type BaiduConfig struct {
	APIKey    string `json:"apiKey"`
	SecretKey string `json:"secretKey"`
}

// Config 应用级密钥集合。
type Config struct {
	Baidu BaiduConfig `json:"baidu"`
}

// UpscaleReady 是否具备调用「图像清晰度增强」所需字段。
func (c *Config) UpscaleReady() bool {
	if c == nil {
		return false
	}
	return strings.TrimSpace(c.Baidu.APIKey) != "" &&
		strings.TrimSpace(c.Baidu.SecretKey) != ""
}

// Load 解析本地配置文件；文件不存在时返回 (nil, nil)。
func Load() (*Config, error) {
	path := LocalConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("secrets: read local config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("secrets: parse local json: %w", err)
	}
	return &cfg, nil
}

// Save 写入本地配置文件（目录不存在会自动创建）。
func Save(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("secrets: nil config")
	}
	cfg.Baidu.APIKey = strings.TrimSpace(cfg.Baidu.APIKey)
	cfg.Baidu.SecretKey = strings.TrimSpace(cfg.Baidu.SecretKey)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("secrets: marshal local config: %w", err)
	}
	path := LocalConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("secrets: mkdir local dir: %w", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		return fmt.Errorf("secrets: write local config: %w", err)
	}
	return nil
}

// LocalConfigPath 返回本地配置文件路径（位于二进制同级 config 目录）。
func LocalConfigPath() string {
	exePath, err := os.Executable()
	if err != nil {
		return filepath.Join(localConfigDirName, localConfigFileName)
	}
	exeDir := filepath.Dir(exePath)
	return filepath.Join(exeDir, localConfigDirName, localConfigFileName)
}

// UpscaleNotReadyHint 说明未配置完整时的原因（供 UI / 导出报错使用）。
func UpscaleNotReadyHint(c *Config) string {
	if c == nil {
		return "未找到本地配置（请点击“配置百度云”并保存）"
	}
	var miss []string
	if strings.TrimSpace(c.Baidu.APIKey) == "" {
		miss = append(miss, "baidu.apiKey")
	}
	if strings.TrimSpace(c.Baidu.SecretKey) == "" {
		miss = append(miss, "baidu.secretKey")
	}
	if len(miss) == 0 {
		return "配置异常（应为内部错误）"
	}
	return "缺少或为空: " + strings.Join(miss, ", ")
}
