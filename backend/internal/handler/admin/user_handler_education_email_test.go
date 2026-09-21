package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAdminUserList_ParsesEducationEmailVerified(t *testing.T) {
	gin.SetMode(gin.TestMode)
	verified := true
	unverified := false
	cases := []struct {
		name  string
		query string
		want  *bool
	}{
		{"missing", "", nil},
		{"empty ignored", "?education_email_verified=", nil},
		{"true", "?education_email_verified=true", &verified},
		{"false", "?education_email_verified=false", &unverified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &listUsersFilterStub{AdminService: newStubAdminService()}
			r := gin.New()
			h := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
			r.GET("/admin/users", h.List)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/admin/users"+tc.query, nil)
			r.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			if tc.want == nil {
				require.Nil(t, stub.captured.EducationEmailVerified)
			} else {
				require.NotNil(t, stub.captured.EducationEmailVerified)
				require.Equal(t, *tc.want, *stub.captured.EducationEmailVerified)
			}
		})
	}

	t.Run("invalid value rejected with 400", func(t *testing.T) {
		stub := &listUsersFilterStub{AdminService: newStubAdminService()}
		r := gin.New()
		h := NewUserHandler(stub, nil, nil, nil, nil, nil, nil)
		r.GET("/admin/users", h.List)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/admin/users?education_email_verified=abc", nil)
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Nil(t, stub.captured.EducationEmailVerified)
	})
}

// educationEmailUserRepoStub 只覆盖 GetProfileIdentitySummaries 用到的方法，
// 其余 UserRepository 方法经由内嵌接口（未覆盖的调用触发 nil panic，即意外调用）。
type educationEmailUserRepoStub struct {
	service.UserRepository
	users      map[int64]*service.User
	identities []service.UserAuthIdentityRecord
}

func (s *educationEmailUserRepoStub) GetByID(_ context.Context, id int64) (*service.User, error) {
	if user, ok := s.users[id]; ok {
		return user, nil
	}
	return nil, service.ErrUserNotFound
}

func (s *educationEmailUserRepoStub) ListUserAuthIdentities(_ context.Context, _ int64) ([]service.UserAuthIdentityRecord, error) {
	out := make([]service.UserAuthIdentityRecord, len(s.identities))
	copy(out, s.identities)
	return out, nil
}

func setupEducationEmailRevokeRouter(adminSvc service.AdminService, userSvc *service.UserService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewUserHandler(adminSvc, nil, nil, nil, nil, userSvc, nil)
	r.DELETE("/admin/users/:id/education-email", h.RevokeEducationEmail)
	return r
}

func TestAdminUserRevokeEducationEmail(t *testing.T) {
	now := time.Now().UTC()
	verifiedIdentity := service.UserAuthIdentityRecord{
		ProviderType:    "education_email",
		ProviderKey:     "muc.edu.cn",
		ProviderSubject: "student@muc.edu.cn",
		VerifiedAt:      &now,
	}

	t.Run("success returns revoked count and post-revoke summary", func(t *testing.T) {
		adminSvc := newStubAdminService()
		adminSvc.revokedEducationEmailCount = 2
		// 撤销后身份已删除：summary 列表为空，bound 应为 false。
		userSvc := service.NewUserService(&educationEmailUserRepoStub{
			users: map[int64]*service.User{
				5: {ID: 5, Email: "user@example.com"},
			},
		}, nil, nil, nil)
		router := setupEducationEmailRevokeRouter(adminSvc, userSvc)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodDelete, "/admin/users/5/education-email", nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var body struct {
			Code int `json:"code"`
			Data struct {
				UserID         int64 `json:"user_id"`
				RevokedCount   int64 `json:"revoked_count"`
				EducationEmail struct {
					Bound    bool   `json:"bound"`
					Provider string `json:"provider"`
				} `json:"education_email"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, 0, body.Code)
		require.Equal(t, int64(5), body.Data.UserID)
		require.Equal(t, int64(2), body.Data.RevokedCount)
		require.False(t, body.Data.EducationEmail.Bound)
		require.Equal(t, "education_email", body.Data.EducationEmail.Provider)
		require.Equal(t, []int64{5}, adminSvc.revokedEducationEmailFor)
	})

	t.Run("summary reflects verified identity reported by identity service", func(t *testing.T) {
		adminSvc := newStubAdminService()
		userSvc := service.NewUserService(&educationEmailUserRepoStub{
			users: map[int64]*service.User{
				5: {ID: 5, Email: "user@example.com"},
			},
			identities: []service.UserAuthIdentityRecord{verifiedIdentity},
		}, nil, nil, nil)
		router := setupEducationEmailRevokeRouter(adminSvc, userSvc)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodDelete, "/admin/users/5/education-email", nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var body struct {
			Code int `json:"code"`
			Data struct {
				EducationEmail struct {
					Bound       bool   `json:"bound"`
					BoundCount  int    `json:"bound_count"`
					DisplayName string `json:"display_name"`
				} `json:"education_email"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		require.Equal(t, 0, body.Code)
		require.True(t, body.Data.EducationEmail.Bound)
		require.Equal(t, 1, body.Data.EducationEmail.BoundCount)
		require.Equal(t, "student@muc.edu.cn", body.Data.EducationEmail.DisplayName)
	})

	t.Run("invalid user id returns 400", func(t *testing.T) {
		adminSvc := newStubAdminService()
		userSvc := service.NewUserService(&educationEmailUserRepoStub{}, nil, nil, nil)
		router := setupEducationEmailRevokeRouter(adminSvc, userSvc)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodDelete, "/admin/users/abc/education-email", nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		require.Empty(t, adminSvc.revokedEducationEmailFor)
	})

	t.Run("unknown user returns 404", func(t *testing.T) {
		adminSvc := newStubAdminService()
		adminSvc.revokeEducationEmailErr = service.ErrUserNotFound
		userSvc := service.NewUserService(&educationEmailUserRepoStub{}, nil, nil, nil)
		router := setupEducationEmailRevokeRouter(adminSvc, userSvc)

		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodDelete, "/admin/users/999/education-email", nil)
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusNotFound, w.Code)
	})
}
