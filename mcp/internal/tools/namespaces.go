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
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kmesh.net/kmesh/mcp/internal/cluster"
)

type NamespacesArgs struct{}

type NamespacesResult struct {
	Namespaces []cluster.MeshNamespace `json:"namespaces"`
	Summary    string                  `json:"summary"`
}

func MeshNamespaces(cli cluster.Client) mcp.ToolHandlerFor[NamespacesArgs, NamespacesResult] {
	return func(ctx context.Context, _ *mcp.CallToolRequest, _ NamespacesArgs) (*mcp.CallToolResult, NamespacesResult, error) {
		namespaces, err := cluster.MeshNamespaces(ctx, cli)
		if err != nil {
			return nil, NamespacesResult{}, err
		}

		return nil, NamespacesResult{Namespaces: namespaces, Summary: describeNamespaces(namespaces)}, nil
	}
}

func describeNamespaces(ns []cluster.MeshNamespace) string {
	var enrolled, optedOut []string
	for _, n := range ns {
		if n.Mode == "none" {
			optedOut = append(optedOut, n.Name)
			continue
		}
		enrolled = append(enrolled, n.Name)
	}

	switch {
	case len(enrolled) == 0 && len(optedOut) == 0:
		return "no namespaces carry the dataplane-mode label"
	case len(enrolled) == 0:
		return fmt.Sprintf("no namespaces are enrolled in the mesh; %d explicitly opted out (%s)",
			len(optedOut), strings.Join(optedOut, ", "))
	case len(optedOut) == 0:
		return fmt.Sprintf("%s enrolled in the mesh (%s)",
			plural(len(enrolled), "namespace"), strings.Join(enrolled, ", "))
	}
	return fmt.Sprintf("%s enrolled in the mesh (%s); %d explicitly opted out (%s)",
		plural(len(enrolled), "namespace"), strings.Join(enrolled, ", "),
		len(optedOut), strings.Join(optedOut, ", "))
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
