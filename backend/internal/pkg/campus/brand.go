// Package campus 校园品牌注册表：MUC / HUBU 共用同一套 connect-code 机制，
// 差异全部收敛在这里 —— 新增学校 = 新增一个 Brand 值 + 一组路由，不复制 handler。
package campus

// Brand 校园品牌档案（HUBU_LOCAL_IMPLEMENTATION_PLAN §1）。
type Brand struct {
	ID          string // 稳定标识："muc" / "hubu"
	PathSegment string // API 路径段：/api/v1/{PathSegment}/*
	Name        string // 中文名
	EnglishName string
	ShortName   string // HUBU
	ProductName string // 桌面/终端产品名
	Motto       string // 校训
	Founded     string // 建校年份

	ProtocolScheme string // 深链协议：muc:// / hubu://
	RedisPrefix    string // connect-code 在 Redis 的 key 前缀
	KeyPrefix      string // per-device API Key 名前缀（"MUC <device>"）
	DeviceHint     string // 客户端未传设备名时的默认 Key 名
}

// MUC 中央民族大学（既有品牌，字段与历史行为逐字节对齐，勿改动取值）。
var MUC = Brand{
	ID:             "muc",
	PathSegment:    "muc",
	Name:           "中央民族大学",
	EnglishName:    "Minzu University of China",
	ShortName:      "MUC",
	ProductName:    "MUC AI Harness",
	Motto:          "美美与共，知行合一",
	Founded:        "1941",
	ProtocolScheme: "muc",
	RedisPrefix:    "muc:code:",
	KeyPrefix:      "MUC ",
	DeviceHint:     "MUC Desktop",
}

// HUBU 湖北大学（2026-09 新增，仅本地开发）。
var HUBU = Brand{
	ID:             "hubu",
	PathSegment:    "hubu",
	Name:           "湖北大学",
	EnglishName:    "Hubei University",
	ShortName:      "HUBU",
	ProductName:    "HUBU AI",
	Motto:          "日思日睿，笃志笃行",
	Founded:        "1931",
	ProtocolScheme: "hubu",
	RedisPrefix:    "hubu:code:",
	KeyPrefix:      "HUBU ",
	DeviceHint:     "HUBU AI Desktop",
}

// All 全部已注册品牌（路由注册等处遍历用）。
var All = []Brand{MUC, HUBU}

// ByID 按 ID 查品牌；未注册返回 false。
func ByID(id string) (Brand, bool) {
	for _, b := range All {
		if b.ID == id {
			return b, true
		}
	}
	return Brand{}, false
}
