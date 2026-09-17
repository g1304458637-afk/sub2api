package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

// ---- 打桩 ----

type stubKeyCreator struct {
	lastUserID int64
	lastReq    service.CreateAPIKeyRequest
	key        *service.APIKey
	err        error
	calls      int
}

func (s *stubKeyCreator) Create(ctx context.Context, userID int64, req service.CreateAPIKeyRequest) (*service.APIKey, error) {
	s.calls++
	s.lastUserID = userID
	s.lastReq = req
	if s.err != nil {
		return nil, s.err
	}
	return s.key, nil
}

type stubUserLookup struct {
	user *service.User
}

func (s *stubUserLookup) GetByID(ctx context.Context, id int64) (*service.User, error) {
	return s.user, nil
}

// ---- 测试脚手架 ----

func newMucTestEnv(t *testing.T) (*MucConnectHandler, *miniredis.Miniredis, *stubKeyCreator) {
	t.Helper()
	mr := miniredis.RunT(t)
	h := NewMucConnectHandler(redis.NewClient(&redis.Options{Addr: mr.Addr()}), nil, nil)
	creator := &stubKeyCreator{key: &service.APIKey{Key: "sk-muc-test-key", Name: "MUC test"}}
	// 替换 creator/user 为可观察桩
	h.apiKeyCreator = creator
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
		Code int    `json:"code"`
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
		mr.Set(mucCodeKeyPrefix+sum, string(payload))
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
	mr.Set(mucCodeKeyPrefix+sum, string(payload))

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
