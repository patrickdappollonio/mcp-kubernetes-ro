package handlers

import (
	"strings"
	"testing"
)

func TestBase64ToolsDoNotEchoTheirInput(t *testing.T) {
	t.Parallel()

	h := NewUtilsHandler()

	tests := []struct {
		name    string
		handler func(t *testing.T) string
		input   string
	}{
		{
			name:  "decode_base64",
			input: "czNjcjN0LXBAc3N3MHJk",
			handler: func(t *testing.T) string {
				text, _ := callTool(t, h.DecodeBase64, map[string]any{"data": "czNjcjN0LXBAc3N3MHJk"})
				return text
			},
		},
		{
			name:  "encode_base64",
			input: "s3cr3t-p@ssw0rd",
			handler: func(t *testing.T) string {
				text, _ := callTool(t, h.EncodeBase64, map[string]any{"data": "s3cr3t-p@ssw0rd"})
				return text
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if text := tt.handler(t); strings.Contains(text, tt.input) {
				t.Fatalf("%s response echoes its input: %s", tt.name, text)
			}
		})
	}
}
