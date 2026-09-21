//go:build unit

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────── 专用 repo stub ───────────────────────────

// webChatSettingRepoStub 支持 GetValue/GetAll/SetMultiple 的设置仓库桩：
// SetMultiple 直接合并进 values，天然支持 parse → update → parse 的 round-trip 断言。
type webChatSettingRepoStub struct {
	values map[string]string
}

func (s *webChatSettingRepoStub) Get(ctx context.Context, key string) (*Setting, error) {
	panic("unexpected Get call")
}

func (s *webChatSettingRepoStub) GetValue(ctx context.Context, key string) (string, error) {
	if v, ok := s.values[key]; ok {
		return v, nil
	}
	return "", ErrSettingNotFound
}

func (s *webChatSettingRepoStub) Set(ctx context.Context, key, value string) error {
	panic("unexpected Set call")
}

func (s *webChatSettingRepoStub) GetMultiple(ctx context.Context, keys []string) (map[string]string, error) {
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if v, ok := s.values[key]; ok {
			result[key] = v
		}
	}
	return result, nil
}

func (s *webChatSettingRepoStub) SetMultiple(ctx context.Context, settings map[string]string) error {
	for k, v := range settings {
		s.values[k] = v
	}
	return nil
}

func (s *webChatSettingRepoStub) GetAll(ctx context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for k, v := range s.values {
		out[k] = v
	}
	return out, nil
}

func (s *webChatSettingRepoStub) Delete(ctx context.Context, key string) error {
	panic("unexpected Delete call")
}

// ─────────────────────────── 依赖接口 stub ───────────────────────────

type webChatAPIKeyServiceStub struct {
	visibility     map[int64]struct{}
	restrictPublic bool
	existingKeys   []APIKey
	created        []CreateAPIKeyRequest
}

func (s *webChatAPIKeyServiceStub) GetUserGroupVisibility(ctx context.Context, userID int64) (map[int64]struct{}, bool, error) {
	return s.visibility, s.restrictPublic, nil
}

func (s *webChatAPIKeyServiceStub) List(ctx context.Context, userID int64, params pagination.PaginationParams, filters APIKeyListFilters) ([]APIKey, *pagination.PaginationResult, error) {
	return s.existingKeys, &pagination.PaginationResult{Total: int64(len(s.existingKeys))}, nil
}

func (s *webChatAPIKeyServiceStub) Create(ctx context.Context, userID int64, req CreateAPIKeyRequest) (*APIKey, error) {
	s.created = append(s.created, req)
	return &APIKey{ID: int64(len(s.created)), Key: "sk-web-chat-new", Name: req.Name, GroupID: req.GroupID, Status: StatusActive}, nil
}

type webChatGroupListerStub struct {
	groups []Group
}

func (s *webChatGroupListerStub) ListActive(ctx context.Context) ([]Group, error) {
	return s.groups, nil
}

type webChatGatewayStub struct {
	byGroup map[int64][]string
}

func (s *webChatGatewayStub) GetAvailableModels(ctx context.Context, groupID *int64, platform string) []string {
	if groupID == nil {
		return nil
	}
	return s.byGroup[*groupID]
}

// newWebChatTestService 组装被测服务（settings 已注入指定值）
func newWebChatTestService(values map[string]string, keys *webChatAPIKeyServiceStub, gw *webChatGatewayStub, groups *webChatGroupListerStub) *WebChatService {
	return &WebChatService{
		settingService: NewSettingService(&webChatSettingRepoStub{values: values}, &config.Config{}),
		apiKeyService:  keys,
		gatewayService: gw,
		groupService:   groups,
		cfg:            &config.Config{Server: config.ServerConfig{Port: 8080}},
	}
}

// ─────────────────────────── 测试 ───────────────────────────

func TestWebChatInitializeDefaultSettings(t *testing.T) {
	repo := &webChatSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})

	require.NoError(t, svc.InitializeDefaultSettings(context.Background()))

	require.Equal(t, "true", repo.values[SettingKeyWebChatEnabled])
	require.Equal(t, "", repo.values[SettingKeyWebChatModels])
	require.Equal(t, "", repo.values[SettingKeyWebChatDefaultModel])
	require.Equal(t, "true", repo.values[SettingKeyWebChatEntranceChat])
	require.Equal(t, "true", repo.values[SettingKeyWebChatEntranceDraw])
	require.Equal(t, "true", repo.values[SettingKeyWebChatEntranceTTS])
	require.Equal(t, "true", repo.values[SettingKeyWebChatEntranceMusic])
}

