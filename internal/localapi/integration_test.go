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
	reindex := exec.Command(binary, "--vault-path", vault.Path, "--json", "reindex", "--full")
	if out, err := reindex.CombinedOutput(); err != nil {
		t.Fatalf("reindex: %v\n%s", err, out)
	}
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

	sessionReq, err := http.NewRequest(http.MethodGet, envelope.Data.URL+"/v1/session", nil)
	if err != nil {
		t.Fatal(err)
	}
	auth(sessionReq)
	sessionResp, err := client.Do(sessionReq)
	if err != nil {
		t.Fatal(err)
	}
	defer sessionResp.Body.Close()
	if sessionResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(sessionResp.Body)
		t.Fatalf("session status = %d body=%s", sessionResp.StatusCode, body)
	}
	var sessionEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			APIVersion string `json:"api_version"`
			VaultPath  string `json:"vault_path"`
			Editor     string `json:"editor"`
			EditorMode string `json:"editor_mode"`
			Daily      struct {
				Directory   string `json:"directory"`
				TodayID     string `json:"today_id"`
				TodayPath   string `json:"today_path"`
				TodayExists bool   `json:"today_exists"`
			} `json:"daily"`
			Revision struct {
				Generation string `json:"generation"`
				Counter    int64  `json:"counter"`
			} `json:"revision"`
		} `json:"data"`
	}
	if err := json.NewDecoder(sessionResp.Body).Decode(&sessionEnvelope); err != nil {
		t.Fatal(err)
	}
	if !sessionEnvelope.OK {
		t.Fatalf("session envelope = %#v", sessionEnvelope)
	}
	if sessionEnvelope.Data.APIVersion != "1" {
		t.Fatalf("session api_version = %q, want 1", sessionEnvelope.Data.APIVersion)
	}
	if sessionEnvelope.Data.VaultPath != vault.Path {
		t.Fatalf("session vault_path = %q, want %q", sessionEnvelope.Data.VaultPath, vault.Path)
	}
	if sessionEnvelope.Data.EditorMode == "" {
		t.Fatal("session editor_mode is empty")
	}
	if sessionEnvelope.Data.Daily.Directory == "" || sessionEnvelope.Data.Daily.TodayID == "" || sessionEnvelope.Data.Daily.TodayPath == "" {
		t.Fatalf("session daily = %#v, want directory/today_id/today_path", sessionEnvelope.Data.Daily)
	}
	if sessionEnvelope.Data.Revision.Generation == "" {
		t.Fatal("session revision generation is empty")
	}

	revReq, err := http.NewRequest(http.MethodGet, envelope.Data.URL+"/v1/revision", nil)
	if err != nil {
		t.Fatal(err)
	}
	auth(revReq)
	revResp, err := client.Do(revReq)
	if err != nil {
		t.Fatal(err)
	}
	defer revResp.Body.Close()
	if revResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(revResp.Body)
		t.Fatalf("revision status = %d body=%s", revResp.StatusCode, body)
	}
	var revEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Generation string `json:"generation"`
			Counter    int64  `json:"counter"`
		} `json:"data"`
	}
	if err := json.NewDecoder(revResp.Body).Decode(&revEnvelope); err != nil {
		t.Fatal(err)
	}
	if !revEnvelope.OK || revEnvelope.Data.Generation != sessionEnvelope.Data.Revision.Generation {
		t.Fatalf("revision envelope = %#v session revision = %#v", revEnvelope, sessionEnvelope.Data.Revision)
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

	objectsReq, err := http.NewRequest(http.MethodPost, envelope.Data.URL+"/v1/invoke", strings.NewReader(`{"command":"objects","args":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	auth(objectsReq)
	objectsReq.Header.Set("Content-Type", "application/json")
	objectsResp, err := client.Do(objectsReq)
	if err != nil {
		t.Fatal(err)
	}
	defer objectsResp.Body.Close()
	var objectsEnvelope struct {
		OK   bool `json:"ok"`
		Data struct {
			Items []struct {
				ID       string `json:"id"`
				Type     string `json:"type"`
				Name     string `json:"name"`
				FilePath string `json:"file_path"`
			} `json:"items"`
			Total int `json:"total"`
		} `json:"data"`
	}
	if err := json.NewDecoder(objectsResp.Body).Decode(&objectsEnvelope); err != nil {
		t.Fatal(err)
	}
	if !objectsEnvelope.OK || objectsEnvelope.Data.Total < 1 {
		t.Fatalf("objects invoke = %#v", objectsEnvelope)
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
