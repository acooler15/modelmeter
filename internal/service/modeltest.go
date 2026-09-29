// modeltest.go 模型测试业务:校验测试参数、经 llmclient 代理三种协议的
// 对话请求(全量/流式),成功与失败都落 TestRecord 并只保留最近 200 条。
package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
	"github.com/acooler15/modelmeter/internal/service/llmclient"
)

// maxTestRecords 测试记录保留上限:超出后按 created_at 倒序清理旧记录。
const maxTestRecords = 200

// TestInput 模型测试输入:接口配置 ID + 对话参数。
// 内嵌 llmclient.ChatRequest 使请求体字段与前端一致(provider_id + 协议参数)。
type TestInput struct {
	ProviderID uint `json:"provider_id"`
	llmclient.ChatRequest
}

// validate 校验测试参数;不合法时报 2401,文案与缺失项一一对应。
func (in TestInput) validate() error {
	if in.ProviderID == 0 {
		return apperr.New(apperr.CodeTestInvalid, "请选择接口配置")
	}
	if strings.TrimSpace(in.Model) == "" {
		return apperr.New(apperr.CodeTestInvalid, "模型不能为空")
	}
	if strings.TrimSpace(in.User) == "" {
		return apperr.New(apperr.CodeTestInvalid, "用户消息不能为空")
	}
	if !in.Protocol.Valid() {
		return apperr.New(apperr.CodeTestInvalid, "不支持的对话协议")
	}
	return nil
}

// TestErrMessage 提取面向用户的中文错误文案;非业务错误返回通用文案,
// 避免把技术细节(含上游原始信息)写进记录或推给前端。
func TestErrMessage(err error) string {
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return ae.Message
	}
	return "测试失败,发生未知错误"
}

// RunTest 非流式测试:查配置 → 全量请求 → 无论成败都落 TestRecord。
// 返回结果与记录;测试失败时 err 非 nil(此时 Result 为零值,记录仍已落库)。
func RunTest(ctx context.Context, db *gorm.DB, in TestInput) (llmclient.Result, *model.TestRecord, error) {
	in.Stream = false // 非流式入口与请求参数无关,强制按非流式执行与记录
	return executeTest(ctx, db, in, nil)
}

// StreamTest 流式测试:onDelta 在收到每个内容增量时被同步调用。
// 推送过程中出错时同样落失败记录;返回值语义与 RunTest 一致。
func StreamTest(ctx context.Context, db *gorm.DB, in TestInput, onDelta func(string)) (llmclient.Result, *model.TestRecord, error) {
	in.Stream = true
	return executeTest(ctx, db, in, onDelta)
}

// executeTest 测试主流程:参数校验 → 查配置 → 调上游 → 落记录 → 清理超限。
// onDelta 非 nil 时走流式。校验失败与配置不存在发生在记录生成之前,不落记录。
func executeTest(ctx context.Context, db *gorm.DB, in TestInput, onDelta func(string)) (llmclient.Result, *model.TestRecord, error) {
	if err := in.validate(); err != nil {
		return llmclient.Result{}, nil, err
	}
	var p model.Provider
	if err := db.WithContext(ctx).First(&p, in.ProviderID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return llmclient.Result{}, nil, apperr.New(apperr.CodeProviderNotFound, "接口配置不存在")
		}
		return llmclient.Result{}, nil, apperr.Wrap(apperr.CodeInternal, "查询接口配置失败", err)
	}

	cfg := llmclient.UpstreamConfig{BaseURL: p.BaseURL, APIKey: p.APIKey}
	record := newTestRecord(in, p.Name)
	var (
		result llmclient.Result
		err    error
	)
	if onDelta != nil {
		result, err = llmclient.StreamChat(ctx, cfg, in.ChatRequest, onDelta)
	} else {
		result, err = llmclient.DoChat(ctx, cfg, in.ChatRequest)
	}

	if err != nil {
		record.Succeeded = false
		record.ErrMessage = TestErrMessage(err)
		// 用户主动中断:上游错误不可归类,记录文案改为明确的中文提示
		if ctx.Err() != nil {
			record.ErrMessage = "测试已被用户中断"
		}
	} else {
		record.Succeeded = true
		record.Reply = result.Reply
		record.FirstLatencyMs = result.FirstLatencyMs
		record.TotalLatencyMs = result.TotalLatencyMs
		record.PromptTokens = result.Usage.Prompt
		record.CompletionTokens = result.Usage.Completion
		record.TotalTokens = result.Usage.Total
	}

	// 落库使用脱离取消信号的 ctx:用户中断后失败记录仍需留存
	persistCtx := context.WithoutCancel(ctx)
	if dbErr := db.WithContext(persistCtx).Create(record).Error; dbErr != nil {
		slog.Error("测试记录落库失败", "err", dbErr)
	}
	trimTestRecords(persistCtx, db)
	return result, record, err
}

// newTestRecord 按输入构造待落库的记录;API Key 等凭据不进记录字段。
func newTestRecord(in TestInput, providerName string) *model.TestRecord {
	return &model.TestRecord{
		ProviderID:   in.ProviderID,
		ProviderName: providerName,
		Protocol:     string(in.Protocol),
		Model:        in.Model,
		SystemPrompt: in.System,
		UserMessage:  in.User,
		Temperature:  in.Temperature,
		MaxTokens:    in.MaxTokens,
		Stream:       in.Stream,
	}
}

// trimTestRecords 只保留最近 200 条测试记录(成败都算),按 created_at 倒序、
// id 倒序兜底;清理失败只记日志,不影响测试主流程。
func trimTestRecords(ctx context.Context, db *gorm.DB) {
	keep := db.WithContext(ctx).Model(&model.TestRecord{}).
		Order("created_at DESC, id DESC").Limit(maxTestRecords).Select("id")
	if err := db.WithContext(ctx).
		Where("id NOT IN (?)", keep).
		Delete(&model.TestRecord{}).Error; err != nil {
		slog.Error("清理历史测试记录失败", "err", err)
	}
}

// ListTestRecords 按创建时间倒序返回最近 limit 条测试记录;limit 上限 200。
func ListTestRecords(ctx context.Context, db *gorm.DB, limit int) ([]model.TestRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > maxTestRecords {
		limit = maxTestRecords
	}
	var records []model.TestRecord
	if err := db.WithContext(ctx).
		Order("created_at DESC, id DESC").
		Limit(limit).
		Find(&records).Error; err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "查询测试记录失败", err)
	}
	return records, nil
}

// ClearTestRecords 清空全部测试记录,返回删除条数。
func ClearTestRecords(ctx context.Context, db *gorm.DB) (int64, error) {
	// AllowGlobalUpdate 放行无条件删除:清空是显式业务动作
	res := db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.TestRecord{})
	if res.Error != nil {
		return 0, apperr.Wrap(apperr.CodeInternal, "清空测试记录失败", res.Error)
	}
	slog.Info("测试记录已清空", "deleted", res.RowsAffected)
	return res.RowsAffected, nil
}