func TestWebChatEntranceFlagsParseAndConfig(t *testing.T) {
	repo := &webChatSettingRepoStub{values: map[string]string{
		SettingKeyWebChatEnabled: "true",
		// 不写入 entrance 键 = 模拟存量站点：缺 key 视为显示
	}}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WebChatEntranceChat)
	require.True(t, settings.WebChatEntranceDraw)
	require.True(t, settings.WebChatEntranceTTS)
	require.True(t, settings.WebChatEntranceMusic)

	// 显式 "false" 隐藏；脏值按显示处理
	repo.values[SettingKeyWebChatEntranceDraw] = "false"
	repo.values[SettingKeyWebChatEntranceTTS] = "1"
	require.False(t, settingsOf(t, svc, repo).WebChatEntranceDraw)
	require.True(t, settingsOf(t, svc, repo).WebChatEntranceTTS)

	// round-trip：写回 false 后仍是 false
	settings.WebChatEntranceMusic = false
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, "false", repo.values[SettingKeyWebChatEntranceMusic])
	again, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.False(t, again.WebChatEntranceMusic)
}

func settingsOf(t *testing.T, svc *SettingService, repo *webChatSettingRepoStub) *SystemSettings {
	t.Helper()
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	return settings
}

func TestWebChatSettingsParseAndRoundTrip(t *testing.T) {
	modelsJSON := `[{"model":"glm-4.6","display_name":"GLM","vendor":"智谱","type":"chat","api_only":false},{"model":"gemini-2.5-flash-image","display_name":"","vendor":"","type":"image","api_only":false}]`
	repo := &webChatSettingRepoStub{values: map[string]string{
		SettingKeyWebChatEnabled:      "true",
		SettingKeyWebChatModels:       modelsJSON,
		SettingKeyWebChatDefaultModel: " glm-4.6 ",
	}}
	svc := NewSettingService(repo, &config.Config{})

	// parse
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WebChatEnabled)
	require.Equal(t, modelsJSON, settings.WebChatModels)
	require.Equal(t, "glm-4.6", settings.WebChatDefaultModel)

	// update（整体写回 3 个 key；bool 用 strconv.FormatBool、JSON 串原样）
	settings.WebChatEnabled = false
	settings.WebChatDefaultModel = ""
	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, "false", repo.values[SettingKeyWebChatEnabled])
	require.Equal(t, modelsJSON, repo.values[SettingKeyWebChatModels])
	require.Equal(t, "", repo.values[SettingKeyWebChatDefaultModel])

	// round-trip：写回的值再次 parse 语义一致
	again, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.False(t, again.WebChatEnabled)
	require.Equal(t, modelsJSON, again.WebChatModels)
	require.Equal(t, "", again.WebChatDefaultModel)
}

func TestWebChatEnabledParse(t *testing.T) {
	cases := []struct {
		raw   string
		want  bool
	}{
		{"true", true},
		{"", true},    // 键缺失/空值（存量站点未写入设置行）默认开启
		{"false", false},
		{"True", false}, // 脏值 fail-closed
		{"1", false},
		{"yes", false},
	}
	for _, tc := range cases {
		repo := &webChatSettingRepoStub{values: map[string]string{
			SettingKeyWebChatEnabled: tc.raw,
		}}
		svc := NewSettingService(repo, &config.Config{})
		settings, err := svc.GetAllSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, tc.want, settings.WebChatEnabled, "value %q", tc.raw)
	}
	// 完全没有该设置行时同样默认开启
	repo := &webChatSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{})
	settings, err := svc.GetAllSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.WebChatEnabled)
}

