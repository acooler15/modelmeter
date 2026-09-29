// Package config 负责在启动时把环境变量解析为类型化的运行配置。
package config

import (
	"os"
	"strconv"
)

// Config 服务运行配置,全部来自环境变量,启动时一次性解析。
type Config struct {
	Port      int    // HTTP 监听端口,默认 8422(避开 8080 等常用端口)
	DataDir   string // SQLite 数据目录,默认 data
	LogLevel  string // 日志级别:debug/info/warn/error,默认 info
	LogFormat string // 日志格式:json/text,默认 json
}

// Load 读取环境变量并填充配置;非法取值回退默认值,保证服务总能启动。
func Load() Config {
	cfg := Config{
		Port:      8422,
		DataDir:   "data",
		LogLevel:  "info",
		LogFormat: "json",
	}

	if v := os.Getenv("PORT"); v != "" {
		// 端口非法(非数字/越界)时保持默认,避免因配置笔误导致启动失败
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			cfg.Port = p
		}
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("LOG_FORMAT"); v != "" {
		cfg.LogFormat = v
	}
	return cfg
}
