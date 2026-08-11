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
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const Path = "/mcp"

// RequireToken rejects any request that does not carry a bearer token the
// verifier accepts. There is no fallback to the server's own identity; a
// missing or bad token is a 401, not a quieter path to more access.
func RequireToken(h http.Handler, verify auth.TokenVerifier) http.Handler {
	return auth.RequireBearerToken(verify, &auth.RequireBearerTokenOptions{
		AllowMissingExpiration: true,
	})(h)
}

func Handler(s *mcp.Server) http.Handler {
	return mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{
			Stateless:    true,
			JSONResponse: true,
		},
	)
}