func TestWebChatModelsParseIgnoresBadEntries(t *testing.T) {
	raw := `[
		{"model":"glm-4.6","display_name":"智谱旗舰","vendor":"智谱","description":"d1","type":"chat","api_only":false},
		"not-an-object",
		{"model":"","type":"chat"},
		{"model":"bad-type","type":"video"},
		{"model":"claude-sonnet-5","type":"CHAT"},
		{"model":"gpt-5-image","type":"image","api_only":true},
		{"display_name":"no model","type":"chat"}
	]`
	models := parseWebChatModels(raw)
	require.Len(t, models, 3)
	require.Equal(t, "glm-4.6", models[0].Model)
	require.Equal(t, "智谱旗舰", models[0].DisplayName)
	require.Equal(t, WebChatModelTypeChat, models[0].Type)
	// type 大小写归一
	require.Equal(t, "claude-sonnet-5", models[1].Model)
	require.Equal(t, WebChatModelTypeChat, models[1].Type)
	// api_only 原样保留（由白名单判定排除）
	require.Equal(t, "gpt-5-image", models[2].Model)
	require.True(t, models[2].APIOnly)

	// 整体非法 → 空（调用方按回退/无模型语义处理）
	require.Nil(t, parseWebChatModels("not-json"))
	require.Nil(t, parseWebChatModels(""))
	require.Nil(t, parseWebChatModels("   "))
}

func TestWebChatVendorForModel(t *testing.T) {
	cases := map[string]string{
		"glm-4.6":         "智谱",
		"claude-sonnet-5": "Anthropic",
		"gpt-4o":          "OpenAI",
		"o1-mini":         "OpenAI",
		"o3":              "OpenAI",
		"gemini-2.5-pro":  "Google",
		"grok-4":          "xAI",
		"deepseek-chat":   "DeepSeek",
		"qwen-max":        "通义千问",
		"doubao-pro-32k":  "字节跳动",
		"kimi-k2":         "月之暗面",
		"moonshot-v1-8k":  "月之暗面",
		"llama-3":         "",
		"":                "",
	}
	for model, want := range cases {
		require.Equal(t, want, WebChatVendorForModel(model), "model %q", model)
	}
}

func TestWebChatModelAllowedWhitelist(t *testing.T) {
	models := []WebChatModel{
		{Model: "glm-4.6", Type: WebChatModelTypeChat},
		{Model: "claude-sonnet-5", Type: WebChatModelTypeChat, APIOnly: true},
		{Model: "gemini-image", Type: WebChatModelTypeImage},
		{Model: "gpt-image-only-api", Type: WebChatModelTypeImage, APIOnly: true},
	}

	require.True(t, WebChatModelAllowed(models, WebChatModelTypeChat, "glm-4.6"))
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeChat, "claude-sonnet-5")) // api_only
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeChat, "gemini-image"))    // 类型不匹配
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeChat, "unknown"))
	require.True(t, WebChatModelAllowed(models, WebChatModelTypeImage, "gemini-image"))
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeImage, "gpt-image-only-api"))
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeImage, "glm-4.6"))
	// 空候选集（如 enabled=false）→ 全部拒绝
	require.False(t, WebChatModelAllowed(nil, WebChatModelTypeChat, "glm-4.6"))
}

func TestWebChatGetConfigDisabledReturnsEmptyModels(t *testing.T) {
	svc := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled:      "false",
		SettingKeyWebChatModels:       `[{"model":"glm-4.6","type":"chat"}]`,
		SettingKeyWebChatDefaultModel: "glm-4.6",
	}, nil, nil, nil)

	cfg, err := svc.GetConfig(context.Background(), 42)
	require.NoError(t, err)
	require.False(t, cfg.Enabled)
	require.Equal(t, "glm-4.6", cfg.DefaultModel) // default_model 原样回传
	require.NotNil(t, cfg.Models)
	require.Empty(t, cfg.Models) // enabled=false → models 空数组
}

func TestWebChatCheckModelAllowed(t *testing.T) {
	svc := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled: "true",
		SettingKeyWebChatModels:  `[{"model":"glm-4.6","type":"chat"},{"model":"gemini-image","type":"image","api_only":true}]`,
	}, nil, nil, nil)

	require.NoError(t, svc.checkModelAllowed(context.Background(), 42, "glm-4.6", WebChatModelTypeChat))

	err := svc.checkModelAllowed(context.Background(), 42, "claude-sonnet-5", WebChatModelTypeChat)
	require.Error(t, err)
	require.Equal(t, 400, errors.Code(err))

	err = svc.checkModelAllowed(context.Background(), 42, "gemini-image", WebChatModelTypeImage)
	require.Error(t, err)
	require.Equal(t, 400, errors.Code(err))

	// enabled=false → 一律 403
	disabled := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled: "false",
		SettingKeyWebChatModels:  `[{"model":"glm-4.6","type":"chat"}]`,
	}, nil, nil, nil)
	err = disabled.checkModelAllowed(context.Background(), 42, "glm-4.6", WebChatModelTypeChat)
	require.Error(t, err)
	require.Equal(t, 403, errors.Code(err))
}

