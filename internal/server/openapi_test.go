package server

import (
	"encoding/json"
	"mime"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOpenAPIExportContract(t *testing.T) {
	rec := httptest.NewRecorder()
	New(nil, "test-version", "test", "test").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if mediaType, _, err := mime.ParseMediaType(rec.Header().Get("Content-Type")); err != nil || mediaType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json; error = %v", rec.Header().Get("Content-Type"), err)
	}
	var doc struct {
		Info       struct{ Version string }
		Components struct {
			SecuritySchemes map[string]map[string]string
		}
		Security []map[string][]string
		Paths    map[string]map[string]struct {
			Description string
			Security    []map[string][]string
			Responses   map[string]struct{ Description string }
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("OpenAPI JSON parse failed: %v", err)
	}
	if doc.Info.Version != "test-version" {
		t.Errorf("info.version = %q, want test-version", doc.Info.Version)
	}

	t.Run("session cookie scheme", func(t *testing.T) {
		want := map[string]string{"type": "apiKey", "in": "cookie", "name": "orbit_session"}
		if got := doc.Components.SecuritySchemes["sessionCookie"]; !reflect.DeepEqual(got, want) {
			t.Errorf("sessionCookie = %v, want %v", got, want)
		}
	})
	t.Run("export overrides bearer authentication", func(t *testing.T) {
		// 인증 정책과 문서의 일치 검사이며, 실제 세션/API 키 인증 통합 시험은 아니다.
		if got := requiredScope(http.MethodGet, "/api/v1/personal/export"); got != "session-only" {
			t.Fatalf("export requiredScope = %q, want session-only", got)
		}
		export := doc.Paths["/personal/export"]["get"]
		want := []map[string][]string{{"sessionCookie": {}}}
		if !reflect.DeepEqual(export.Security, want) {
			t.Errorf("export security = %v, want %v", export.Security, want)
		}
		for _, phrase := range []string{"session-only", "API 키로 호출할 수 없습니다"} {
			if !strings.Contains(export.Description, phrase) {
				t.Errorf("export description missing %q: %q", phrase, export.Description)
			}
		}
		if strings.Contains(export.Description, "people:read") {
			t.Errorf("export still advertises people:read: %q", export.Description)
		}
	})
	t.Run("other operations retain bearer authentication", func(t *testing.T) {
		want := []map[string][]string{{"bearerAuth": {}}}
		if !reflect.DeepEqual(doc.Security, want) {
			t.Errorf("global security = %v, want %v", doc.Security, want)
		}
		if got := doc.Components.SecuritySchemes["bearerAuth"]; !reflect.DeepEqual(got, map[string]string{"type": "http", "scheme": "bearer"}) {
			t.Errorf("bearerAuth changed: %v", got)
		}
		people := doc.Paths["/people/"]["get"]
		if people.Description != "필요 API 키 권한: people:read" {
			t.Errorf("people description = %q", people.Description)
		}
		for path, methods := range doc.Paths {
			for method, op := range methods {
				if path != "/personal/export" && op.Security != nil {
					t.Errorf("%s %s unexpectedly overrides global security: %v", method, path, op.Security)
				}
			}
		}
	})
	t.Run("export response explains completion", func(t *testing.T) {
		description := doc.Paths["/personal/export"]["get"].Responses["200"].Description
		for _, phrase := range []string{"HTTP 200", "JSON 파싱 성공", "complete === true", "complete:false", "failed_section", "people", "interactions", "memories", "links", "일부 또는 빈 배열", "이후 섹션 키", "네트워크/쓰기 실패", "보장하지 않습니다"} {
			if !strings.Contains(description, phrase) {
				t.Errorf("export 200 description missing %q: %q", phrase, description)
			}
		}
	})
}
