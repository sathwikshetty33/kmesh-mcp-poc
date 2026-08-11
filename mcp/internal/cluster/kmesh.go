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

// Reusing these kmesh which is also used by kmeshctl so that we avoid duplication of logic.
package cluster

import (
	"kmesh.net/kmesh/ctl/utils"
	"kmesh.net/kmesh/pkg/kube"
)

const (
	Namespace = utils.KmeshNamespace

	Selector = utils.KmeshLabel

	AdminPort = utils.KmeshAdminPort
)

type Client = kube.CLIClient

type PortForwarder = kube.PortForwarder

// Building a Kubernetes client the same way kmeshctl does.
func NewClient() (Client, error) {
	return utils.CreateKubeClient()
}

// Opens a tunnel to one daemon pod's admin port similar to kmeshctl.
func NewPortForwarder(cli Client, podName string) (PortForwarder, error) {
	return utils.CreateKmeshPortForwarder(cli, podName)
}
