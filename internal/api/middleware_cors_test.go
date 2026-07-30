package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsNativeDesktopOrigin(t *testing.T) {
	for _, origin := range []string{"http://wails.localhost", "https://wails.localhost", "wails://wails.localhost"} {
		t.Run(origin, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1:9090/api/v1/status", nil)
			request.Header.Set("Origin", origin)
			recorder := httptest.NewRecorder()
			CORS(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("preflight must not reach the API handler")
			})).ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
			}
			if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != origin {
				t.Fatalf("allow origin = %q, want %q", got, origin)
			}
		})
	}
}
