package service

import (
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
