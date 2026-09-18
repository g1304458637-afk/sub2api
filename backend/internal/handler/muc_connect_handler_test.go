package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/Wei-Shaw/sub2api/internal/pkg/campus"
	"github.com/gin-gonic/gin"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// campusCodeKeyPrefixForTest 与 campus.MUC.RedisPrefix 一致（历史常量名沿用）
const campusCodeKeyPrefixForTest = "muc:code:"

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
	k.Name = req.Name
	s.live[k.ID] = k
	return &k, nil
}

func (s *stubKeyManager) Delete(ctx context.Context, id int64, userID int64) error {
	s.deleted = append(s.deleted, id)
	delete(s.live, id)
	return nil
}

func (s *stubKeyManager) SearchAPIKeys(ctx context.Context, userID int64, keyword string, limit int) ([]service.APIKey, error) {
	var out []service.APIKey
	for _, k := range s.live {
		if strings.Contains(k.Name, keyword) {
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

// miniredis 适配 mucCodeStore：单线程测试下 Set/GetDel 语义与真实 GETDEL 等价
type stubCodeStore struct {
	mr *miniredis.Miniredis
}

func (s *stubCodeStore) SetCode(ctx context.Context, key string, payload []byte, ttl time.Duration) error {
	return s.mr.Set(key, string(payload))
}

func (s *stubCodeStore) GetDelCode(ctx context.Context, key string) (string, error) {
	if !s.mr.Exists(key) {
		return "", errMucCodeNotFound
	}
	v, _ := s.mr.Get(key)
	_ = s.mr.Del(key)
	return v, nil
}

var errMucCodeNotFound = errors.New("muc code not found")

// ---- 测试脚手架 ----

func newMucTestEnv(t *testing.T) (*CampusConnectHandler, *miniredis.Miniredis, *stubKeyManager) {
	t.Helper()
	mr := miniredis.RunT(t)
	h := NewCampusConnectHandler(campus.MUC, nil, nil, nil)
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
		_ = mr.Set(campusCodeKeyPrefixForTest+sum, string(payload))
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
	_ = mr.Set(campusCodeKeyPrefixForTest+sum, string(payload))

	c, w := mucCtxWithBody(t, `{"code":"code-fail-aaaaaaaaaaaaaaaa"}`)
	h.Exchange(c)
	if w.Code == http.StatusOK {
		t.Fatal("expected error propagation when key creation fails")
	}
}

// sha256Hex 与 handler 内部逻辑一致（brand.RedisPrefix + hex(sha256(code))）
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
		_ = mr.Set(campusCodeKeyPrefixForTest+hex.EncodeToString(sum[:]), string(payload))
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

	// 旧 Key 已被轮换删除（异步协程，轮询等待）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(creator.deleted) == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if len(creator.deleted) == 0 {
		t.Fatalf("expected old device key to be rotated (deleted), got none")
	}
	if len(creator.live) != 1 {
		t.Fatalf("expected exactly 1 live key after rotation, got %d", len(creator.live))
	}
	for _, k := range creator.live {
		if !strings.Contains(k.Name, "MUC MacBookPro") {
			t.Fatalf("unexpected surviving key name: %s", k.Name)
		}
	}
}

// sha256HexWithPrefix 用指定品牌前缀算 Redis key（与 handler 内部逻辑一致）
func sha256HexWithPrefix(prefix, code string) string {
	sum := sha256.Sum256([]byte(code))
	return prefix + hex.EncodeToString(sum[:])
}

// HUBU：与 MUC 共用机制，但 Redis 前缀 hubu:code:、Key 名前缀 "HUBU "，且单次使用
func TestHubuExchange_BrandPrefixAndSingleUse(t *testing.T) {
	h, mr, creator := newMucTestEnv(t)
	hubu := NewCampusConnectHandler(campus.HUBU, nil, nil, nil)
	hubu.codes = h.codes
	hubu.keys = h.keys
	hubu.userLookup = h.userLookup

	code := "hubu-local-test-code-12345678"
	payload, _ := json.Marshal(mucCodePayload{UserID: 42})
	_ = mr.Set(sha256HexWithPrefix(campus.HUBU.RedisPrefix, code), string(payload))

	c, w := mucCtxWithBody(t, `{"code":"`+code+`","device_name":"TestMac"}`)
	hubu.Exchange(c)
	if w.Code != http.StatusOK {
		t.Fatalf("hubu exchange: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if creator.lastReq.Name != "HUBU TestMac" {
		t.Fatalf("expected key name prefixed with HUBU, got %q", creator.lastReq.Name)
	}

	// 单次使用：同一 code 第二次必须 404
	c2, w2 := mucCtxWithBody(t, `{"code":"`+code+`"}`)
	hubu.Exchange(c2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for reused code, got %d", w2.Code)
	}
}

// HUBU 授权码必须写在 hubu:code: 前缀下，与 MUC 互不读取
func TestHubuCodeIsolatedFromMucPrefix(t *testing.T) {
	h, mr, _ := newMucTestEnv(t)
	hubu := NewCampusConnectHandler(campus.HUBU, nil, nil, nil)
	hubu.codes = h.codes
	hubu.keys = h.keys
	hubu.userLookup = h.userLookup

	code := "cross-brand-code-000000000000"
	payload, _ := json.Marshal(mucCodePayload{UserID: 42})
	_ = mr.Set(sha256HexWithPrefix(campus.MUC.RedisPrefix, code), string(payload)) // 只写在 MUC 前缀下

	c, w := mucCtxWithBody(t, `{"code":"`+code+`"}`)
	hubu.Exchange(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("hubu must not read muc-prefixed codes, got %d", w.Code)
	}
}
