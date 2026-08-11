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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"kmesh.net/kmesh/mcp/internal/fake"
)

func loggers(names []string, level string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		if name == "" {
			_ = json.NewEncoder(w).Encode(names)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"name": name, "level": level})
	}
}

func TestToolIsRegistered_kmesh_loggers(t *testing.T) {
	cs := connect(t, fake.New(t))
	if !registered(listTools(t, cs), "kmesh_loggers") {
		t.Errorf("kmesh_loggers not registered")
	}
}

func TestLoggersListsNamesFromEveryDaemon(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: loggers([]string{"default", "bpf"}, "info")},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: loggers([]string{"default", "bpf"}, "info")},
	))

	out := callTool(t, cs, "kmesh_loggers", nil)

	if out["complete"] != true {
		t.Errorf("complete = %v, want true", out["complete"])
	}
	nodes, _ := out["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("nodes = %v, want 2", out["nodes"])
	}
	first, _ := nodes[0].(map[string]any)
	value, _ := first["value"].(map[string]any)
	levels, _ := value["levels"].(map[string]any)
	if len(levels) != 2 {
		t.Fatalf("levels = %v, want a level for each of the 2 loggers", value["levels"])
	}
	for name, level := range levels {
		if level == "" {
			t.Errorf("logger %s has no level; listing must not need a second call", name)
		}
	}
}

func TestLoggersFlagsDisagreement(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: loggers(nil, "info")},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: loggers(nil, "info")},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", Handler: loggers(nil, "debug")},
	))

	out := callTool(t, cs, "kmesh_loggers", map[string]any{"name": "default"})

	if out["logger"] != "default" {
		t.Errorf("logger = %v, want default", out["logger"])
	}
	s, _ := out["summary"].(string)
	if !strings.Contains(s, "levels differ across nodes") {
		t.Errorf("summary = %q, want it to flag the disagreement", s)
	}
	if !strings.Contains(s, "debug") || !strings.Contains(s, "info") {
		t.Errorf("summary = %q, want both levels named", s)
	}
}

func TestLoggersAgreementIsNotFlagged(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: loggers(nil, "info")},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: loggers(nil, "info")},
	))

	out := callTool(t, cs, "kmesh_loggers", map[string]any{"name": "default"})

	if s, _ := out["summary"].(string); strings.Contains(s, "differ") {
		t.Errorf("summary = %q, want no disagreement reported", s)
	}
}

func TestLoggerAgreementIsScopedToDaemonsThatAnswered(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: loggers([]string{"default", "bpf"}, "info")},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: loggers([]string{"default", "bpf"}, "info")},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", NotReady: true, Handler: loggers([]string{"default", "bpf"}, "debug")},
	))

	s, _ := callTool(t, cs, "kmesh_loggers", nil)["summary"].(string)

	if strings.Contains(s, "agree across nodes") {
		t.Errorf("summary = %q; claims agreement across nodes while one never answered", s)
	}
	if !strings.Contains(s, "the 2 that answered agree") {
		t.Errorf("summary = %q, want agreement scoped to the daemons that replied", s)
	}
}

func TestNamedLoggerAgreementIsScoped(t *testing.T) {
	cs := connect(t, fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: loggers(nil, "info")},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: loggers(nil, "info")},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", NotReady: true, Handler: loggers(nil, "debug")},
	))

	s, _ := callTool(t, cs, "kmesh_loggers", map[string]any{"name": "default"})["summary"].(string)

	if strings.Contains(s, "on every node") {
		t.Errorf("summary = %q; claims every node while one never answered", s)
	}
	if !strings.Contains(s, "on the 2 that answered") {
		t.Errorf("summary = %q, want the level claim scoped", s)
	}
}
