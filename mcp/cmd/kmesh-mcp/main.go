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

// Command kmesh-mcp serves kmesh's read-only state to MCP clients.
//
// It finds the kmesh daemons itself and reaches them the same way kmeshctl does,
// so it needs a kubeconfig with permission to list pods in the kmesh namespace
// and to port-forward to them.
package main

import (
	"flag"
	"log"
	"net/http"

	"kmesh.net/kmesh/mcp/internal/cluster"
	"kmesh.net/kmesh/mcp/server"
)

func main() {
	listen := flag.String("listen", ":8080", "address to serve MCP on")
	flag.Parse()

	// Credentials are resolved by client-go, exactly as for kmeshctl: KUBECONFIG,
	// then ~/.kube/config, then an in-cluster service account.
	cli, err := cluster.NewClient()
	if err != nil {
		log.Fatalf("building kubernetes client: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle(server.Path, server.Handler(server.New(cli)))

	log.Printf("%s %s serving on %s%s", server.Name, server.Version, *listen, server.Path)
	if err := (&http.Server{Addr: *listen, Handler: mux}).ListenAndServe(); err != nil {
		log.Fatalf("mcp server: %v", err)
	}
}
