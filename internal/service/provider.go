// Package service 承载业务逻辑,只依赖 model 层与类型化参数,
// 禁止 import gin / net/http(分层约束见 .trellis/spec/backend/directory-structure.md)。
package service

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/acooler15/modelmeter/internal/apperr"
	"github.com/acooler15/modelmeter/internal/model"
)

// ProviderInput 新增/编辑接口配置的输入。编辑时 APIKey 为空表示沿用原值。
type ProviderInput struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

// ProviderView 接口配置的对外视图。API Key 按产品决策(2026-10-01)明文回显:
// 本项目为本地单用户工具,凭据由用户自录自见,这是"对外一律脱敏视图"规范的
// 有意例外;New API 令牌与 Agent 模型清单凭据仍走脱敏视图,不受此例外影响。
type ProviderView struct {
	ID        uint      `json:"id"`
	Name      string    `json:"name"`
	BaseURL   string    `json:"base_url"`
	APIKey    string    `json:"api_key"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MaskKey 对 API Key 脱敏:长度不超过 8 位时全部替换为 '*',
// 否则保留前 3 位与后 4 位,中间固定输出 '****'。
// 接口配置视图已改为明文回显,该函数目前仅用于 New API 令牌的脱敏展示。
func MaskKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:3] + "****" + key[len(key)-4:]
}

// toProviderView 把模型转换为对外视图;API Key 按新契约明文回显(用户自见
// 凭据的有意例外)。它仍是读路径上唯一接触明文 Key 的出口,写请求、日志与
// 错误信息路径不得输出明文。
func toProviderView(p model.Provider) ProviderView {
	return ProviderView{
		ID:        p.ID,
		Name:      p.Name,
		BaseURL:   p.BaseURL,
		APIKey:    p.APIKey,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}

// ListProviders 返回全部接口配置,按创建时间正序排列。
func ListProviders(ctx context.Context, db *gorm.DB) ([]ProviderView, error) {
	var providers []model.Provider
	if err := db.WithContext(ctx).Order("created_at ASC").Find(&providers).Error; err != nil {
		return nil, apperr.Wrap(apperr.CodeInternal, "查询接口配置失败", err)
	}
	views := make([]ProviderView, 0, len(providers))
	for _, p := range providers {
		views = append(views, toProviderView(p))
	}
	return views, nil
}

// CreateProvider 新增接口配置:三项均必填,名称全局唯一。
func CreateProvider(ctx context.Context, db *gorm.DB, in ProviderInput) (ProviderView, error) {
	if err := validateInput(in, true); err != nil {
		return ProviderView{}, err
	}
	if err := ensureNameAvailable(ctx, db, in.Name, 0); err != nil {
		return ProviderView{}, err
	}
	p := model.Provider{
		Name:    in.Name,
		BaseURL: in.BaseURL,
		APIKey:  in.APIKey,
	}
	if err := db.WithContext(ctx).Create(&p).Error; err != nil {
		return ProviderView{}, apperr.Wrap(apperr.CodeInternal, "保存接口配置失败", err)
	}
	slog.Info("接口配置创建成功", "id", p.ID, "name", p.Name)
	return toProviderView(p), nil
}

// UpdateProvider 编辑接口配置;APIKey 为空表示沿用原值,id 不存在时报 1404。
func UpdateProvider(ctx context.Context, db *gorm.DB, id uint, in ProviderInput) (ProviderView, error) {
	if err := validateInput(in, false); err != nil {
		return ProviderView{}, err
	}
	var p model.Provider
	if err := db.WithContext(ctx).First(&p, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ProviderView{}, apperr.New(apperr.CodeProviderNotFound, "接口配置不存在")
		}
		return ProviderView{}, apperr.Wrap(apperr.CodeInternal, "查询接口配置失败", err)
	}
	if err := ensureNameAvailable(ctx, db, in.Name, p.ID); err != nil {
		return ProviderView{}, err
	}
	p.Name = in.Name
	p.BaseURL = in.BaseURL
	// 编辑留空(含纯空白)表示不修改原 Key,避免误输入空格清掉凭据
	newKey := strings.TrimSpace(in.APIKey)
	if newKey != "" {
		p.APIKey = newKey
	}
	if err := db.WithContext(ctx).Save(&p).Error; err != nil {
		return ProviderView{}, apperr.Wrap(apperr.CodeInternal, "保存接口配置失败", err)
	}
	slog.Info("接口配置更新成功", "id", p.ID, "name", p.Name, "key_updated", newKey != "")
	return toProviderView(p), nil
}

// DeleteProvider 删除接口配置(物理删除),id 不存在时报 1404。
func DeleteProvider(ctx context.Context, db *gorm.DB, id uint) error {
	res := db.WithContext(ctx).Delete(&model.Provider{}, id)
	if res.Error != nil {
		return apperr.Wrap(apperr.CodeInternal, "删除接口配置失败", res.Error)
	}
	if res.RowsAffected == 0 {
		return apperr.New(apperr.CodeProviderNotFound, "接口配置不存在")
	}
	slog.Info("接口配置删除成功", "id", id)
	return nil
}

// validateInput 校验必填与格式;requireKey 区分新增(Key 必填)与编辑(可留空)。
func validateInput(in ProviderInput, requireKey bool) error {
	if strings.TrimSpace(in.Name) == "" {
		return apperr.New(apperr.CodeProviderInvalid, "名称不能为空")
	}
	if strings.TrimSpace(in.BaseURL) == "" {
		return apperr.New(apperr.CodeProviderInvalid, "Base URL 不能为空")
	}
	if !isValidHTTPURL(in.BaseURL) {
		return apperr.New(apperr.CodeProviderInvalid, "Base URL 不是合法的 http(s) 地址")
	}
	if requireKey && strings.TrimSpace(in.APIKey) == "" {
		return apperr.New(apperr.CodeProviderInvalid, "API Key 不能为空")
	}
	return nil
}

// isValidHTTPURL 要求 URL 可解析、scheme 为 http/https 且带主机名,
// 排除 "http://" 这类缺主机的写法。
func isValidHTTPURL(raw string) bool {
	u, err := url.ParseRequestURI(raw)
	if err != nil {
		return false
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return false
	}
	return true
}

// ensureNameAvailable 校验名称全局唯一;excludeID 用于编辑时排除自身,
// 命中同名记录即报 1402。
func ensureNameAvailable(ctx context.Context, db *gorm.DB, name string, excludeID uint) error {
	var count int64
	query := db.WithContext(ctx).Model(&model.Provider{}).Where("name = ?", name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return apperr.Wrap(apperr.CodeInternal, "查询接口配置失败", err)
	}
	if count > 0 {
		return apperr.New(apperr.CodeProviderDuplicate, "名称已存在")
	}
	return nil
}
