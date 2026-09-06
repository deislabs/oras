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
	"errors"
	"slices"
	"testing"

	"github.com/oras-project/oras-go/v3"
	"github.com/spf13/cobra"
)

func TestBinaryTarget_ModifyError(t *testing.T) {
	testCases := []struct {
		name         string
		target       *BinaryTarget
		err          error
		wantModified bool
		wantPrefix   string
		wantErr      error
	}{
		{
			name: "CopyError with Source origin sets prefix",
			target: &BinaryTarget{
				From: Target{
					Type:         "registry",
					RawReference: "localhost:5000/test:v1",
				},
				To: Target{
					Type:         "oci-layout",
					RawReference: "oci-dir:v1",
				},
			},
			err:          &oras.CopyError{Origin: oras.CopyErrorOriginSource, Err: errors.New("source error")},
			wantModified: true,
			wantPrefix:   `Error from source registry for "localhost:5000/test:v1":`,
			wantErr:      errors.New("source error"),
		},
		{
			name: "CopyError with Destination origin sets prefix",
			target: &BinaryTarget{
				From: Target{
					Type:         "registry",
					RawReference: "localhost:5000/test:v1",
				},
				To: Target{
					Type:         "oci-layout",
					RawReference: "oci-dir:v1",
				},
			},
			err:          &oras.CopyError{Origin: oras.CopyErrorOriginDestination, Err: errors.New("destination error")},
			wantModified: true,
			wantPrefix:   `Error from destination oci-layout for "oci-dir:v1":`,
			wantErr:      errors.New("destination error"),
		},
		{
			name: "CopyError with unknown origin",
			target: &BinaryTarget{
				From: Target{
					Type:         "registry",
					RawReference: "localhost:5000/test:v1",
				},
				To: Target{
					Type:         "oci-layout",
					RawReference: "oci-dir:v1",
				},
			},
			err:          &oras.CopyError{Origin: oras.CopyErrorOrigin(-1), Err: errors.New("unknown error")},
			wantPrefix:   "Error:",
			wantModified: true,
			wantErr:      errors.New("unknown error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			modified, err := tc.target.ModifyError(cmd, tc.err)
			if modified != tc.wantModified {
				t.Errorf("ModifyError() modified = %v, want %v", modified, tc.wantModified)
			}
			if modified && cmd.ErrPrefix() != tc.wantPrefix {
				t.Errorf("ModifyError() cmd.ErrPrefix() = %q, want %q", cmd.ErrPrefix(), tc.wantPrefix)
			}
			if err.Error() != tc.wantErr.Error() {
				t.Errorf("ModifyError() error = %q, want %q", err.Error(), tc.wantErr.Error())
			}
		})
	}
}

func TestBinaryTarget_Parse_resolveFlag(t *testing.T) {
	const (
		src = "src.example:443:10.0.0.1"
		dst = "dst.example:443:10.0.0.2"
	)
	tests := []struct {
		name     string
		args     []string
		wantFrom []string
		wantTo   []string
	}{
		{
			name:     "no base rules",
			args:     []string{"--from-resolve", src, "--to-resolve", dst},
			wantFrom: []string{src},
			wantTo:   []string{dst},
		},
		{
			name:     "base rules with spare capacity",
			args:     []string{"--resolve", "a:1:1.1.1.1", "--resolve", "b:1:1.1.1.1", "--resolve", "c:1:1.1.1.1", "--from-resolve", src, "--to-resolve", dst},
			wantFrom: []string{"a:1:1.1.1.1", "b:1:1.1.1.1", "c:1:1.1.1.1", src},
			wantTo:   []string{"a:1:1.1.1.1", "b:1:1.1.1.1", "c:1:1.1.1.1", dst},
		},
		{
			name:     "base rules without spare capacity",
			args:     []string{"--resolve", "a:1:1.1.1.1", "--resolve", "b:1:1.1.1.1", "--resolve", "c:1:1.1.1.1", "--resolve", "d:1:1.1.1.1", "--from-resolve", src, "--to-resolve", dst},
			wantFrom: []string{"a:1:1.1.1.1", "b:1:1.1.1.1", "c:1:1.1.1.1", "d:1:1.1.1.1", src},
			wantTo:   []string{"a:1:1.1.1.1", "b:1:1.1.1.1", "c:1:1.1.1.1", "d:1:1.1.1.1", dst},
		},
		{
			name:     "only one side-specific rule",
			args:     []string{"--resolve", "a:1:1.1.1.1", "--resolve", "b:1:1.1.1.1", "--resolve", "c:1:1.1.1.1", "--from-resolve", src},
			wantFrom: []string{"a:1:1.1.1.1", "b:1:1.1.1.1", "c:1:1.1.1.1", src},
			wantTo:   []string{"a:1:1.1.1.1", "b:1:1.1.1.1", "c:1:1.1.1.1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := BinaryTarget{
				From: Target{RawReference: "src.example/repo:tag"},
				To:   Target{RawReference: "dst.example/repo:tag"},
			}
			cmd := &cobra.Command{}
			opts.ApplyFlags(cmd.Flags())
			if err := cmd.ParseFlags(tt.args); err != nil {
				t.Fatalf("cmd.ParseFlags() error = %v", err)
			}
			if err := opts.Parse(cmd); err != nil {
				t.Fatalf("BinaryTarget.Parse() error = %v", err)
			}
			if !slices.Equal(opts.From.resolveFlag, tt.wantFrom) {
				t.Errorf("From.resolveFlag = %v, want %v", opts.From.resolveFlag, tt.wantFrom)
			}
			if !slices.Equal(opts.To.resolveFlag, tt.wantTo) {
				t.Errorf("To.resolveFlag = %v, want %v", opts.To.resolveFlag, tt.wantTo)
			}
		})
	}
}