func TestWebChatConfiguredModelsOverrideFallback(t *testing.T) {
	keys := &webChatAPIKeyServiceStub{visibility: map[int64]struct{}{1: {}, 2: {}}}
	gw := &webChatGatewayStub{byGroup: map[int64][]string{1: {"should-not-appear"}}}
	svc := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled: "true",
		SettingKeyWebChatModels:  `[{"model":"glm-4.6","display_name":"GLM","type":"chat","api_only":false}]`,
	}, keys, gw, &webChatGroupListerStub{})

	cfg, err := svc.GetConfig(context.Background(), 42)
	require.NoError(t, err)
	require.Len(t, cfg.Models, 1)
	require.Equal(t, "GLM", cfg.Models[0].DisplayName)
	require.Equal(t, "智谱", cfg.Models[0].Vendor)
	require.False(t, cfg.Models[0].APIOnly)
}

func TestWebChatFallbackModelsUnionShape(t *testing.T) {
	keys := &webChatAPIKeyServiceStub{
		visibility:     map[int64]struct{}{1: {}, 2: {}},
		restrictPublic: true, // 开启公开分组限制：公开分组 3 不在授权集合内 → 不可见
	}
	gw := &webChatGatewayStub{byGroup: map[int64][]string{
		1: {"glm-4.6", "gpt-4o", "glm-4.6"},
		2: {"claude-sonnet-5"},
		3: {"kimi-k2"}, // 公开分组但用户受限且未授权（不应并入）
	}}
	groups := &webChatGroupListerStub{groups: []Group{
		{ID: 1, IsExclusive: false},
		{ID: 2, IsExclusive: true},
		{ID: 3, IsExclusive: false},
	}}
	svc := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled: "true",
		SettingKeyWebChatModels:  "", // 空串 = 回退模式
	}, keys, gw, groups)

	cfg, err := svc.GetConfig(context.Background(), 42)
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	require.Len(t, cfg.Models, 3)
	// 并集去重 + 排序：可见分组 = 授权 {2} ∪ 公开 {1}
	require.Equal(t, "claude-sonnet-5", cfg.Models[0].Model)
	require.Equal(t, "glm-4.6", cfg.Models[1].Model)
	require.Equal(t, "gpt-4o", cfg.Models[2].Model)
	// 回退模式每条形状：display_name=model、vendor=前缀推断、type=chat、api_only=false
	for _, m := range cfg.Models {
		require.Equal(t, m.Model, m.DisplayName)
		require.Equal(t, WebChatVendorForModel(m.Model), m.Vendor)
		require.Equal(t, WebChatModelTypeChat, m.Type)
		require.False(t, m.APIOnly)
		require.Empty(t, m.Description)
	}
}

func TestWebChatVisibleGroupsWithPublicRestriction(t *testing.T) {
	groups := []Group{
		{ID: 1, IsExclusive: false},
		{ID: 2, IsExclusive: true},
		{ID: 3, IsExclusive: false},
	}

	// 不限公开分组：授权 {2} + 公开 {1,3}
	visible := webChatVisibleGroups(groups, map[int64]struct{}{2: {}}, false)
	require.Len(t, visible, 3)

	// 开启公开分组限制：只有授权 {2}
	visible = webChatVisibleGroups(groups, map[int64]struct{}{2: {}}, true)
	require.Len(t, visible, 1)
	require.Equal(t, int64(2), visible[0].ID)
}

