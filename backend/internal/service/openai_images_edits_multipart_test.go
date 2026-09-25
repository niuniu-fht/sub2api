package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConvertOpenAIImagesJSONEditsToMultipart(t *testing.T) {
	pngA := []byte("\x89PNG\r\n\x1a\nAAA")
	pngB := []byte("\x89PNG\r\n\x1a\nBBB")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngA)
		case "/b.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngB)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	// httptest 服务器在环回地址上,临时放行 SSRF 防护。
	origGuard := editsImagePrivateIPGuard
	editsImagePrivateIPGuard = func(ip net.IP) bool { return false }
	defer func() { editsImagePrivateIPGuard = origGuard }()

	parsed := &OpenAIImagesRequest{
		Endpoint:        "/v1/images/edits",
		Prompt:          "a cat",
		N:               1,
		Size:            "1024x1024",
		InputImageURLs:  []string{upstream.URL + "/a.png", upstream.URL + "/b.png"},
	}
	body := []byte(`{"prompt":"a cat","n":1,"size":"1024x1024","images":[{"image_url":"` + upstream.URL + `/a.png"},{"image_url":"` + upstream.URL + `/b.png"}]}`)

	got, contentType, err := convertOpenAIImagesJSONEditsToMultipart(context.Background(), body, parsed)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if !strings.Contains(contentType, "multipart/form-data") {
		t.Fatalf("contentType=%q, want multipart", contentType)
	}
	if !strings.Contains(string(got), `"prompt"`) || !strings.Contains(string(got), "a cat") {
		t.Fatalf("prompt field missing, body=%s", got)
	}
	if !strings.Contains(string(got), string(pngA)) || !strings.Contains(string(got), string(pngB)) {
		t.Fatalf("image payloads missing, body len=%d", len(got))
	}
	if strings.Count(string(got), `name="image"`) != 2 {
		t.Fatalf("image file parts missing, body=%s", got)
	}
}

func TestConvertOpenAIImagesJSONEditsToMultipartBlocksPrivateURL(t *testing.T) {
	parsed := &OpenAIImagesRequest{
		Endpoint:       "/v1/images/edits",
		InputImageURLs: []string{"http://127.0.0.1:9/secret.png"},
	}
	body := []byte(`{"prompt":"p","images":[{"image_url":"http://127.0.0.1:9/secret.png"}]}`)

	_, _, err := convertOpenAIImagesJSONEditsToMultipart(context.Background(), body, parsed)
	if err == nil {
		t.Fatalf("expected private address to be blocked")
	}
	if !strings.Contains(err.Error(), "reference image 1 download failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAccountWantsEditsMultipartUpload(t *testing.T) {
	if accountWantsEditsMultipartUpload(&Account{Platform: PlatformOpenAI}) {
		t.Fatalf("default must be false")
	}
	if !accountWantsEditsMultipartUpload(&Account{Extra: map[string]any{editsMultipartExtraKey: true}}) {
		t.Fatalf("enabled flag not honored")
	}
}
