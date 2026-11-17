package service

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// BenchmarkCaptureOpenAIFinalImageURLs 量化采集点在请求路径上的同步开销。
// 场景：1 张图、b64_json 未删除（最坏 body ~4MB）与已删除（~1KB）两种形态。
func BenchmarkCaptureOpenAIFinalImageURLs(b *testing.B) {
	gin.SetMode(gin.TestMode)
	smallBody := []byte(`{"data":[{"url":"https://pub-abc.r2.dev/sync-req-1-0.png","b64_json":""}],"created":1}`)
	// 模拟 b64 未删的 4MB body：b64 字段填充 5.4M 字符（≈4MB 解码后）
	bigB64 := strings.Repeat("A", 5_400_000)
	bigBody := []byte(fmt.Sprintf(`{"data":[{"url":"https://pub-abc.r2.dev/sync-req-1-0.png","b64_json":"%s"}],"created":1}`, bigB64))

	mkCtx := func() *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/v1/images/generations", nil)
		return c
	}

	b.Run("small_body_1kb", func(b *testing.B) {
		c := mkCtx()
		b.SetBytes(int64(len(smallBody)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			CaptureOpenAIFinalImageURLs(c, smallBody)
		}
	})
	b.Run("big_body_5mb_b64_kept", func(b *testing.B) {
		c := mkCtx()
		b.SetBytes(int64(len(bigBody)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			CaptureOpenAIFinalImageURLs(c, bigBody)
		}
	})
}
