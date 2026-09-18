package main

import (
	"strings"
	"testing"
)

func TestTextStream(t *testing.T) {
	chunk := "data: {\"id\":\"req-1\",\"choices\":[{\"delta\":{\"content\":\"你好\"},\"finish_reason\":\"stop\"}]}\r\n\r\n"
	usage := "data: {\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":20}}\n\n"
	for _, tc := range []struct {
		name, body string
		ok         bool
	}{
		{"complete", chunk + usage + "data: [DONE]\n\n", true},
		{"missing usage", chunk + "data: [DONE]\n\n", false},
		{"truncated", chunk + usage, false},
		{"missing terminal choice", usage + "data: [DONE]\n\n", false},
		{"broken JSON", "data: {broken}\n\n" + chunk + usage + "data: [DONE]\n\n", false},
		{"upstream error", "data: {\"error\":{\"code\":\"123\"}}\n\n", false},
		{"negative usage", chunk + "data: {\"usage\":{\"prompt_tokens\":-1,\"completion_tokens\":2}}\n\ndata: [DONE]\n\n", false},
		{"partial usage", chunk + "data: {\"usage\":{\"prompt_tokens\":1}}\n\ndata: [DONE]\n\n", false},
		{"final line", chunk + usage + "data: [DONE]", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var preview strings.Builder
			got, err := readTextStream(strings.NewReader(tc.body), func(s string) { preview.WriteString(s) })
			if (err == nil) != tc.ok {
				t.Fatalf("result=%+v error=%v", got, err)
			}
			if tc.ok && (got.Text != "你好" || preview.String() != got.Text || got.Input != 100 || got.Output != 20 || got.ID != "req-1") {
				t.Fatalf("%+v preview=%q", got, preview.String())
			}
		})
	}
}

func TestImageSettings(t *testing.T) {
	for _, tc := range []struct {
		model, size string
		ok          bool
	}{
		{"glm-image", "1152x1536", true},
		{"glm-image", "900x1200", false},
		{"cogview-4", "864x1152", true},
		{"cogview-4", "2048x2048", false},
		{"unknown", "1152x1536", false},
		{"glm-image", "-32x1280", false},
	} {
		t.Setenv("BIGMODEL_IMAGE_MODEL", tc.model)
		t.Setenv("BIGMODEL_IMAGE_SIZE", tc.size)
		_, _, err := imageSettings()
		if (err == nil) != tc.ok {
			t.Fatalf("%+v: %v", tc, err)
		}
	}
}

func TestPaidConfiguration(t *testing.T) {
	t.Setenv("MODEL_MODE", "bigmodel")
	t.Setenv("PAID_CALLS_VERIFIED", "true")
	t.Setenv("BIGMODEL_API_KEY", "test-key")
	t.Setenv("BIGMODEL_TEXT_MODEL", "test-model")
	t.Setenv("TEXT_CONTEXT_TOKENS", "200000")
	t.Setenv("TEXT_MAX_TOKENS", "2048")
	t.Setenv("TEXT_INPUT_PER_MILLION_MICRO", "500000")
	t.Setenv("TEXT_OUTPUT_PER_MILLION_MICRO", "3000000")
	t.Setenv("TEXT_RESERVE_MICRO", "106144")
	if p, err := price("text"); err != nil || p != 106144 {
		t.Fatal(p, err)
	}
	t.Setenv("TEXT_RESERVE_MICRO", "106143")
	if _, err := price("text"); err == nil {
		t.Fatal("under-reserved request accepted")
	}
	t.Setenv("BIGMODEL_API_KEY", "")
	if _, err := price("image"); err == nil {
		t.Fatal("missing key accepted")
	}
}
