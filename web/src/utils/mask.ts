/**
 * 前端展示层打码:仅用于界面渲染。后端返回的 api_key 仍是完整明文,
 * 复制、编辑回填等需要真实值的功能不受影响,故不改动数据层只改展示。
 *
 * 口径与后端 New API 令牌脱敏(internal/service/provider.go 的 MaskKey)
 * 保持一致,避免同一凭据在不同页面呈现两种打码形态:
 * 长度 ≤8 全部替换为 *;否则保留前 3 位与后 4 位,中间固定 4 个 *
 * (如 sk-****klmn)。
 * 后端按字节截取,此处按字符截取;API Key 均为 ASCII 字符,结果一致。
 */
export function maskKey(key: string): string {
  if (key.length <= 8) {
    return '*'.repeat(key.length)
  }
  return `${key.slice(0, 3)}****${key.slice(-4)}`
}
