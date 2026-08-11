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


package server

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kmesh.net/kmesh/mcp/internal/cluster"
	"kmesh.net/kmesh/mcp/internal/tools"
)

const (
	Name    = "kmesh-mcp"
	Version = "v0.1.0"
)

func New(cli cluster.Client) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    Name,
		Version: Version,
		Title:   "Kmesh",
	}, nil)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kmesh_version",
		Description: "Report the build version of every kmesh daemon in the cluster. " +
			"Kmesh runs one daemon per node, so this answers for the whole mesh and " +
			"names any daemon that did not respond.",
	}, tools.Version(cli))

	return s
}