func TestWebChatPickGroupIDPrefersDefaultModel(t *testing.T) {
	keys := &webChatAPIKeyServiceStub{visibility: map[int64]struct{}{1: {}, 2: {}}}
	gw := &webChatGatewayStub{byGroup: map[int64][]string{
		1: {"claude-sonnet-5"},
		2: {"claude-sonnet-5", "glm-4.6"},
	}}
	groups := &webChatGroupListerStub{groups: []Group{
		{ID: 1, IsExclusive: false},
		{ID: 2, IsExclusive: false},
	}}
	svc := newWebChatTestService(map[string]string{SettingKeyWebChatEnabled: "true"}, keys, gw, groups)

	// 默认模型挂在分组 2 → 优先分组 2
	gid, err := svc.pickGroupID(context.Background(), 42, "glm-4.6")
	require.NoError(t, err)
	require.NotNil(t, gid)
	require.Equal(t, int64(2), *gid)

	// 无默认模型 → 第一个可见分组
	gid, err = svc.pickGroupID(context.Background(), 42, "")
	require.NoError(t, err)
	require.Equal(t, int64(1), *gid)
}

func TestWebChatGetOrCreateAPIKeyReusesExisting(t *testing.T) {
	keys := &webChatAPIKeyServiceStub{
		visibility: map[int64]struct{}{1: {}},
		existingKeys: []APIKey{
			{Key: "sk-other", Name: "别的 Key", Status: StatusActive},
			{Key: "sk-web-chat", Name: WebChatAPIKeyName, Status: StatusActive},
		},
	}
	gw := &webChatGatewayStub{byGroup: map[int64][]string{1: {"glm-4.6"}}}
	groups := &webChatGroupListerStub{groups: []Group{{ID: 1, IsExclusive: false}}}
	svc := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled:      "true",
		SettingKeyWebChatDefaultModel: "glm-4.6",
	}, keys, gw, groups)

	key, err := svc.getOrCreateAPIKey(context.Background(), 42, "glm-4.6")
	require.NoError(t, err)
	require.Equal(t, "sk-web-chat", key)
	require.Empty(t, keys.created) // 已有"网页聊天"Key → 不重复创建
}

func TestWebChatGetOrCreateAPIKeyCreatesWithGroup(t *testing.T) {
	// 决策记录验证：免分组 Key 在本网关不可用（见 web_chat_service.go 头注释），
	// 新建 Key 必须绑定具体分组。
	keys := &webChatAPIKeyServiceStub{visibility: map[int64]struct{}{1: {}}}
	gw := &webChatGatewayStub{byGroup: map[int64][]string{1: {"glm-4.6"}}}
	groups := &webChatGroupListerStub{groups: []Group{{ID: 1, IsExclusive: false}}}
	svc := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled:      "true",
		SettingKeyWebChatDefaultModel: "glm-4.6",
	}, keys, gw, groups)

	key, err := svc.getOrCreateAPIKey(context.Background(), 42, "glm-4.6")
	require.NoError(t, err)
	require.Equal(t, "sk-web-chat-new", key)
	require.Len(t, keys.created, 1)
	require.NotNil(t, keys.created[0].GroupID)
	require.Equal(t, int64(1), *keys.created[0].GroupID)
	require.Equal(t, WebChatAPIKeyName, keys.created[0].Name)
}

func TestWebChatLoopbackBaseURL(t *testing.T) {
	svc := &WebChatService{cfg: &config.Config{Server: config.ServerConfig{Port: 9090, Host: "0.0.0.0"}}}
	require.Equal(t, "http://127.0.0.1:9090", svc.webChatLoopbackBaseURL())
	svc.cfg = nil
	require.Equal(t, "http://127.0.0.1:0", svc.webChatLoopbackBaseURL())
}

func TestWebChatAdminAvailableModels(t *testing.T) {
	gw := &webChatGatewayStub{byGroup: map[int64][]string{
		1: {"glm-4.6", "gpt-4o"},
		2: {"gpt-4o", "claude-sonnet-5"},
	}}
	svc := &WebChatService{
		settingService: NewSettingService(&webChatSettingRepoStub{values: map[string]string{}}, &config.Config{}),
		gatewayService: gw,
		groupService:   &webChatGroupListerStub{groups: []Group{{ID: 1}, {ID: 2}, {ID: 3}}},
	}

	models, err := svc.AdminAvailableModels(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"claude-sonnet-5", "glm-4.6", "gpt-4o"}, models)
}

// ─────────────────────────── tts / music 类型 ───────────────────────────

