package campus

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDeploymentIdentity(t *testing.T) {
	for _, id := range []string{"muc", "hubu"} {
		t.Run(id, func(t *testing.T) {
			t.Setenv("BRAND", id)
			brand := Current()
			require.Equal(t, id, brand.ID)
			require.Equal(t, id, brand.ProtocolScheme)
			require.Equal(t, id+":desktop", brand.Audience())
			require.Equal(t, id+":code:", brand.RedisPrefix)
			require.Contains(t, brand.ManifestPath, map[string]string{"muc": "mucode", "hubu": "hubu-ai"}[id])
		})
	}
	t.Setenv("BRAND", "unknown")
	require.Panics(t, func() { Current() })
}

func TestExplicitDeploymentURLsAndEmailPolicy(t *testing.T) {
	t.Setenv("BRAND", "hubu")
	t.Setenv("CAMPUS_LOCAL_BUILD", "1")
	t.Setenv("HUBU_GATEWAY_URL", "http://127.0.0.1:18102")
	t.Setenv("HUBU_EDUCATION_EMAIL_DOMAIN", "test.hubu.example")
	require.Equal(t, "http://127.0.0.1:18102", Current().GatewayURL)
	require.Equal(t, "test.hubu.example", Current().EducationDomain)
	t.Setenv("HUBU_GATEWAY_URL", "https://user:password@example.org")
	require.Panics(t, func() { Current() })
}

func TestProductionGatewayRequiresHTTPS(t *testing.T) {
	t.Setenv("BRAND", "muc")
	t.Setenv("MUC_GATEWAY_URL", "")
	t.Setenv("CAMPUS_LOCAL_BUILD", "")
	require.Equal(t, "https://admin.wuxuexi.top", Current().GatewayURL)
	for _, local := range []string{"", "1"} {
		t.Setenv("CAMPUS_LOCAL_BUILD", local)
		t.Setenv("MUC_GATEWAY_URL", "http://admin.wuxuexi.top")
		require.Panics(t, func() { Current() })
	}
}
