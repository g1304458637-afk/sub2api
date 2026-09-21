// Package campus 校园品牌注册表：MUC / HUBU 共用同一套 connect-code 机制，
// 差异全部收敛在这里 —— 新增学校 = 新增一个 Brand 值 + 一组路由，不复制 handler。
package campus

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

// Brand 校园品牌档案（HUBU_LOCAL_IMPLEMENTATION_PLAN §1）。
type Brand struct {
	SiteName            string `json:"site_name"`
	Logo                string `json:"logo"`
	Favicon             string `json:"favicon"`
	PrimaryColor        string `json:"primary_color"`
	PublicURL           string `json:"public_url"`
	GatewayURL          string `json:"gateway_url"`
	AdminURL            string `json:"admin_url"`
	DownloadBaseURL     string `json:"download_base_url"`
	ManifestPath        string `json:"manifest_path"`
	EducationDomain     string `json:"education_email_domain"`
	DeploymentNamespace string `json:"deployment_namespace"`

	ID          string `json:"id"`           // 稳定标识："muc" / "hubu"
	PathSegment string `json:"path_segment"` // API 路径段：/api/v1/{PathSegment}/*
	Name        string `json:"school_name"`  // 中文名
	EnglishName string `json:"english_name"`
	ShortName   string `json:"short_name"`   // HUBU
	ProductName string `json:"product_name"` // 桌面/终端产品名
	Motto       string `json:"motto"`        // 校训
	Founded     string `json:"founded"`      // 建校年份

	ProtocolScheme string `json:"connect_scheme"` // 深链协议：muc:// / hubu://
	RedisPrefix    string `json:"redis_prefix"`   // connect-code 在 Redis 的 key 前缀
	KeyPrefix      string `json:"key_prefix"`     // per-device API Key 名前缀（"MUC <device>"）
	DeviceHint     string `json:"device_hint"`    // 客户端未传设备名时的默认 Key 名
}

// MUC 中央民族大学（既有品牌，字段与历史行为逐字节对齐，勿改动取值）。
var MUC = Brand{
	GatewayURL: "https://admin.wuxuexi.top",
	SiteName:   "中央民族大学 AI 服务平台", Logo: "/campus-assets/muc.svg", Favicon: "/campus-assets/muc.svg", PrimaryColor: "#AC0E0F",
	ManifestPath: "/downloads/latest-mucode.json", EducationDomain: "muc.edu.cn", DeploymentNamespace: "muc",

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
	SiteName: "湖北大学 AI 服务平台", Logo: "/campus-assets/hubu.svg", Favicon: "/campus-assets/hubu.svg", PrimaryColor: "#135440",
	ManifestPath: "/downloads/latest-hubu-ai.json", DeploymentNamespace: "hubu",

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

// Current is deployment configuration, never a user-selected request parameter.
// Unknown identities fail closed at startup instead of silently adopting MUC.
func Current() Brand {
	id := strings.ToLower(strings.TrimSpace(os.Getenv("BRAND")))
	if id == "" {
		id = "muc"
	}
	brand, ok := ByID(id)
	if !ok {
		panic("unsupported campus BRAND")
	}
	prefix := strings.ToUpper(id)
	values := map[string]*string{
		"PUBLIC_URL": &brand.PublicURL, "GATEWAY_URL": &brand.GatewayURL,
		"ADMIN_URL": &brand.AdminURL, "DOWNLOAD_BASE_URL": &brand.DownloadBaseURL,
	}
	for name, destination := range values {
		if value := strings.TrimSpace(os.Getenv(prefix + "_" + name)); value != "" {
			parsed, err := url.Parse(value)
			if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
				panic(fmt.Sprintf("invalid %s_%s", prefix, name))
			}
			if parsed.Scheme == "http" && (os.Getenv("CAMPUS_LOCAL_BUILD") != "1" || (parsed.Hostname() != "localhost" && parsed.Hostname() != "127.0.0.1" && parsed.Hostname() != "::1")) {
				panic("campus endpoints require HTTPS")
			}
			if name == "GATEWAY_URL" && (parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "") {
				panic("campus gateway must be an origin")
			}
			*destination = strings.TrimRight(value, "/")
		}
	}
	if value := strings.TrimSpace(os.Getenv(prefix + "_EDUCATION_EMAIL_DOMAIN")); value != "" {
		if strings.ContainsAny(value, "/@ :") || !strings.Contains(value, ".") {
			panic("invalid campus education email domain")
		}
		brand.EducationDomain = strings.ToLower(value)
	}
	return brand
}
func (b Brand) Audience() string { return b.ID + ":desktop" }
