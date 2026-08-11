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
	"sort"
	"sync"
)

const DefaultParallelism = 8

// Node is one daemon's answer, or the reason it did not give one. Value is nil
// exactly when Error is set.
type Node[T any] struct {
	Daemon
	Value *T     `json:"value,omitempty"`
	Error string `json:"error,omitempty"`
}

// OK reports whether this daemon answered.
func (n Node[T]) OK() bool { return n.Error == "" }

// Fleet is the result of asking every daemon the same question.
//
// We carry failures here instead of dropping or logging so that
// AI agent distinguish between what succeeded and what did not.
type Fleet[T any] struct {
	Nodes       []Node[T]     `json:"nodes"`
	Unreachable []Unreachable `json:"unreachable,omitempty"`
}

func (f Fleet[T]) Answered() []Node[T] {
	out := make([]Node[T], 0, len(f.Nodes))
	for _, n := range f.Nodes {
		if n.OK() {
			out = append(out, n)
		}
	}
	return out
}

func (f Fleet[T]) Failed() []Node[T] {
	out := make([]Node[T], 0, len(f.Nodes))
	for _, n := range f.Nodes {
		if !n.OK() {
			out = append(out, n)
		}
	}
	return out
}

func (f Fleet[T]) Complete() bool {
	return len(f.Unreachable) == 0 && len(f.Failed()) == 0
}

// Runs fn against every reachable daemon and collects an answer or an error
// for each. Naming a pod narrows it to that one daemon. It returns an error
// only if the daemons could not be listed at all; a daemon that fails is a
// result, not an error.
func Ask[T any](ctx context.Context, cli Client, fn func(context.Context, Daemon) (T, error), podName string) (*Fleet[T], error) {
	var d *Discovery
	var err error
	if podName == "" {
		d, err = Discover(ctx, cli)
	} else {
		d, err = DiscoverOne(ctx, cli, podName)
	}
	if err != nil {
		return nil, err
	}

	fleet := &Fleet[T]{
		Nodes:       make([]Node[T], len(d.Ready)),
		Unreachable: d.Unreachable,
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, DefaultParallelism)

	for i, daemon := range d.Ready {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			node := Node[T]{Daemon: daemon}
			value, err := fn(ctx, daemon)
			if err != nil {
				node.Error = err.Error()
			} else {
				node.Value = &value
			}
			fleet.Nodes[i] = node
		}()
	}
	wg.Wait()

	// Pods come back from the API server in whatever order it chose; sort so the
	// same cluster produces the same answer twice.
	sort.Slice(fleet.Nodes, func(i, j int) bool { return fleet.Nodes[i].Pod < fleet.Nodes[j].Pod })
	sort.Slice(fleet.Unreachable, func(i, j int) bool {
		return fleet.Unreachable[i].Pod < fleet.Unreachable[j].Pod
	})
	return fleet, nil
}
