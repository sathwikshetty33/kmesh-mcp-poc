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
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kmesh.net/kmesh/mcp/internal/fake"
	"kmesh.net/kmesh/mcp/server"
)

const versionBody = `{"gitVersion":"v1.2.0","gitCommit":"abc123"}`

func serves(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}
}

func connect(t *testing.T, cli *fake.Cluster) *mcp.ClientSession {
	t.Helper()

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	ss, err := server.New(cli).Connect(context.Background(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "v0"}, nil).
		Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	return cs
}

func callVersion(t *testing.T, cs *mcp.ClientSession) map[string]any {
	t.Helper()
	return callTool(t, cs, "kmesh_version", nil)
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) map[string]any {
	t.Helper()

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported an error: %+v", res.Content)
	}

	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	return out
}

func TestServerBuilds(t *testing.T) {
	cs := connect(t, fake.New(t))

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if !registered(tools.Tools, "kmesh_version") {
		t.Errorf("kmesh_version not registered; got %v", tools.Tools)
	}
	for _, tool := range tools.Tools {
		if tool.OutputSchema == nil {
			t.Errorf("%s has no output schema, so clients cannot tell what it returns", tool.Name)
		}
	}
}

func registered(tools []*mcp.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func listTools(t *testing.T, cs *mcp.ClientSession) []*mcp.Tool {
	t.Helper()
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	return tools.Tools
}

func TestVersionToolReportsWholeFleet(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: serves(versionBody)},
	))

	out := callVersion(t, cs)

	if out["complete"] != true {
		t.Errorf("complete = %v, want true", out["complete"])
	}
	nodes, _ := out["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("nodes = %v, want 2", out["nodes"])
	}
	if s, _ := out["summary"].(string); !strings.Contains(s, "all 2 daemons") {
		t.Errorf("summary = %q", s)
	}
}

func TestVersionToolSaysWhenAnswerIsPartial(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2"},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", NotReady: true, Handler: serves(versionBody)},
	))

	out := callVersion(t, cs)

	if out["complete"] != false {
		t.Errorf("complete = %v, want false", out["complete"])
	}
	if unreachable, _ := out["unreachable"].([]any); len(unreachable) != 1 {
		t.Errorf("unreachable = %v, want 1 entry", out["unreachable"])
	}
	s, _ := out["summary"].(string)
	if !strings.Contains(s, "1 of 3") {
		t.Errorf("summary = %q, want it to state the coverage", s)
	}
}

func TestVersionToolOnEmptyCluster(t *testing.T) {
	cs := connect(t, fake.New(t))

	out := callVersion(t, cs)

	if out["complete"] != true {
		t.Errorf("complete = %v, want true", out["complete"])
	}
	if s, _ := out["summary"].(string); !strings.Contains(s, "no kmesh daemon pods found") {
		t.Errorf("summary = %q, want it to say no daemons were found", s)
	}
}

func TestAgreementIsScopedToDaemonsThatAnswered(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", NotReady: true, Handler: serves(versionBody)},
	))

	s, _ := callVersion(t, cs)["summary"].(string)

	if strings.Contains(s, "all on") {
		t.Errorf("summary = %q; claims every daemon is on one version while one never answered", s)
	}
	if !strings.Contains(s, "those that answered are on v1.2.0") {
		t.Errorf("summary = %q, want the version claim scoped to the daemons that replied", s)
	}
}

func TestAgreementIsUnqualifiedWhenComplete(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: serves(versionBody)},
	))

	if s, _ := callVersion(t, cs)["summary"].(string); !strings.Contains(s, "all on v1.2.0") {
		t.Errorf("summary = %q, want an unhedged claim when every daemon answered", s)
	}
}

func TestDisagreementIsNotHedged(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(`{"gitVersion":"v1.1.0"}`)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: serves(`{"gitVersion":"v1.2.0"}`)},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", NotReady: true, Handler: serves(versionBody)},
	))

	s, _ := callVersion(t, cs)["summary"].(string)

	// Two daemons differing is positive evidence, so it needs no hedge even
	// though a third never answered.
	if !strings.Contains(s, "versions differ") {
		t.Errorf("summary = %q, want the disagreement stated", s)
	}
	if strings.Contains(s, "those that answered") {
		t.Errorf("summary = %q; disagreement should not be hedged", s)
	}
}
