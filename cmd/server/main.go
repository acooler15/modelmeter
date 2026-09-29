// 服务入口:装配配置、日志、数据库、路由与前端托管,启动 HTTP 服务。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/config"
	"github.com/acooler15/modelmeter/internal/handler"
	"github.com/acooler15/modelmeter/internal/model"
	"github.com/acooler15/modelmeter/internal/web"
)

func main() {
	// 访问日志由自有中间件输出,关闭 gin 的 debug 模式横幅
	gin.SetMode(gin.ReleaseMode)

	cfg := config.Load()
	setupLogger(cfg)

	// 优雅停机:收到中断/终止信号后停止接收新请求,处理完存量请求再退出
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db := mustOpenDB(cfg)

	// dataDir 供 Agent 配置的备份/还原使用(备份落在 dataDir/agent-backups/)
	r := handler.NewRouter(db, cfg.DataDir)
	web.Register(r)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: r,
	}
	go func() {
		slog.Info("服务已启动", "port", cfg.Port, "data_dir", cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP 服务异常退出", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("收到退出信号,开始优雅停机")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("优雅停机超时,强制退出", "err", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		if err := sqlDB.Close(); err != nil {
			slog.Warn("关闭数据库失败", "err", err)
		}
	}
	slog.Info("服务已停止")
}

// setupLogger 初始化全局 slog。仅允许在 main.go 调用 slog.SetDefault
// (见 .trellis/spec/backend/quality-guidelines.md)。
func setupLogger(cfg config.Config) {
	level := slog.LevelInfo
	if err := level.UnmarshalText([]byte(cfg.LogLevel)); err != nil {
		// 级别写错时回退 info,不阻断启动
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.LogFormat == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}

// mustOpenDB 打开 SQLite 数据库;失败属于启动期致命错误,直接终止进程。
// 全项目只允许此处调用 gorm.Open,db 向下注入(service → model)。
func mustOpenDB(cfg config.Config) *gorm.DB {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		slog.Error("创建数据目录失败", "dir", cfg.DataDir, "err", err)
		os.Exit(1)
	}
	dsn := filepath.Join(cfg.DataDir, "modelmeter.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		// 时间字段统一以 UTC 写入(见 .trellis/spec/backend/database-guidelines.md)
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		slog.Error("打开数据库失败", "dsn", dsn, "err", err)
		os.Exit(1)
	}
	// 表结构变更走启动时 AutoMigrate;新增模型必须在此登记
	// (见 .trellis/spec/backend/database-guidelines.md)
	if err := db.AutoMigrate(&model.Provider{}, &model.TestRecord{}, &model.Setting{}); err != nil {
		slog.Error("数据库迁移失败", "err", err)
		os.Exit(1)
	}
	return db
}
