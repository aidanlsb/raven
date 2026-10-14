//go:build integration

package localapi_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_APIServerAgainstTempVault(t *testing.T) {
	t.Parallel()
	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\nQueen of Asgard.\n").
		Build()

	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", vault.Path, "--json", "api", "--port", "0", "--token", "integration-token")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rvn api: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
	}()

	line, err := readStartupLine(stdout, 5*time.Second)
	if err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("read startup line: %v\nstderr: %s", err, stderr.String())
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			URL   string `json:"url"`
			Token string `json:"token"`
			Port  int    `json:"port"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		t.Fatalf("parse startup JSON %q: %v\nstderr: %s", line, err, stderr.String())
	}
	if !envelope.OK || envelope.Data.URL == "" || envelope.Data.Token != "integration-token" {
		t.Fatalf("startup envelope = %#v (raw %s)", envelope, line)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	auth := func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer integration-token")
	}

	var healthResp *http.Response
	deadline := time.Now().Add(3 * time.Second)
	for {
		healthReq, err := http.NewRequest(http.MethodGet, envelope.Data.URL+"/v1/health", nil)
		if err != nil {
			t.Fatal(err)
		}
		auth(healthReq)
		healthResp, err = client.Do(healthReq)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("health: %v\nstderr: %s", err, stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer healthResp.Body.Close()
	if healthResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(healthResp.Body)
		t.Fatalf("health status = %d body=%s", healthResp.StatusCode, body)
	}

	schemaReq, err := http.NewRequest(http.MethodGet, envelope.Data.URL+"/v1/schema", nil)
	if err != nil {
		t.Fatal(err)
	}
	auth(schemaReq)
	schemaResp, err := client.Do(schemaReq)
	if err != nil {
		t.Fatal(err)
	}
	defer schemaResp.Body.Close()
	if schemaResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(schemaResp.Body)
		t.Fatalf("schema status = %d body=%s", schemaResp.StatusCode, body)
	}

	invokeBody := `{"command":"read","args":{"reference":"people/freya","raw":true}}`
	invokeReq, err := http.NewRequest(http.MethodPost, envelope.Data.URL+"/v1/invoke", strings.NewReader(invokeBody))
	if err != nil {
		t.Fatal(err)
	}
	auth(invokeReq)
	invokeReq.Header.Set("Content-Type", "application/json")
	invokeResp, err := client.Do(invokeReq)
	if err != nil {
		t.Fatal(err)
	}
	defer invokeResp.Body.Close()
	var invokeEnvelope struct {
		OK    bool                   `json:"ok"`
		Data  map[string]interface{} `json:"data"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(invokeResp.Body).Decode(&invokeEnvelope); err != nil {
		t.Fatal(err)
	}
	if !invokeEnvelope.OK {
		t.Fatalf("invoke read failed: %#v", invokeEnvelope)
	}

	blocked := `{"command":"delete","args":{"reference":"people/freya"}}`
	blockReq, err := http.NewRequest(http.MethodPost, envelope.Data.URL+"/v1/invoke", strings.NewReader(blocked))
	if err != nil {
		t.Fatal(err)
	}
	auth(blockReq)
	blockReq.Header.Set("Content-Type", "application/json")
	blockResp, err := client.Do(blockReq)
	if err != nil {
		t.Fatal(err)
	}
	defer blockResp.Body.Close()
	var blockedEnvelope struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(blockResp.Body).Decode(&blockedEnvelope); err != nil {
		t.Fatal(err)
	}
	if blockedEnvelope.OK || blockedEnvelope.Error == nil || blockedEnvelope.Error.Code != "COMMAND_NOT_ALLOWED" {
		t.Fatalf("delete allowlist = %#v, want COMMAND_NOT_ALLOWED", blockedEnvelope)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("api server exit: %v\nstderr: %s", err, stderr.String())
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("api server did not shut down after SIGTERM")
	}
}

func readStartupLine(r io.Reader, timeout time.Duration) (string, error) {
	type result struct {
		line string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		scanner := bufio.NewScanner(r)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				ch <- result{err: err}
				return
			}
			ch <- result{err: io.EOF}
			return
		}
		ch <- result{line: scanner.Text()}
	}()
	select {
	case got := <-ch:
		return got.line, got.err
	case <-time.After(timeout):
		return "", fmt.Errorf("timed out waiting for startup JSON")
	}
}
