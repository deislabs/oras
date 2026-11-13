/*
Copyright The ORAS Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package option

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/spf13/cobra"
)

func TestRemote_readSecret_warnings(t *testing.T) {
	tests := []struct {
		name     string
		prefix   string
		withStd  bool
		args     []string
		wantWarn string
	}{
		{
			name:     "unprefixed password warning with stdin flag",
			withStd:  true,
			args:     []string{"--password", "secret"},
			wantWarn: "WARNING! Using --password via the CLI is insecure. Use --password-stdin.\n",
		},
		{
			name:     "unprefixed identity-token warning with stdin flag",
			withStd:  true,
			args:     []string{"--identity-token", "token"},
			wantWarn: "WARNING! Using --identity-token via the CLI is insecure. Use --identity-token-stdin.\n",
		},
		{
			name:     "unprefixed password warning without stdin flag",
			args:     []string{"--password", "secret"},
			wantWarn: "WARNING! Using --password via the CLI is insecure.\n",
		},
		{
			name:     "prefixed from-password warning",
			prefix:   "from-",
			args:     []string{"--from-password", "secret"},
			wantWarn: "WARNING! Using --from-password via the CLI is insecure.\n",
		},
		{
			name:     "prefixed to-identity-token warning",
			prefix:   "to-",
			args:     []string{"--to-identity-token", "token"},
			wantWarn: "WARNING! Using --to-identity-token via the CLI is insecure.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts Remote
			cmd := &cobra.Command{}
			var errBuf bytes.Buffer
			cmd.SetErr(&errBuf)
			if tt.withStd {
				opts.ApplyFlags(cmd.Flags())
			} else {
				opts.ApplyFlagsWithPrefix(cmd.Flags(), tt.prefix, "")
			}
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}
			if err := opts.readSecret(cmd); err != nil {
				t.Fatalf("readSecret() error = %v", err)
			}
			if got := errBuf.String(); got != tt.wantWarn {
				t.Errorf("readSecret() warning = %q, want %q", got, tt.wantWarn)
			}
		})
	}
}

func TestRemote_readSecret_stdin(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		stdin      io.Reader
		wantSecret string
	}{
		{
			name:       "password-stdin trims LF",
			args:       []string{"--password-stdin"},
			stdin:      strings.NewReader("my-secret\n"),
			wantSecret: "my-secret",
		},
		{
			name:       "identity-token-stdin trims CRLF",
			args:       []string{"--identity-token-stdin"},
			stdin:      strings.NewReader("my-token\r\n"),
			wantSecret: "my-token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts Remote
			cmd := &cobra.Command{}
			var errBuf bytes.Buffer
			cmd.SetErr(&errBuf)
			cmd.SetIn(tt.stdin)
			opts.ApplyFlags(cmd.Flags())
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}
			if err := opts.readSecret(cmd); err != nil {
				t.Fatalf("readSecret() error = %v", err)
			}
			if opts.Secret != tt.wantSecret {
				t.Errorf("Remote.Secret = %q, want %q", opts.Secret, tt.wantSecret)
			}
			if errBuf.Len() != 0 {
				t.Errorf("unexpected warning on stderr: %q", errBuf.String())
			}
		})
	}
}

func TestRemote_readSecret_stdinError(t *testing.T) {
	wantErr := errors.New("read failed")
	var opts Remote
	cmd := &cobra.Command{}
	cmd.SetIn(iotest.ErrReader(wantErr))
	opts.ApplyFlags(cmd.Flags())
	if err := cmd.ParseFlags([]string{"--password-stdin"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := opts.readSecret(cmd); !errors.Is(err, wantErr) {
		t.Errorf("readSecret() error = %v, want %v", err, wantErr)
	}
}
