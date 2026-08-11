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
	"net/url"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kmesh.net/kmesh/mcp/internal/cluster"
)

const routeLoggers = "/debug/loggers"

// loggerInfo is what the daemon returns for a named logger.
//
// kmesh declares this twice already, in pkg/status and ctl/log, and neither is
// worth importing: pkg/status needs generated eBPF objects to build at all, and
// ctl/log reaches github.com/cilium/ebpf through pkg/logger, which is a large
// dependency to take on for two strings. So this is a third copy, for the same
// reason the other two exist. Shared wire types belong in a package free of
// eBPF dependencies.
type loggerInfo struct {
	Name  string `json:"name,omitempty"`
	Level string `json:"level,omitempty"`
}

type LoggersArgs struct {
	Name    string `json:"name,omitempty" jsonschema:"logger name; omit to list every logger the daemons have"`
	PodName string `json:"pod_name,omitempty" jsonschema:"name of a single kmesh daemon pod; omit to ask every daemon in the cluster"`
}

// LoggerState is one daemon's answer: every logger it has, with its level.
// Naming a logger narrows the map to that one entry.
type LoggerState struct {
	Levels map[string]string `json:"levels"`
}

type LoggersResult struct {
	Logger      string                      `json:"logger,omitempty"`
	Nodes       []cluster.Node[LoggerState] `json:"nodes"`
	Unreachable []cluster.Unreachable       `json:"unreachable,omitempty"`

	Complete bool   `json:"complete"`
	Summary  string `json:"summary"`
}

func Loggers(cli cluster.Client) mcp.ToolHandlerFor[LoggersArgs, LoggersResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args LoggersArgs) (*mcp.CallToolResult, LoggersResult, error) {
		fleet, err := cluster.Ask(ctx, cli, func(ctx context.Context, d cluster.Daemon) (LoggerState, error) {
			s, err := cluster.Open(ctx, cli, d.Pod)
			if err != nil {
				return LoggerState{}, err
			}
			defer s.Close()

			names := []string{args.Name}
			if args.Name == "" {
				// The daemon has no endpoint returning every level at once, so
				// the names come first and each level follows on the same tunnel.
				if err := s.Get(ctx, routeLoggers, &names); err != nil {
					return LoggerState{}, err
				}
			}

			levels := make(map[string]string, len(names))
			for _, name := range names {
				var info loggerInfo
				if err := s.Get(ctx, routeLoggers+"?name="+url.QueryEscape(name), &info); err != nil {
					return LoggerState{}, err
				}
				levels[name] = info.Level
			}
			return LoggerState{Levels: levels}, nil
		}, args.PodName)
		if err != nil {
			return nil, LoggersResult{}, err
		}

		return nil, LoggersResult{
			Logger:      args.Name,
			Nodes:       fleet.Nodes,
			Unreachable: fleet.Unreachable,
			Complete:    fleet.Complete(),
			Summary:     summarise(fleet) + describeLevels(fleet),
		}, nil
	}
}

func describeLevels(f *cluster.Fleet[LoggerState]) string {
	answered := f.Answered()
	if len(answered) == 0 {
		return ""
	}

	seen := map[string]map[string]bool{}
	for _, n := range answered {
		if n.Value == nil {
			continue
		}
		for name, level := range n.Value.Levels {
			if seen[name] == nil {
				seen[name] = map[string]bool{}
			}
			seen[name][level] = true
		}
	}
	if len(seen) == 0 {
		return ""
	}

	var drifted []string
	for name, levels := range seen {
		if len(levels) > 1 {
			drifted = append(drifted, name+" ("+strings.Join(sortedKeys(levels), ", ")+")")
		}
	}
	sort.Strings(drifted)

	if len(drifted) > 0 {
		return "; levels differ across nodes for " + strings.Join(drifted, ", ")
	}
	if len(answered) == 1 {
		return ""
	}

	if len(seen) == 1 {
		for name, levels := range seen {
			level := sortedKeys(levels)[0]
			if !f.Complete() {
				return fmt.Sprintf("; %s is %s on the %d that answered", name, level, len(answered))
			}
			return "; " + name + " is " + level + " on every node"
		}
	}
	if !f.Complete() {
		return fmt.Sprintf("; the %d that answered agree on all %d loggers", len(answered), len(seen))
	}
	return fmt.Sprintf("; all %d loggers agree across nodes", len(seen))
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
