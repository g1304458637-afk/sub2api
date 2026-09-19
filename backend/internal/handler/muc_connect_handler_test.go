package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"

	"github.com/Wei-Shaw/sub2api/internal/pkg/muccode"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ---- 打桩 ----

type stubKeyManager struct {
	lastUserID int64
	lastReq    service.CreateAPIKeyRequest
	key        *service.APIKey
	err        error
	calls      int
	deleted    []int64
	live       map[int64]service.APIKey // id -> key（模拟库存）
	nextID     int64
}

func newStubKeyManager() *stubKeyManager {
	return &stubKeyManager{
		key:    &service.APIKey{Key: "sk-muc-test-key", Name: "MUC test"},
		live:   map[int64]service.APIKey{},
		nextID: 1000,
	}
}

func (s *stubKeyManager) Create(ctx context.Context, userID int64, req service.CreateAPIKeyRequest) (*service.APIKey, error) {
	s.calls++
	s.lastUserID = userID
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	s.nextID++
	k := *s.key
	k.ID = s.nextID
	// 与生产 service.CreateAPIKey 对齐：名称经 html.EscapeString 落库
	k.Name = html.EscapeString(req.Name)
	s.live[k.ID] = k
	return &k, nil
}

func (s *stubKeyManager) Delete(ctx context.Context, id int64, userID int64) error {
	s.deleted = append(s.deleted, id)
	delete(s.live, id)
	return nil
}

func (s *stubKeyManager) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	// 与生产 NameContainsFold 对齐：大小写不敏感的子串搜索
	kw := strings.ToLower(keyword)
	var out []service.APIKey
	for _, k := range s.live {
		if strings.Contains(strings.ToLower(k.Name), kw) {
			out = append(out, k)
		}
	}
	return out, nil
}

type stubUserLookup struct {
	user *service.User
}

func (s *stubUserLookup) GetByID(ctx context.Context, id int64) (*service.User, error) {
	return s.user, nil
}

// miniredis 适配 mucCodeStore：单线程测试下 Set/GetDel 语义与真实 GETDEL 等价。
// infraErr 非 nil 时模拟 Redis 基础设施故障（网络/超时），与"码不存在"区分。
type stubCodeStore struct {
	mr       *miniredis.Miniredis
	infraErr error
}

func (s *stubCodeStore) SetCode(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	return s.mr.Set(key, string(payload))
}

func (s *stubCodeStore) GetDelCode(ctx context.Context, key string) (string, error) {
	if s.infraErr != nil {
		return "", s.infraErr
	}
	if !s.mr.Exists(key) {
		return "", muccode.ErrCodeNotFound
	}
	v, _ := s.mr.Get(key)
	_ = s.mr.Del(key)
	return v, nil
}

func (s *stubCodeStore) setInfraErr(err error) {
	s.infraErr = err
}

var errMucCodeNotFound = muccode.ErrCodeNotFound

// ---- 测试脚手架 ----

func newMucTestEnv(t *testing.T) (*MucConnectHandler, *miniredis.Miniredis, *stubKeyManager) {
	t.Helper()
	mr := miniredis.RunT(t)
	h := NewMucConnectHandler(nil, nil, nil)
	h.codes = &stubCodeStore{mr: mr}
	creator := newStubKeyManager()
	creator.key = &service.APIKey{Key: "sk-muc-test-key", Name: "MUC test"}
	// 替换 keys/user 为可观察桩
	h.keys = creator
	h.userLookup = &stubUserLookup{user: &service.User{Email: "student@muc.edu.cn"}}
	return h, mr, creator
}

func mucCtxWithUser(t *testing.T, userID int64) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/muc/connect-code", nil)
	c.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: userID})
	return c, w
}

func mucCtxWithBody(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/muc/exchange", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// ---- 用例 ----

func TestMucConnectCode_RequiresAuth(t *testing.T) {
	h, _, _ := newMucTestEnv(t)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/muc/connect-code", nil)
	h.ConnectCode(c)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated, got %d", w.Code)
	}
}

