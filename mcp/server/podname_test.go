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
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kmesh.net/kmesh/mcp/internal/fake"
)

func TestVersionForOneNamedPod(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", Handler: serves(versionBody)},
	))

	out := callTool(t, cs, "kmesh_version", map[string]any{"pod_name": "kmesh-b"})

	nodes, _ := out["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("nodes = %v, want just the one asked for", out["nodes"])
	}
	node, _ := nodes[0].(map[string]any)
	if node["pod"] != "kmesh-b" {
		t.Errorf("pod = %v, want kmesh-b", node["pod"])
	}
	if node["node"] != "node-2" {
		t.Errorf("node = %v, want node-2", node["node"])
	}
	if s, _ := out["summary"].(string); !strings.HasPrefix(s, "the daemon answered") {
		t.Errorf("summary = %q, want singular phrasing", s)
	}
}

func TestUnknownPodNameIsAnError(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
	))

	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "kmesh_version",
		Arguments: map[string]any{"pod_name": "kmesh-typo"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected a tool error for an unknown pod, got %+v", res.StructuredContent)
	}
}

func TestNamedPodThatIsNotReady(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", NotReady: true, Handler: serves(versionBody)},
	))

	out := callTool(t, cs, "kmesh_version", map[string]any{"pod_name": "kmesh-a"})

	if out["complete"] != false {
		t.Errorf("complete = %v, want false", out["complete"])
	}
	unreachable, _ := out["unreachable"].([]any)
	if len(unreachable) != 1 {
		t.Fatalf("unreachable = %v, want the pod reported with a reason", out["unreachable"])
	}
	u, _ := unreachable[0].(map[string]any)
	if u["reason"] != "ContainersNotReady" {
		t.Errorf("reason = %v, want ContainersNotReady", u["reason"])
	}
}

func TestLoggersForOneNamedPod(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: loggers(nil, "info")},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: loggers(nil, "debug")},
	))

	out := callTool(t, cs, "kmesh_loggers", map[string]any{"name": "default", "pod_name": "kmesh-b"})

	nodes, _ := out["nodes"].([]any)
	if len(nodes) != 1 {
		t.Fatalf("nodes = %v, want one", out["nodes"])
	}
	if s, _ := out["summary"].(string); strings.Contains(s, "differ") {
		t.Errorf("summary = %q, want no disagreement for a single pod", s)
	}
	node, _ := nodes[0].(map[string]any)
	value, _ := node["value"].(map[string]any)
	levels, _ := value["levels"].(map[string]any)
	if levels["default"] != "debug" {
		t.Errorf("levels = %v, want default=debug", value["levels"])
	}
}
