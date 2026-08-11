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
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kmesh.net/kmesh/pkg/constants"
)

type MeshNamespace struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

func MeshNamespaces(ctx context.Context, cli Client) ([]MeshNamespace, error) {
	list, err := cli.Kube().CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: constants.DataPlaneModeLabel,
	})
	if err != nil {
		return nil, fmt.Errorf("listing namespaces labelled %s: %w", constants.DataPlaneModeLabel, err)
	}

	out := make([]MeshNamespace, 0, len(list.Items))
	for _, ns := range list.Items {
		out = append(out, MeshNamespace{
			Name: ns.Name,
			Mode: ns.Labels[constants.DataPlaneModeLabel],
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
