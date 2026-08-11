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

// Package fake stands in for a cluster of kmesh daemons.
//
// It implements kmesh's own kube.CLIClient, so tests drive the real Discover,
// Fetch and Ask code rather than a test-only copy of them. Each daemon gets an
// httptest server in place of its admin API, reached through a port forwarder
// that returns that server's address.
package fake

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	gatewayapi "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"

	"kmesh.net/kmesh/pkg/constants"
	"kmesh.net/kmesh/pkg/kube"
)

// This describes one kmesh daemon pod to pretend exists.
type Daemon struct {
	Pod  string
	Node string

	Phase       corev1.PodPhase
	NotReady    bool
	Terminating bool

	Handler http.HandlerFunc
}

type Cluster struct {
	pods    []corev1.Pod
	servers map[string]*httptest.Server
	broken  map[string]bool
	tokens  map[string]string
	kube    kubernetes.Interface
}

// WithToken makes token authenticate as username. Any token not registered
// this way is rejected, so tests get both outcomes.
func (c *Cluster) WithToken(token, username string) *Cluster {
	c.tokens[token] = username
	return c
}

// answerTokenReviews makes the fake clientset resolve TokenReviews against
// whatever WithToken registered.
func (c *Cluster) answerTokenReviews(fk *k8sfake.Clientset) {
	fk.PrependReactor("create", "tokenreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			review, ok := action.(k8stesting.CreateAction).GetObject().(*authv1.TokenReview)
			if !ok {
				return false, nil, nil
			}
			out := review.DeepCopy()
			user, known := c.tokens[review.Spec.Token]
			if !known {
				out.Status = authv1.TokenReviewStatus{Error: "invalid bearer token"}
				return true, out, nil
			}
			out.Status = authv1.TokenReviewStatus{
				Authenticated: true,
				User:          authv1.UserInfo{Username: user},
			}
			return true, out, nil
		})
}

func New(t *testing.T, daemons ...Daemon) *Cluster {
	t.Helper()

	fk := k8sfake.NewSimpleClientset()
	c := &Cluster{
		servers: map[string]*httptest.Server{},
		broken:  map[string]bool{},
		tokens:  map[string]string{},
		kube:    fk,
	}
	c.answerTokenReviews(fk)

	for _, d := range daemons {
		p := pod(d)
		c.pods = append(c.pods, p)
		// Also seed the clientset, so a lookup by pod name finds it.
		if _, err := c.kube.CoreV1().Pods(p.Namespace).Create(context.Background(), &p, metav1.CreateOptions{}); err != nil {
			t.Fatalf("seeding pod %s: %v", d.Pod, err)
		}
		if d.Handler == nil {
			c.broken[d.Pod] = true
			continue
		}
		srv := httptest.NewServer(d.Handler)
		t.Cleanup(srv.Close)
		c.servers[d.Pod] = srv
	}
	return c
}

func pod(d Daemon) corev1.Pod {
	phase := d.Phase
	if phase == "" {
		phase = corev1.PodRunning
	}
	ready := corev1.ConditionTrue
	if d.NotReady {
		ready = corev1.ConditionFalse
	}

	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      d.Pod,
			Namespace: kube.KmeshNamespace,
			Labels:    map[string]string{"app": "kmesh"},
		},
		Spec: corev1.PodSpec{NodeName: d.Node},
		Status: corev1.PodStatus{
			Phase: phase,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: ready, Reason: readyReason(d)},
			},
		},
	}
	if d.Terminating {
		now := metav1.Now()
		p.DeletionTimestamp = &now
	}
	return p
}

func readyReason(d Daemon) string {
	if d.NotReady {
		return "ContainersNotReady"
	}
	return ""
}

func (c *Cluster) WithNamespace(t *testing.T, name, mode string) *Cluster {
	t.Helper()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	if mode != "" {
		ns.Labels = map[string]string{constants.DataPlaneModeLabel: mode}
	}
	if _, err := c.kube.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seeding namespace %s: %v", name, err)
	}
	return c
}

func (c *Cluster) Kube() kubernetes.Interface { return c.kube }

func (c *Cluster) GatewayAPI() gatewayapi.Interface { return nil }

func (c *Cluster) PodsForSelector(_ context.Context, _ string, _ ...string) (*corev1.PodList, error) {
	return &corev1.PodList{Items: c.pods}, nil
}

func (c *Cluster) NewPortForwarder(podName, _, _ string, _, _ int) (kube.PortForwarder, error) {
	if c.broken[podName] {
		return &forwarder{broken: true}, nil
	}
	srv, ok := c.servers[podName]
	if !ok {
		return nil, errors.New("no such pod: " + podName)
	}
	return &forwarder{addr: strings.TrimPrefix(srv.URL, "http://")}, nil
}

type forwarder struct {
	addr   string
	broken bool
}

func (f *forwarder) Start() error {
	if f.broken {
		return errors.New("connection refused")
	}
	return nil
}

func (f *forwarder) Address() string { return f.addr }

func (f *forwarder) Close() {}