func TestWebChatModelsParseAcceptsTTSAndMusic(t *testing.T) {
	raw := `[
		{"model":"tts-1","type":"tts","api_only":false},
		{"model":"music-large","type":"music","api_only":true},
		{"model":"TTS-Pro","type":"  TTS  "},
		{"model":"music-free","type":"music"},
		{"model":"video-x","type":"video"}
	]`
	models := parseWebChatModels(raw)
	require.Len(t, models, 4)
	require.Equal(t, "tts-1", models[0].Model)
	require.Equal(t, WebChatModelTypeTTS, models[0].Type)
	require.False(t, models[0].APIOnly)
	require.Equal(t, "music-large", models[1].Model)
	require.Equal(t, WebChatModelTypeMusic, models[1].Type)
	require.True(t, models[1].APIOnly)
	// type 大小写/空白归一
	require.Equal(t, "TTS-Pro", models[2].Model)
	require.Equal(t, WebChatModelTypeTTS, models[2].Type)
	require.Equal(t, "music-free", models[3].Model)
	require.Equal(t, WebChatModelTypeMusic, models[3].Type)

	// 白名单判定对新类型泛化成立（type 匹配 + api_only 排除 + 跨类型拒绝）
	require.True(t, WebChatModelAllowed(models, WebChatModelTypeTTS, "tts-1"))
	require.True(t, WebChatModelAllowed(models, WebChatModelTypeTTS, "TTS-Pro"))
	require.True(t, WebChatModelAllowed(models, WebChatModelTypeMusic, "music-free"))
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeTTS, "music-large")) // api_only
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeMusic, "music-large"))
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeMusic, "tts-1"))
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeImage, "tts-1"))
	require.False(t, WebChatModelAllowed(models, WebChatModelTypeChat, "tts-1"))
}

func TestWebChatCheckModelAllowedTTSAndMusic(t *testing.T) {
	svc := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled: "true",
		SettingKeyWebChatModels:  `[{"model":"tts-1","type":"tts"},{"model":"music-x","type":"music"}]`,
	}, nil, nil, nil)

	require.NoError(t, svc.checkModelAllowed(context.Background(), 42, "tts-1", WebChatModelTypeTTS))
	require.NoError(t, svc.checkModelAllowed(context.Background(), 42, "music-x", WebChatModelTypeMusic))

	// speech 入口用 chat 模型 → 400
	err := svc.checkModelAllowed(context.Background(), 42, "music-x", WebChatModelTypeTTS)
	require.Error(t, err)
	require.Equal(t, 400, errors.Code(err))
}

// ─────────────────────────── 回环代理（httptest loopback） ───────────────────────────

// newLoopbackTestService 组装指向 httptest 回环目标的被测服务
func newLoopbackTestService(t *testing.T, srv *httptest.Server, modelsJSON, defaultModel string) *WebChatService {
	t.Helper()
	u, err := url.Parse(srv.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(u.Port())
	require.NoError(t, err)
	return &WebChatService{
		settingService: NewSettingService(&webChatSettingRepoStub{values: map[string]string{
			SettingKeyWebChatEnabled:      "true",
			SettingKeyWebChatModels:       modelsJSON,
			SettingKeyWebChatDefaultModel: defaultModel,
		}}, &config.Config{}),
		apiKeyService: &webChatAPIKeyServiceStub{
			visibility:   map[int64]struct{}{1: {}},
			existingKeys: []APIKey{{Key: "sk-web-chat", Name: WebChatAPIKeyName, Status: StatusActive}},
		},
		gatewayService: &webChatGatewayStub{byGroup: map[int64][]string{1: {"glm-4.6", "gpt-image-2", "tts-1"}}},
		groupService:   &webChatGroupListerStub{groups: []Group{{ID: 1, IsExclusive: false}}},
		cfg:            &config.Config{Server: config.ServerConfig{Port: port}},
		httpClient:     &http.Client{},
	}
}

func newLoopbackTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/web-chat/loopback", nil)
	return c, w
}

