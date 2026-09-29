// treeutil.go 各 Agent 实现共用的 JSON 树遍历与编辑辅助。
// 写回采用 map[string]any 树整体重编码:配合 json.Decoder 的 UseNumber,
// 任意数字字面量零精度丢失;白名单之外的节点只要不触碰即原样保留。
package agents

import (
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/acooler15/modelmeter/internal/service/agentconf"
)

// mapGet 沿 keys 逐层取嵌套对象字段;任一层缺失或不是对象时 ok=false。
func mapGet(obj map[string]any, keys ...string) (any, bool) {
	cur := any(obj)
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// mapGetObj 取嵌套对象并断言为 map;缺失或类型不符时返回 nil。
func mapGetObj(obj map[string]any, keys ...string) map[string]any {
	v, ok := mapGet(obj, keys...)
	if !ok {
		return nil
	}
	m, _ := v.(map[string]any)
	return m
}

// ensureMap 取 obj[key] 为对象;不存在或类型不符时创建空对象并回填,
// 供写回侧补齐缺失的中间节点。
func ensureMap(obj map[string]any, key string) map[string]any {
	if m, ok := obj[key].(map[string]any); ok {
		return m
	}
	m := map[string]any{}
	obj[key] = m
	return m
}

// numberValue 把任意提交值规范为 json.Number;非数字(含 NaN/Inf)报失败。
// 整数值的 float64 落成整数形式,避免 128000 被编码成 1.28e+05。
func numberValue(v any) (json.Number, bool) {
	switch n := v.(type) {
	case json.Number:
		return n, true
	case int:
		return json.Number(strconv.Itoa(n)), true
	case int64:
		return json.Number(strconv.FormatInt(n, 10)), true
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return "", false
		}
		if n == math.Trunc(n) && math.Abs(n) < 1e15 {
			return json.Number(strconv.FormatInt(int64(n), 10)), true
		}
		return json.Number(strconv.FormatFloat(n, 'g', -1, 64)), true
	}
	return "", false
}

// stringOf 宽容取字符串:缺失或非字符串返回空串。
func stringOf(v any) string {
	s, _ := v.(string)
	return s
}

// boolOf 宽容取布尔:缺失或非布尔时 ok=false,由调用方决定缺省值。
func boolOf(v any) (b, ok bool) {
	b, ok = v.(bool)
	return b, ok
}

// anySlice 宽容取数组:缺失或非数组返回 nil。
func anySlice(v any) []any {
	arr, _ := v.([]any)
	return arr
}

// stringSliceOf 把 JSON 数组宽容转换为字符串切片,跳过非字符串元素。
func stringSliceOf(v any) []string {
	arr := anySlice(v)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// isStringSlice 判断提交值是否为字符串数组(接受 []string 与元素全为字符串的 []any)。
func isStringSlice(v any) bool {
	switch s := v.(type) {
	case []string:
		return true
	case []any:
		for _, e := range s {
			if _, ok := e.(string); !ok {
				return false
			}
		}
		return true
	}
	return false
}

// isNonEmptyStringSlice 判断提交值是否为元素非空字符串的数组;比 isStringSlice
// 更严,供不允许空串档位值的 list 列(如 reasoning_levels)校验使用。
func isNonEmptyStringSlice(v any) bool {
	switch s := v.(type) {
	case []string:
		for _, e := range s {
			if e == "" {
				return false
			}
		}
		return true
	case []any:
		for _, e := range s {
			if str, ok := e.(string); !ok || str == "" {
				return false
			}
		}
		return true
	}
	return false
}

// stringSliceValue 把提交的字符串数组规范为 []any(与解析树元素类型一致),
// 元素已在校验阶段确认全为字符串;供 list 列落盘使用。
func stringSliceValue(v any) []any {
	switch s := v.(type) {
	case []string:
		out := make([]any, len(s))
		for i, e := range s {
			out[i] = e
		}
		return out
	case []any:
		return s
	}
	return nil
}

// boolDefault 宽容取布尔:缺失或非布尔返回 false。
func boolDefault(v any) bool {
	b, _ := boolOf(v)
	return b
}

// patchChangedKeys 汇总 patch 改动的字段键名(仅键名,不含值)供日志使用,
// 避免日志中出现任何提交值。
func patchChangedKeys(patches []agentconf.ModelPatch) string {
	seen := map[string]bool{}
	keys := make([]string, 0, len(patches))
	for _, p := range patches {
		for k := range p.Fields {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}
