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

package cluster_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"kmesh.net/kmesh/mcp/internal/cluster"
	"kmesh.net/kmesh/mcp/internal/fake"
)

func serves(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}
}

func status(code int, body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}
}

const versionBody = `{"gitVersion":"v1.2.0","gitCommit":"abc123"}`

type versionInfo struct {
	GitVersion string `json:"gitVersion"`
	GitCommit  string `json:"gitCommit"`
}

func askVersions(t *testing.T, c *fake.Cluster) *cluster.Fleet[versionInfo] {
	t.Helper()
	fleet, err := cluster.Ask(context.Background(), c,
		func(ctx context.Context, d cluster.Daemon) (versionInfo, error) {
			var v versionInfo
			err := cluster.Fetch(ctx, c, d.Pod, "/version", &v)
			return v, err
		})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	return fleet
}

func TestDiscoverKeepsOnlyReachablePods(t *testing.T) {
	c := fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", NotReady: true, Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", Terminating: true, Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-d", Node: "node-4", Phase: corev1.PodPending, Handler: serves(versionBody)},
	)

	d, err := cluster.Discover(context.Background(), c)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(d.Ready) != 1 || d.Ready[0].Pod != "kmesh-a" {
		t.Fatalf("Ready = %+v, want only kmesh-a", d.Ready)
	}
	if len(d.Unreachable) != 3 {
		t.Fatalf("Unreachable = %+v, want 3", d.Unreachable)
	}

	reasons := map[string]string{}
	for _, u := range d.Unreachable {
		reasons[u.Pod] = u.Reason
	}
	for pod, want := range map[string]string{
		"kmesh-b": "ContainersNotReady",
		"kmesh-c": "Terminating",
		"kmesh-d": "Pending",
	} {
		if reasons[pod] != want {
			t.Errorf("reason for %s = %q, want %q", pod, reasons[pod], want)
		}
	}
}

func TestAskAllDaemonsAnswer(t *testing.T) {
	c := fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", Handler: serves(versionBody)},
	)

	fleet := askVersions(t, c)

	if !fleet.Complete() {
		t.Errorf("Complete() = false, want true")
	}
	if got := len(fleet.Answered()); got != 3 {
		t.Fatalf("Answered() = %d, want 3", got)
	}
	for _, n := range fleet.Nodes {
		if n.Value == nil || n.Value.GitVersion != "v1.2.0" {
			t.Errorf("%s: Value = %+v, want v1.2.0", n.Pod, n.Value)
		}
	}
}

func TestAskReportsFailuresRatherThanDroppingThem(t *testing.T) {
	c := fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2"},
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", Handler: status(http.StatusInternalServerError, "boom")},
		fake.Daemon{Pod: "kmesh-d", Node: "node-4", NotReady: true, Handler: serves(versionBody)},
	)

	fleet := askVersions(t, c)

	if fleet.Complete() {
		t.Errorf("Complete() = true, want false when daemons are missing")
	}
	if got := len(fleet.Answered()); got != 1 {
		t.Errorf("Answered() = %d, want 1", got)
	}
	if got := len(fleet.Failed()); got != 2 {
		t.Errorf("Failed() = %d, want 2", got)
	}
	if got := len(fleet.Unreachable); got != 1 {
		t.Errorf("Unreachable = %d, want 1", got)
	}

	for _, n := range fleet.Failed() {
		if n.Node == "" {
			t.Errorf("failed node %s lost its node name", n.Pod)
		}
		if n.Value != nil {
			t.Errorf("%s: Value set alongside Error", n.Pod)
		}
		if n.Error == "" {
			t.Errorf("%s: no reason recorded", n.Pod)
		}
	}
}

func TestAskWithNoDaemons(t *testing.T) {
	c := fake.New(t)

	fleet := askVersions(t, c)

	if len(fleet.Nodes) != 0 || len(fleet.Unreachable) != 0 {
		t.Fatalf("expected an empty fleet, got %+v", fleet)
	}

	if !fleet.Complete() {
		t.Errorf("Complete() = false, want true for an empty cluster")
	}
}

func TestAskIsDeterministic(t *testing.T) {
	c := fake.New(t,
		fake.Daemon{Pod: "kmesh-c", Node: "node-3", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
		fake.Daemon{Pod: "kmesh-b", Node: "node-2", Handler: serves(versionBody)},
	)

	for i := 0; i < 5; i++ {
		fleet := askVersions(t, c)
		var pods []string
		for _, n := range fleet.Nodes {
			pods = append(pods, n.Pod)
		}
		if got := strings.Join(pods, ","); got != "kmesh-a,kmesh-b,kmesh-c" {
			t.Fatalf("run %d: order = %s", i, got)
		}
	}
}

func TestFetchSurfacesHTTPStatus(t *testing.T) {
	c := fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: status(http.StatusBadRequest, "\tInvalid Client Mode\n")},
	)

	var v versionInfo
	err := cluster.Fetch(context.Background(), c, "kmesh-a", "/version", &v)
	if err == nil {
		t.Fatal("Fetch: expected an error")
	}

	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "Invalid Client Mode") {
		t.Errorf("error = %q, want the status and the daemon's message", err)
	}
}

func TestFetchRespectsCancellation(t *testing.T) {
	c := fake.New(t,
		fake.Daemon{Pod: "kmesh-a", Node: "node-1", Handler: serves(versionBody)},
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var v versionInfo
	if err := cluster.Fetch(ctx, c, "kmesh-a", "/version", &v); err == nil {
		t.Fatal("Fetch: expected an error from a cancelled context")
	}
}
