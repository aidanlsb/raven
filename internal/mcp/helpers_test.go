package mcp

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSuccessEnvelopeFallsBackWhenMarshalFails(t *testing.T) {
	t.Parallel()

	cyclic := map[string]interface{}{}
	cyclic["self"] = cyclic

	out := successEnvelope(cyclic, nil)

	var envelope struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if envelope.OK {
		t.Fatalf("expected fallback error envelope, got: %s", out)
	}
	if envelope.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("error.code=%q, want INTERNAL_ERROR", envelope.Error.Code)
	}
}

func TestServerSendFallsBackWhenMarshalFails(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	server := &Server{out: buf}

	cyclic := map[string]interface{}{}
	cyclic["self"] = cyclic
	server.send(Response{
		JSONRPC: "2.0",
		ID:      7,
		Result:  cyclic,
	})

	var resp struct {
		JSONRPC string    `json:"jsonrpc"`
		ID      int       `json:"id"`
		Error   *RPCError `json:"error"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Fatalf("jsonrpc=%q, want 2.0", resp.JSONRPC)
	}
	if resp.ID != 7 {
		t.Fatalf("id=%d, want 7", resp.ID)
	}
	if resp.Error == nil || resp.Error.Code != -32603 {
		t.Fatalf("unexpected error payload: %#v", resp.Error)
	}
}
