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

package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

type Daemon struct {
	Pod  string `json:"pod"`
	Node string `json:"node"`
}

type Unreachable struct {
	Daemon
	Reason string `json:"reason"`
}

type Discovery struct {
	Ready       []Daemon      `json:"ready"`
	Unreachable []Unreachable `json:"unreachable,omitempty"`
}

// The ctl never checks readiness, so it tunnels to pods that cannot accept one
// and then swallows the failure that follows.
func Discover(ctx context.Context, cli Client) (*Discovery, error) {
	podList, err := cli.PodsForSelector(ctx, Namespace, Selector)
	if err != nil {
		return nil, fmt.Errorf("listing kmesh daemon pods in %s: %w", Namespace, err)
	}

	d := &Discovery{}
	for i := range podList.Items {
		pod := &podList.Items[i]
		daemon := Daemon{Pod: pod.Name, Node: pod.Spec.NodeName}
		if reason, ok := unreachableReason(pod); ok {
			d.Unreachable = append(d.Unreachable, Unreachable{Daemon: daemon, Reason: reason})
			continue
		}
		d.Ready = append(d.Ready, daemon)
	}
	return d, nil
}

func unreachableReason(pod *corev1.Pod) (string, bool) {
	if pod.DeletionTimestamp != nil {
		return "Terminating", true
	}
	if pod.Status.Phase != corev1.PodRunning {
		return string(pod.Status.Phase), true
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			if c.Status == corev1.ConditionTrue {
				return "", false
			}
			if c.Reason != "" {
				return c.Reason, true
			}
			return "NotReady", true
		}
	}
	return "NoReadyCondition", true
}