func TestMucConnectCode_IssuesCode(t *testing.T) {
	h, _, _ := newMucTestEnv(t)
	c, w := mucCtxWithUser(t, 42)
	h.ConnectCode(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code int `json:"code"`
		Data struct {
			Code      string `json:"code"`
			ExpiresIn int    `json:"expires_in"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 0 || resp.Data.Code == "" || resp.Data.ExpiresIn != 60 {
		t.Fatalf("unexpected payload: %s", w.Body.String())
	}
	if len(resp.Data.Code) < 16 {
		t.Fatalf("code too short: %s", resp.Data.Code)
	}
}

func TestMucExchange_HappyPath_SingleUse(t *testing.T) {
	h, mr, creator := newMucTestEnv(t)

	// 直接向 miniredis 写入一个有效 code 的哈希键（模拟 ConnectCode 已签发）
	issue := func(code string, userID int64) {
		sum := sha256Hex(code)
		payload, _ := json.Marshal(mucCodePayload{UserID: userID})
		_ = mr.Set(mucCodeKeyPrefix+sum, string(payload))
	}
	issue("valid-code-aaaaaaaaaaaaaaaaaa", 42)

	c, w := mucCtxWithBody(t, `{"code":"valid-code-aaaaaaaaaaaaaaaaaa","device_name":"MacBook-Pro"}`)
	h.Exchange(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			APIKey  string `json:"api_key"`
			KeyName string `json:"key_name"`
			User    string `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Data.APIKey != "sk-muc-test-key" {
		t.Fatalf("expected per-device key in response")
	}
	if creator.calls != 1 || creator.lastUserID != 42 {
		t.Fatalf("expected key created for user 42, calls=%d", creator.calls)
	}
	if !strings.Contains(creator.lastReq.Name, "MUC") {
		t.Fatalf("expected per-device key name, got %q", creator.lastReq.Name)
	}

	// 验收 D：第二次 exchange 必须失败（单次使用）
	c2, w2 := mucCtxWithBody(t, `{"code":"valid-code-aaaaaaaaaaaaaaaaaa"}`)
	h.Exchange(c2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 on reuse, got %d", w2.Code)
	}
}

func TestMucExchange_UnknownOrExpiredCode(t *testing.T) {
	h, _, _ := newMucTestEnv(t)
	c, w := mucCtxWithBody(t, `{"code":"never-issued-aaaaaaaaaaaaaa"}`)
	h.Exchange(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestMucExchange_RejectsBadRequests(t *testing.T) {
	h, _, _ := newMucTestEnv(t)
	c, w := mucCtxWithBody(t, `{"code":""}`)
	h.Exchange(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty code, got %d", w.Code)
	}
	long := strings.Repeat("a", 200)
	c2, w2 := mucCtxWithBody(t, `{"code":"`+long+`"}`)
	h.Exchange(c2)
	if w2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for overlong code, got %d", w2.Code)
	}
}

func TestMucExchange_KeyCreateFailure_Propagates(t *testing.T) {
	h, mr, creator := newMucTestEnv(t)
	creator.err = context.DeadlineExceeded
	sum := sha256Hex("code-fail-aaaaaaaaaaaaaaaa")
	payload, _ := json.Marshal(mucCodePayload{UserID: 7})
	_ = mr.Set(mucCodeKeyPrefix+sum, string(payload))

	c, w := mucCtxWithBody(t, `{"code":"code-fail-aaaaaaaaaaaaaaaa"}`)
	h.Exchange(c)
	if w.Code == http.StatusOK {
		t.Fatal("expected error propagation when key creation fails")
	}
}

// sha256Hex 与 handler 内部逻辑一致（mucCodeKeyPrefix + hex(sha256(code))）
func sha256Hex(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

// MUC Harness: 同设备重连时旧 Key 应被轮换删除（防积累）
func TestMucExchange_RotatesDeviceKey(t *testing.T) {
	h, mr, creator := newMucTestEnv(t)

	issue := func(code string, userID int64) {
		sum := sha256.Sum256([]byte(code))
		payload, _ := json.Marshal(mucCodePayload{UserID: userID})
		_ = mr.Set(mucCodeKeyPrefix+hex.EncodeToString(sum[:]), string(payload))
	}

	// 第一次连接
	issue("code-first-aaaaaaaaaaaaaaaa", 42)
	c1, w1 := mucCtxWithBody(t, `{"code":"code-first-aaaaaaaaaaaaaaaa","device_name":"MacBookPro"}`)
	h.Exchange(c1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first connect: expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	// 第二次连接（同设备重连）
	issue("code-second-bbbbbbbbbbbbbbbb", 42)
	c2, w2 := mucCtxWithBody(t, `{"code":"code-second-bbbbbbbbbbbbbbbb","device_name":"MacBookPro"}`)
	h.Exchange(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second connect: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// 旧 Key 已被同步轮换删除
	if len(creator.deleted) == 0 {
		t.Fatalf("expected old device key to be rotated (deleted), got none")
	}
	if len(creator.live) != 1 {
		t.Fatalf("expected exactly 1 live key after rotation, got %d", len(creator.live))
	}
	for _, k := range creator.live {
		if k.Name != html.EscapeString("MUC MacBookPro") {
			t.Fatalf("unexpected surviving key name: %s", k.Name)
		}
	}
}

// 回归：轮换必须按名称精确匹配，子串匹配会误删其他设备/用户手建的同前缀 Key。
func TestMucExchange_RotationExactMatch_NoCollateralDelete(t *testing.T) {
	h, mr, creator := newMucTestEnv(t)

	issue := func(code string, userID int64) {
		sum := sha256.Sum256([]byte(code))
		payload, _ := json.Marshal(mucCodePayload{UserID: userID})
		_ = mr.Set(mucCodeKeyPrefix+hex.EncodeToString(sum[:]), string(payload))
	}

	// 预置：用户已有若干 Key，只有 "MUC Mac" 是本设备（重连场景）的旧 Key
	seed := map[int64]string{
		101: "MUC Mac",          // 应被轮换删除
		102: "MUC MacBookPro",   // 其他设备，不能误删
		103: "MUC Mac Studio",   // 其他设备，不能误删
		104: "muc mac 备份钥匙",  // 用户手建，不能误删
		105: "手工钥匙",          // 无关 Key
	}
	for id, name := range seed {
		creator.live[id] = service.APIKey{ID: id, Key: "sk-" + name, Name: html.EscapeString(name)}
	}

	issue("code-exact-aaaaaaaaaaaaaaaa", 42)
	c, w := mucCtxWithBody(t, `{"code":"code-exact-aaaaaaaaaaaaaaaa","device_name":"Mac"}`)
	h.Exchange(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if len(creator.deleted) != 1 || creator.deleted[0] != 101 {
		t.Fatalf("expected exactly old key 101 deleted, got %v", creator.deleted)
	}
	for _, id := range []int64{102, 103, 104, 105} {
		if _, ok := creator.live[id]; !ok {
			t.Fatalf("collateral delete: key %d (%q) must survive", id, seed[id])
		}
	}
}

// 回归：设备名含 HTML 特殊字符时，轮换搜索必须与落库的转义名称对齐。
func TestMucExchange_RotationEscapedName(t *testing.T) {
	h, mr, creator := newMucTestEnv(t)

	issue := func(code string, userID int64) {
		sum := sha256.Sum256([]byte(code))
		payload, _ := json.Marshal(mucCodePayload{UserID: userID})
		_ = mr.Set(mucCodeKeyPrefix+hex.EncodeToString(sum[:]), string(payload))
	}

	issue("code-esc1-aaaaaaaaaaaaaaaa", 42)
	c1, w1 := mucCtxWithBody(t, `{"code":"code-esc1-aaaaaaaaaaaaaaaa","device_name":"Tom & Jerry's <PC>"}`)
	h.Exchange(c1)
	if w1.Code != http.StatusOK {
		t.Fatalf("first connect: expected 200, got %d: %s", w1.Code, w1.Body.String())
	}

	issue("code-esc2-bbbbbbbbbbbbbbbb", 42)
	c2, w2 := mucCtxWithBody(t, `{"code":"code-esc2-bbbbbbbbbbbbbbbb","device_name":"Tom & Jerry's <PC>"}`)
	h.Exchange(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("reconnect: expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	if len(creator.live) != 1 {
		t.Fatalf("expected exactly 1 live key after escaped-name rotation, got %d", len(creator.live))
	}
}

// 回归：超长中文设备名按 rune 截断，不得产生非法 UTF-8，兑换仍应成功。
func TestMucExchange_LongChineseDeviceName(t *testing.T) {
	h, mr, creator := newMucTestEnv(t)

	issue := func(code string, userID int64) {
		sum := sha256.Sum256([]byte(code))
		payload, _ := json.Marshal(mucCodePayload{UserID: userID})
		_ = mr.Set(mucCodeKeyPrefix+hex.EncodeToString(sum[:]), string(payload))
	}

	longName := strings.Repeat("民大校园超级计算终端设备", 20) // 200 runes，全中文
	issue("code-cjk1-aaaaaaaaaaaaaaaa", 42)
	c, w := mucCtxWithBody(t, `{"code":"code-cjk1-aaaaaaaaaaaaaaaa","device_name":"`+longName+`"}`)
	h.Exchange(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for long CJK device name, got %d: %s", w.Code, w.Body.String())
	}
	name := creator.lastReq.Name
	if !utf8.ValidString(name) {
		t.Fatalf("device name must remain valid UTF-8 after truncation, got %q", name)
	}
	if got := len([]rune(strings.TrimPrefix(name, "MUC "))); got > mucMaxDeviceRunes {
		t.Fatalf("truncated device name exceeds rune cap: %d", got)
	}
}

// 回归：Redis 基础设施故障必须表现为服务端错误，不得伪装成 code_not_found(404)。
func TestMucExchange_RedisInfraErrorIsServerError(t *testing.T) {
	h, _, _ := newMucTestEnv(t)
	h.codes.(*stubCodeStore).setInfraErr(errors.New("connection refused"))

	c, w := mucCtxWithBody(t, `{"code":"whatever-aaaaaaaaaaaaaaaa"}`)
	h.Exchange(c)
	if w.Code == http.StatusNotFound {
		t.Fatal("redis infra error must not be masked as 404 code_not_found")
	}
	if w.Code < 500 {
		t.Fatalf("expected 5xx for redis infra error, got %d", w.Code)
	}
}
