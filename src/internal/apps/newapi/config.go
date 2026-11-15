package newapi

import (
	"errors"
	"math"
	"time"

	"hachimodockplus/internal/config"
)

type ProviderConfig struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	config.Service
	Region string `json:"region,omitempty"`
}

type Config struct {
	config.Service
	AdminID        int     `json:"admin_id"`
	TopN           int     `json:"top_n"`
	ResetUSD       float64 `json:"reset_usd"`
	RefreshSeconds int     `json:"refresh_seconds"`
	TimeoutSeconds int     `json:"timeout_seconds"`
	MaxLogs        int     `json:"max_logs"`
	Timezone       string  `json:"timezone"`
	// Legacy fields are accepted so existing board configurations still load.
	ChannelIDs     []int            `json:"channel_ids"`
	HealthyPercent float64          `json:"healthy_percent"`
	Providers      []ProviderConfig `json:"providers"`
}

func LoadConfig(root config.Config) (Config, error) {
	c := Config{AdminID: 1, TopN: 10, ResetUSD: 200, RefreshSeconds: 300, TimeoutSeconds: 20, MaxLogs: 50000, HealthyPercent: 95, Timezone: root.UI.Timezone}
	if err := root.DecodeApp(ID, &c); err != nil {
		return c, err
	}
	c.KeyFile = root.Resolve(c.KeyFile)
	for i := range c.Providers {
		c.Providers[i].KeyFile = root.Resolve(c.Providers[i].KeyFile)
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if err := c.Service.Validate(ID, false); err != nil {
		return err
	}
	if c.AdminID < 1 || c.TopN < 1 || c.TopN > 100 || c.RefreshSeconds < 30 || c.TimeoutSeconds < 1 || c.TimeoutSeconds > 120 || c.MaxLogs < 100 || c.MaxLogs > 1000000 {
		return errors.New("NewAPI: admin_id、top_n(1~100)、刷新间隔(≥30秒)、超时(1~120秒)或日志上限(100~1000000)无效")
	}
	if math.IsNaN(c.ResetUSD) || math.IsInf(c.ResetUSD, 0) || c.ResetUSD < 0 || c.ResetUSD > 1000000 || math.IsNaN(c.HealthyPercent) || c.HealthyPercent < 0 || c.HealthyPercent > 100 {
		return errors.New("NewAPI: 重置美元额度或健康度阈值无效")
	}
	if _, err := time.LoadLocation(c.Timezone); err != nil {
		return errors.New("NewAPI: 时区无效")
	}
	seen := map[int]bool{}
	for _, id := range c.ChannelIDs {
		if id < 1 || seen[id] {
			return errors.New("NewAPI: 渠道 ID 必须为不重复的正整数")
		}
		seen[id] = true
	}
	if len(c.Providers) > 16 {
		return errors.New("NewAPI: 最多配置16个提供商账户")
	}
	for _, p := range c.Providers {
		if p.Kind != "aliyun-token-plan" && p.Kind != "mimo-token-plan" {
			return errors.New("NewAPI: 不支持的提供商类型")
		}
		if err := p.Service.Validate(p.Kind, false); err != nil {
			return err
		}
		if p.Kind == "aliyun-token-plan" && p.Region != "" && p.Region != "cn-beijing" && p.Region != "ap-southeast-1" {
			return errors.New("NewAPI: 阿里地域必须为 cn-beijing 或 ap-southeast-1")
		}
	}
	return nil
}
