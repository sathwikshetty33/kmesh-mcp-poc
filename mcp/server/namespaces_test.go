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
	"strings"
	"testing"

	"kmesh.net/kmesh/mcp/internal/fake"
)

func TestToolIsRegistered_kmesh_mesh_namespaces(t *testing.T) {
	cs := connect(t, fake.New(t))
	if !registered(listTools(t, cs), "kmesh_mesh_namespaces") {
		t.Errorf("kmesh_mesh_namespaces not registered")
	}
}

func TestMeshNamespacesNeedsNoDaemon(t *testing.T) {
	c := fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1"},
	)
	c.WithNamespace(t, "shop", "Kmesh").
		WithNamespace(t, "payments", "Kmesh").
		WithNamespace(t, "legacy", "none").
		WithNamespace(t, "kube-system", "")

	out := callTool(t, connect(t, c), "kmesh_mesh_namespaces", nil)

	namespaces, _ := out["namespaces"].([]any)
	if len(namespaces) != 3 {
		t.Fatalf("namespaces = %v, want the 3 labelled ones", out["namespaces"])
	}

	var names []string
	modes := map[string]string{}
	for _, n := range namespaces {
		ns, _ := n.(map[string]any)
		name, _ := ns["name"].(string)
		mode, _ := ns["mode"].(string)
		names = append(names, name)
		modes[name] = mode
	}
	if strings.Join(names, ",") != "legacy,payments,shop" {
		t.Errorf("names = %v, want sorted", names)
	}
	if modes["legacy"] != "none" || modes["shop"] != "Kmesh" {
		t.Errorf("modes = %v, want the label values preserved", modes)
	}
	// A namespace labelled dataplane-mode=none is opted out, not enrolled, so
	// the count must not lump all three together.
	s, _ := out["summary"].(string)
	if !strings.Contains(s, "2 namespaces enrolled") {
		t.Errorf("summary = %q, want the 2 genuinely enrolled ones counted", s)
	}
	if !strings.Contains(s, "opted out") || !strings.Contains(s, "legacy") {
		t.Errorf("summary = %q, want legacy reported as opted out", s)
	}
}

func TestMeshNamespacesWhenNoneEnrolled(t *testing.T) {
	c := fake.New(t)
	c.WithNamespace(t, "default", "")

	out := callTool(t, connect(t, c), "kmesh_mesh_namespaces", nil)

	if namespaces, _ := out["namespaces"].([]any); len(namespaces) != 0 {
		t.Errorf("namespaces = %v, want none", out["namespaces"])
	}
	if s, _ := out["summary"].(string); !strings.Contains(s, "no namespaces") {
		t.Errorf("summary = %q", s)
	}
}
