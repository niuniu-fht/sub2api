package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCaptureOpenAIFinalImageURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newCtx := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
		return c
	}

	t.Run("captures http urls from final body", func(t *testing.T) {
		c := newCtx()
		body := []byte(`{"data":[{"url":"https://pub-abc.r2.dev/img/1.png"},{"url":"https://pub-abc.r2.dev/img/2.png"}],"usage":{}}`)
		CaptureOpenAIFinalImageURLs(c, body)
		urls := FinalImageURLsFromContext(c)
		if len(urls) != 2 || urls[0] != "https://pub-abc.r2.dev/img/1.png" {
			t.Fatalf("want 2 urls, got %v", urls)
		}
	})

	t.Run("skips data urls and empty entries", func(t *testing.T) {
		c := newCtx()
		body := []byte(`{"data":[{"url":"data:image/png;base64,AAAA"},{"b64_json":"xxxx"},{"url":"https://cdn.example.com/a.png"}]}`)
		CaptureOpenAIFinalImageURLs(c, body)
		urls := FinalImageURLsFromContext(c)
		if len(urls) != 1 || urls[0] != "https://cdn.example.com/a.png" {
			t.Fatalf("want only the http url, got %v", urls)
		}
	})

	t.Run("dedupes repeated urls", func(t *testing.T) {
		c := newCtx()
		body := []byte(`{"data":[{"url":"https://cdn.example.com/a.png"},{"url":"https://cdn.example.com/a.png"}]}`)
		CaptureOpenAIFinalImageURLs(c, body)
		if urls := FinalImageURLsFromContext(c); len(urls) != 1 {
			t.Fatalf("want deduped 1 url, got %v", urls)
		}
	})

	t.Run("caps stored urls", func(t *testing.T) {
		c := newCtx()
		items := make([]string, 0, 12)
		for i := 0; i < 12; i++ {
			items = append(items, fmt.Sprintf(`{"url":"https://cdn.example.com/%d.png"}`, i))
		}
		body := []byte(`{"data":[` + strings.Join(items, ",") + `]}`)
		CaptureOpenAIFinalImageURLs(c, body)
		if urls := FinalImageURLsFromContext(c); len(urls) != maxOpenAIImageResultURLs {
			t.Fatalf("want cap %d urls, got %d", maxOpenAIImageResultURLs, len(urls))
		}
	})

	t.Run("non-images body leaves nothing captured", func(t *testing.T) {
		c := newCtx()
		CaptureOpenAIFinalImageURLs(c, []byte(`{"choices":[{"message":{"content":"hi"}}]}`))
		if urls := FinalImageURLsFromContext(c); urls != nil {
			t.Fatalf("want nil, got %v", urls)
		}
	})

	t.Run("all-inline body stores nothing", func(t *testing.T) {
		c := newCtx()
		CaptureOpenAIFinalImageURLs(c, []byte(`{"data":[{"b64_json":"xxx"}]}`))
		if urls := FinalImageURLsFromContext(c); urls != nil {
			t.Fatalf("want nil for pure b64 body, got %v", urls)
		}
	})
}
