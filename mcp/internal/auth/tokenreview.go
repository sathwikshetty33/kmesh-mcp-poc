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

// Package auth establishes who is calling the server.
//
// It authenticates; it does not delegate. Every caller must present a
// Kubernetes token that the cluster recognises, but the tools still run with
// the server's own credentials, because kmesh's port forwarder builds its
// credentials from a field that cannot be set from outside pkg/kube. The README
// names the upstream change that would close that gap.
//
// There is deliberately no fallback to the server's own identity when a token
// is missing or bad. A fallback would let a caller obtain the server's
// privileges simply by omitting the header.
package auth

import (
	"context"
	"fmt"
	"net/http"

	sdkauth "github.com/modelcontextprotocol/go-sdk/auth"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kmesh.net/kmesh/mcp/internal/cluster"
)

func Verifier(cli cluster.Client) sdkauth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*sdkauth.TokenInfo, error) {
		review, err := cli.Kube().AuthenticationV1().TokenReviews().Create(ctx,
			&authv1.TokenReview{Spec: authv1.TokenReviewSpec{Token: token}},
			metav1.CreateOptions{})
		if err != nil {
			// The review itself failed, which is not the same as the token being
			// rejected: it usually means this server lacks the RBAC to ask.
			return nil, fmt.Errorf("token review failed: %w", err)
		}

		if !review.Status.Authenticated {
			if review.Status.Error != "" {
				return nil, fmt.Errorf("%w: %s", sdkauth.ErrInvalidToken, review.Status.Error)
			}
			return nil, sdkauth.ErrInvalidToken
		}

		// Expiration is left unset: a TokenReview reports whether a token is
		// valid now, not when it stops being valid. Inventing a lifetime would
		// be a fiction, so the middleware is told to allow its absence instead.
		return &sdkauth.TokenInfo{UserID: review.Status.User.Username}, nil
	}
}
