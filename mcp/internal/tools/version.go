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

package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kmesh.net/kmesh/mcp/internal/cluster"
	"kmesh.net/kmesh/pkg/version"
)

// The admin API route constants in pkg/status are unexported, so the path is
// restated here.
const routeVersion = "/version"

type VersionArgs struct {
	PodName string `json:"pod_name,omitempty" jsonschema:"name of a single kmesh daemon pod; omit to ask every daemon in the cluster"`
}

// VersionResult is every daemon's version, and whichever daemons did not answer.
type VersionResult struct {
	Nodes       []cluster.Node[version.Info] `json:"nodes"`
	Unreachable []cluster.Unreachable        `json:"unreachable,omitempty"`

	Complete bool   `json:"complete"`
	Summary  string `json:"summary"`
}

// Version reports the version of every kmesh daemon in the cluster.
func Version(cli cluster.Client) mcp.ToolHandlerFor[VersionArgs, VersionResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args VersionArgs) (*mcp.CallToolResult, VersionResult, error) {
		fleet, err := cluster.Ask(ctx, cli, func(ctx context.Context, d cluster.Daemon) (version.Info, error) {
			var info version.Info
			err := cluster.Fetch(ctx, cli, d.Pod, routeVersion, &info)
			return info, err
		}, args.PodName)
		if err != nil {
			return nil, VersionResult{}, err
		}

		return nil, VersionResult{
			Nodes:       fleet.Nodes,
			Unreachable: fleet.Unreachable,
			Complete:    fleet.Complete(),
			Summary:     summarise(fleet) + describeVersions(fleet),
		}, nil
	}
}

func describeVersions(f *cluster.Fleet[version.Info]) string {
	seen := map[string]int{}
	for _, n := range f.Answered() {
		if n.Value != nil && n.Value.GitVersion != "" {
			seen[n.Value.GitVersion]++
		}
	}
	if len(seen) == 0 {
		return ""
	}

	versions := make([]string, 0, len(seen))
	for v := range seen {
		versions = append(versions, v)
	}
	sort.Strings(versions)

	if len(versions) == 1 {
		if !f.Complete() {
			return "; those that answered are on " + versions[0]
		}
		return "; all on " + versions[0]
	}

	parts := make([]string, 0, len(versions))
	for _, v := range versions {
		parts = append(parts, fmt.Sprintf("%s on %d", v, seen[v]))
	}
	return "; versions differ: " + strings.Join(parts, ", ")
}

func summarise[T any](f *cluster.Fleet[T]) string {
	total := len(f.Nodes) + len(f.Unreachable)
	answered := len(f.Answered())
	if total == 0 {
		return fmt.Sprintf("no kmesh daemon pods found in namespace %s", cluster.Namespace)
	}
	if f.Complete() {
		if total == 1 {
			return "the daemon answered"
		}
		return fmt.Sprintf("all %d daemons answered", total)
	}
	unreachable := len(f.Unreachable)
	was := "were"
	if unreachable == 1 {
		was = "was"
	}
	return fmt.Sprintf("%d of %d daemons answered; %d failed, %d %s not reachable",
		answered, total, len(f.Failed()), unreachable, was)
}
