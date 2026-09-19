package service

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestApplyOpenAIImagesAccountRequestDefaultsAppendOnly(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"openai_image_request_defaults": map[string]any{
				"response_format": "url",
				"quality":         "high",
			},
		},
	}
	parsed := &OpenAIImagesRequest{}
	body := []byte(`{"model":"gpt-image-2","prompt":"a cat","size":"1024x1024","n":1}`)

	got, _, err := applyOpenAIImagesAccountRequestDefaults(account, body, "application/json", parsed)
	if err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	if gotValue := gjson.GetBytes(got, "response_format").String(); gotValue != "url" {
		t.Fatalf("response_format=%q, want url; body=%s", gotValue, got)
	}
	if gotValue := gjson.GetBytes(got, "quality").String(); gotValue != "high" {
		t.Fatalf("quality=%q, want high; body=%s", gotValue, got)
	}
}

func TestApplyOpenAIImagesAccountRequestDefaultsKeepExistingKey(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"openai_image_request_defaults": map[string]any{
				"response_format": "url",
				"quality":         "high",
			},
		},
	}
	parsed := &OpenAIImagesRequest{}
	body := []byte(`{"model":"gpt-image-2","prompt":"a cat","response_format":"b64_json"}`)

	got, _, err := applyOpenAIImagesAccountRequestDefaults(account, body, "application/json", parsed)
	if err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	if gotValue := gjson.GetBytes(got, "response_format").String(); gotValue != "b64_json" {
		t.Fatalf("response_format=%q, want original b64_json; body=%s", gotValue, got)
	}
	if gotValue := gjson.GetBytes(got, "quality").String(); gotValue != "high" {
		t.Fatalf("quality=%q, want high; body=%s", gotValue, got)
	}
}

func TestApplyOpenAIImagesAccountRequestDefaultsOverrideExistingKey(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"openai_image_request_defaults": map[string]any{
				"quality": "high",
				"size":    "1536x1024",
			},
			"openai_image_request_defaults_override": []any{"quality", "size"},
		},
	}
	parsed := &OpenAIImagesRequest{}
	body := []byte(`{"model":"gpt-image-2","prompt":"a cat","quality":"low","size":"1024x1024","n":1}`)

	got, _, err := applyOpenAIImagesAccountRequestDefaults(account, body, "application/json", parsed)
	if err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	if gotValue := gjson.GetBytes(got, "quality").String(); gotValue != "high" {
		t.Fatalf("quality=%q, want overridden high; body=%s", gotValue, got)
	}
	if gotValue := gjson.GetBytes(got, "size").String(); gotValue != "1536x1024" {
		t.Fatalf("size=%q, want overridden 1536x1024; body=%s", gotValue, got)
	}
	if parsed.Quality != "high" {
		t.Fatalf("parsed.Quality=%q, want high", parsed.Quality)
	}
	if parsed.Size != "1536x1024" || !parsed.ExplicitSize {
		t.Fatalf("parsed.Size=%q ExplicitSize=%v, want overridden 1536x1024/true", parsed.Size, parsed.ExplicitSize)
	}
	if parsed.SizeTier != "1K" {
		t.Fatalf("parsed.SizeTier=%q, want 1K for 1536x1024 (1572864 px below default 2K threshold)", parsed.SizeTier)
	}
}

func TestApplyOpenAIImagesAccountRequestDefaultsWithoutOverrideKeepsExisting(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"openai_image_request_defaults": map[string]any{
				"quality": "high",
			},
			"openai_image_request_defaults_override": []any{"quality"},
		},
	}
	// 覆盖列表为空时保持"仅补缺"语义
	account.Extra["openai_image_request_defaults_override"] = []any{}
	parsed := &OpenAIImagesRequest{Quality: "low"}
	body := []byte(`{"model":"gpt-image-2","prompt":"a cat","quality":"low"}`)

	got, _, err := applyOpenAIImagesAccountRequestDefaults(account, body, "application/json", parsed)
	if err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	if gotValue := gjson.GetBytes(got, "quality").String(); gotValue != "low" {
		t.Fatalf("quality=%q, want original low without override; body=%s", gotValue, got)
	}
	if parsed.Quality != "low" {
		t.Fatalf("parsed.Quality=%q, want low", parsed.Quality)
	}
}

func TestApplyOpenAIImagesMultipartAccountRequestDefaultsOverride(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			"openai_image_request_defaults": map[string]any{
				"quality": "high",
			},
			"openai_image_request_defaults_override": []any{"quality"},
		},
	}
	boundary := "testboundary"
	body := "--testboundary\r\n" +
		"Content-Disposition: form-data; name=\"quality\"\r\n\r\n" +
		"low\r\n" +
		"--testboundary--\r\n"

	got, contentType, err := applyOpenAIImagesAccountRequestDefaults(account, []byte(body), "multipart/form-data; boundary="+boundary, &OpenAIImagesRequest{})
	if err != nil {
		t.Fatalf("apply defaults: %v", err)
	}
	if !strings.Contains(contentType, "multipart/form-data") {
		t.Fatalf("contentType=%q, want multipart", contentType)
	}
	if strings.Contains(string(got), "\r\nlow\r\n") {
		t.Fatalf("quality field not overridden, body=%s", got)
	}
	if !strings.Contains(string(got), "high") {
		t.Fatalf("quality=high missing, body=%s", got)
	}
}