func TestWebChatProxyImagesEditsBuildsGatewayEditBody(t *testing.T) {
	var gotPath, gotAuth, gotContentType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"url":"https://example.com/out.png"}]}`))
	}))
	defer srv.Close()

	svc := newLoopbackTestService(t, srv, `[{"model":"gpt-image-2","type":"image"}]`, "")
	c, w := newLoopbackTestContext()

	err := svc.ProxyImagesEdits(c, 42, WebChatImageEditRequest{
		Model:  "gpt-image-2",
		Prompt: "a cat",
		Image:  []string{"data:image/png;base64,AAAA", "  "},
		N:      2,
	})
	require.NoError(t, err)

	// 回环目标与鉴权
	require.Equal(t, "/v1/images/edits", gotPath)
	require.Equal(t, "Bearer sk-web-chat", gotAuth)
	require.Equal(t, "application/json", gotContentType)
	// 网关 JSON 编辑形状：images 数组逐条 {"image_url": "<data URL>"}（空串条目丢弃）
	require.Equal(t, "gpt-image-2", gotBody["model"])
	require.Equal(t, "a cat", gotBody["prompt"])
	require.Equal(t, float64(2), gotBody["n"])
	images, ok := gotBody["images"].([]any)
	require.True(t, ok)
	require.Len(t, images, 1)
	first, ok := images[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "data:image/png;base64,AAAA", first["image_url"])
	// 响应原样透传
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")
	require.JSONEq(t, `{"data":[{"url":"https://example.com/out.png"}]}`, w.Body.String())
}

func TestWebChatProxyImagesEditsPassthroughUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"type":"invalid_request_error","message":"bad image"}}`))
	}))
	defer srv.Close()

	svc := newLoopbackTestService(t, srv, `[{"model":"gpt-image-2","type":"image"}]`, "")
	c, w := newLoopbackTestContext()

	err := svc.ProxyImagesEdits(c, 42, WebChatImageEditRequest{
		Model:  "gpt-image-2",
		Prompt: "a cat",
		Image:  []string{"data:image/png;base64,AAAA"},
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusBadRequest, w.Code)
	require.Contains(t, w.Body.String(), "bad image")
}

func TestWebChatProxyAudioSpeechPassthroughBinary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/audio/speech", r.URL.Path)
		require.Equal(t, "Bearer sk-web-chat", r.Header.Get("Authorization"))
		var body map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		require.Equal(t, "tts-1", body["model"])
		require.Equal(t, "hello", body["input"])
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte{0xff, 0xf3, 0x00, 0x01})
	}))
	defer srv.Close()

	svc := newLoopbackTestService(t, srv, `[{"model":"tts-1","type":"tts"}]`, "")
	c, w := newLoopbackTestContext()

	err := svc.ProxyAudioSpeech(c, 42, WebChatAudioSpeechRequest{Model: "tts-1", Input: "hello"})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "audio/mpeg", w.Header().Get("Content-Type"))
	require.Equal(t, []byte{0xff, 0xf3, 0x00, 0x01}, w.Body.Bytes())
}

func TestWebChatProxyAudioSpeechPassthroughUpstreamErrorStatus(t *testing.T) {
	// 错误状态同样不做 JSON 解析，状态码 + Content-Type + 原始字节透传
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited"}}`))
	}))
	defer srv.Close()

	svc := newLoopbackTestService(t, srv, `[{"model":"tts-1","type":"tts"}]`, "")
	c, w := newLoopbackTestContext()

	err := svc.ProxyAudioSpeech(c, 42, WebChatAudioSpeechRequest{Model: "tts-1", Input: "hello"})
	require.NoError(t, err)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Contains(t, w.Body.String(), "rate limited")
}

func TestWebChatProxyAudioSpeechRejectsNonTTSModel(t *testing.T) {
	svc := newWebChatTestService(map[string]string{
		SettingKeyWebChatEnabled: "true",
		SettingKeyWebChatModels:  `[{"model":"glm-4.6","type":"chat"},{"model":"music-x","type":"music"}]`,
	}, nil, nil, nil)
	c, _ := newLoopbackTestContext()

	// chat 模型不可用于 speech；music 模型也不可（speech 白名单只认 type=tts）
	err := svc.ProxyAudioSpeech(c, 42, WebChatAudioSpeechRequest{Model: "glm-4.6", Input: "hi"})
	require.Error(t, err)
	require.Equal(t, 400, errors.Code(err))

	err = svc.ProxyAudioSpeech(c, 42, WebChatAudioSpeechRequest{Model: "music-x", Input: "hi"})
	require.Error(t, err)
	require.Equal(t, 400, errors.Code(err))
}
