/*
 * Copyright The Kmesh Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at:
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package server_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kmesh.net/kmesh/mcp/internal/auth"
	"kmesh.net/kmesh/mcp/internal/fake"
	"kmesh.net/kmesh/mcp/server"
)

const initialize = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":` +
	`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"v0"}}}`

func guarded(t *testing.T, c *fake.Cluster) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(server.RequireToken(server.Handler(server.New(c)), auth.Verifier(c)))
	t.Cleanup(srv.Close)
	return srv
}

func post(t *testing.T, url, token string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(initialize))
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestNoTokenIsRejected(t *testing.T) {
	c := fake.New(t).WithToken("good", "system:serviceaccount:kmesh-system:reader")

	res := post(t, guarded(t, c).URL, "")

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 a missing token must not fall through to the server's own access", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	if strings.Contains(string(body), "result") {
		t.Errorf("rejected request still produced a result: %s", body)
	}
}

func TestUnknownTokenIsRejected(t *testing.T) {
	c := fake.New(t).WithToken("good", "system:serviceaccount:kmesh-system:reader")

	res := post(t, guarded(t, c).URL, "forged")

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}

func TestValidTokenIsAccepted(t *testing.T) {
	c := fake.New(t).WithToken("good", "system:serviceaccount:kmesh-system:reader")

	res := post(t, guarded(t, c).URL, "good")

	if res.StatusCode == http.StatusUnauthorized {
		t.Fatalf("status = 401 for a token the cluster recognises")
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
}

func TestTokenReviewFailureIsNotAnOpenDoor(t *testing.T) {
	c := fake.New(t)

	res := post(t, guarded(t, c).URL, "anything")

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", res.StatusCode)
	}
}
