//go:build integration

package cli_test

import (
	"bufio"
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

func TestIntegration_LocalAPIStartsAgainstTempVault(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\nQueen of Asgard\n").
		Build()

	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", v.Path, "--json", "api", "--port", "0", "--token", "integration-token")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf
	if err := cmd.Start(); err != nil {
		t.Fatalf("start api: %v", err)
	}
	defer func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		_, _ = cmd.Process.Wait()
	}()

	reader := bufio.NewReader(stdout)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read startup JSON: %v\nstderr=%s", err, stderrBuf.String())
	}

	var startup struct {
		OK   bool `json:"ok"`
		Data struct {
			URL        string `json:"url"`
			Token      string `json:"token"`
			Port       int    `json:"port"`
			APIVersion string `json:"api_version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(line, &startup); err != nil {
		t.Fatalf("parse startup JSON: %v\nraw=%s", err, line)
	}
	if !startup.OK || startup.Data.URL == "" || startup.Data.Token != "integration-token" {
		t.Fatalf("unexpected startup payload: %s", line)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest(http.MethodGet, startup.Data.URL+"/v1/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer integration-token")
	req.Host = fmt.Sprintf("127.0.0.1:%d", startup.Data.Port)

	var health *http.Response
	deadline := time.Now().Add(5 * time.Second)
	for {
		health, err = client.Do(req.Clone(t.Context()))
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("health request failed: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer health.Body.Close()
	if health.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(health.Body)
		t.Fatalf("health status=%d body=%s", health.StatusCode, body)
	}

	invokeBody := `{"command":"read","args":{"reference":"people/freya","raw":true}}`
	invokeReq, err := http.NewRequest(http.MethodPost, startup.Data.URL+"/v1/invoke", strings.NewReader(invokeBody))
	if err != nil {
		t.Fatal(err)
	}
	invokeReq.Header.Set("Authorization", "Bearer integration-token")
	invokeReq.Header.Set("Content-Type", "application/json")
	invokeReq.Host = fmt.Sprintf("127.0.0.1:%d", startup.Data.Port)
	resp, err := client.Do(invokeReq)
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("invoke JSON: %v\nraw=%s", err, raw)
	}
	if !envelope.OK {
		t.Fatalf("invoke failed: %s", raw)
	}
}

func TestIntegration_DeprecatedServeAliasKeepsStdoutClean(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()
	binary := testutil.BuildCLI(t)

	cmd := exec.Command(binary, "--vault-path", v.Path, "serve")
	cmd.Stdin = strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n")
	var stdout strings.Builder
	var stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("deprecated serve failed: %v\nstderr=%s\nstdout=%s", err, stderr.String(), stdout.String())
	}
	if !strings.Contains(stderr.String(), "deprecated") {
		t.Fatalf("expected deprecation notice on stderr, got %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "deprecated") {
		t.Fatalf("deprecation leaked to stdout: %s", stdout.String())
	}
	if !strings.Contains(stdout.String(), `"jsonrpc"`) {
		t.Fatalf("expected JSON-RPC on stdout, got %s", stdout.String())
	}
}
