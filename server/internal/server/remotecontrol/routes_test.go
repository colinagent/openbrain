package remotecontrol

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestManagementOriginAllowsDesktopAndRejectsWebOrigins(t *testing.T) {
	for _, origin := range []string{"", "null", "http://localhost:5173", "http://127.0.0.1:19530", "https://[::1]:19530"} {
		if !allowedManagementOrigin(origin) {
			t.Errorf("desktop origin %q was rejected", origin)
		}
	}
	for _, origin := range []string{"https://example.com", "https://localhost.example.com", "file:///tmp/attack.html", "javascript:alert(1)"} {
		if allowedManagementOrigin(origin) {
			t.Errorf("web origin %q was allowed", origin)
		}
	}
}

func TestRequireLocalManagementRequestRejectsNonLoopback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name       string
		remoteAddr string
		origin     string
		allowed    bool
	}{
		{name: "desktop loopback", remoteAddr: "127.0.0.1:4567", origin: "null", allowed: true},
		{name: "ipv6 loopback", remoteAddr: "[::1]:4567", origin: "http://localhost:5173", allowed: true},
		{name: "remote address", remoteAddr: "203.0.113.7:4567", allowed: false},
		{name: "web origin", remoteAddr: "127.0.0.1:4567", origin: "https://example.com", allowed: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			writer := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(writer)
			ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/remote-control/status", nil)
			ctx.Request.RemoteAddr = test.remoteAddr
			ctx.Request.Header.Set("Origin", test.origin)
			if got := requireLocalManagementRequest(ctx); got != test.allowed {
				t.Fatalf("allowed = %v, want %v", got, test.allowed)
			}
			if !test.allowed && writer.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", writer.Code, http.StatusForbidden)
			}
		})
	}
}
