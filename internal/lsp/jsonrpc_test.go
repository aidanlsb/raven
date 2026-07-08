package lsp

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestReadMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "simple message",
			input: "Content-Length: 13\r\n\r\n{\"a\":\"hello\"}",
			want:  `{"a":"hello"}`,
		},
		{
			name:  "extra headers ignored",
			input: "Content-Type: application/vscode-jsonrpc; charset=utf-8\r\nContent-Length: 2\r\n\r\n{}",
			want:  `{}`,
		},
		{
			name:  "case-insensitive header name",
			input: "content-length: 2\r\n\r\n{}",
			want:  `{}`,
		},
		{
			name:    "missing content length",
			input:   "Content-Type: text\r\n\r\n{}",
			wantErr: true,
		},
		{
			name:    "invalid content length",
			input:   "Content-Length: nope\r\n\r\n{}",
			wantErr: true,
		},
		{
			name:    "truncated body",
			input:   "Content-Length: 10\r\n\r\n{}",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body, err := readMessage(bufio.NewReader(strings.NewReader(tt.input)))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got body %q", body)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if string(body) != tt.want {
				t.Errorf("body = %q, want %q", body, tt.want)
			}
		})
	}
}

func TestWriteMessageRoundTrip(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	msg := Notification{JSONRPC: "2.0", Method: "test/method", Params: map[string]string{"key": "välue"}}
	if err := writeMessage(&buf, msg); err != nil {
		t.Fatalf("writeMessage failed: %v", err)
	}

	body, err := readMessage(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("readMessage failed: %v", err)
	}
	if !strings.Contains(string(body), `"test/method"`) {
		t.Errorf("body missing method: %s", body)
	}
	if !strings.Contains(string(body), "välue") {
		t.Errorf("body missing multi-byte value: %s", body)
	}
}
