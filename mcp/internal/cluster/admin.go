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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const FetchTimeout = 10 * time.Second

// Opens a tunnel to one daemon pod, GETs path from its admin API and
// decodes the JSON body into out. Pass a nil out to ignore the body.

// kmesh's ctl/log.GetJson already does the fetch-and-decode part, but it calls
// http.Get, and Go's default client has no timeout. A daemon that accepts the
// connection and then goes quiet would hang the call for good. A CLI user can
// press Ctrl-C; a long-running server has nobody to do that, so every step here
// runs under a context.

// The tunnel part is duplicated in 6 call sites though ctl/log.GetJson is extracted out;
// it would better to combine tunneling logic with GetJson to a shared helper this can reduce
// the number of lines of codes around those 7 call sites.
func Fetch(ctx context.Context, cli Client, podName, path string, out any) error {
	s, err := Open(ctx, cli, podName)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.Get(ctx, path, out)
}

// Session is one open tunnel to a daemon, reused across several requests.
//
// The daemon has no endpoint that returns every logger's level at once, so
// answering "are the levels consistent" means one request per logger. Opening a
// tunnel per request would mean N+1 SPDY tunnels per daemon for one question;
// this way it is one.
type Session struct {
	fw   PortForwarder
	pod  string
	addr string
}

// Open starts a tunnel to podName. Close it when done.
func Open(ctx context.Context, cli Client, podName string) (*Session, error) {
	fw, err := NewPortForwarder(cli, podName)
	if err != nil {
		return nil, fmt.Errorf("creating tunnel to %s: %w", podName, err)
	}

	// The deadline here bounds bringing the tunnel up, not the session's life,
	// so it is not attached to the returned Session.
	startCtx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		startCtx, cancel = context.WithTimeout(ctx, FetchTimeout)
		defer cancel()
	}
	if err := startForwarder(startCtx, fw); err != nil {
		fw.Close()
		return nil, fmt.Errorf("opening tunnel to %s: %w", podName, err)
	}

	return &Session{fw: fw, pod: podName, addr: fw.Address()}, nil
}

// Pod is the daemon this session is talking to.
func (s *Session) Pod() string { return s.pod }

// Close tears the tunnel down.
func (s *Session) Close() { s.fw.Close() }

// Get fetches path over the open tunnel and decodes the JSON body into out.
// Pass a nil out to ignore the body.
func (s *Session) Get(ctx context.Context, path string, out any) error {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, FetchTimeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+s.addr+path, nil)
	if err != nil {
		return fmt.Errorf("building request for %s: %w", path, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s from %s: %w", path, s.pod, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading %s from %s: %w", path, s.pod, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s from %s: status %d: %s",
			path, s.pod, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decoding %s from %s: %w", path, s.pod, err)
	}
	return nil
}

// startForwarder runs fw.Start under ctx.
//
// kmesh's PortForwarder.Start blocks until the tunnel is ready or fails and
// takes no context, so it runs on its own goroutine and we stop waiting when
// ctx does. The deferred fw.Close in Fetch unblocks it, and the buffered
// channel means the goroutine still exits if we have already given up.
func startForwarder(ctx context.Context, fw PortForwarder) error {
	errCh := make(chan error, 1)
	go func() { errCh <- fw.Start() }()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
