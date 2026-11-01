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
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote/errcode"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
)

func TestTarget_Parse_oci_path(t *testing.T) {
	opts := Target{
		Path:         "foo",
		RawReference: "mocked/test",
	}
	cmd := &cobra.Command{}
	ApplyFlags(&opts, cmd.Flags())
	if err := opts.Parse(cmd); err != nil {
		t.Errorf("Target.Parse() error = %v", err)
	}
	if opts.Type != TargetTypeOCILayout {
		t.Errorf("Target.Parse() failed, got %q, want %q", opts.Type, TargetTypeOCILayout)
	}
}

func TestTarget_Parse_oci(t *testing.T) {
	opts := Target{IsOCILayout: true}
	cmd := &cobra.Command{}
	ApplyFlags(&opts, cmd.Flags())
	err := opts.Parse(cmd)
	if !errors.Is(err, errdef.ErrInvalidReference) {
		t.Errorf("Target.Parse() error = %v, expect %v", err, errdef.ErrInvalidReference)
	}
	if opts.Type != TargetTypeOCILayout {
		t.Errorf("Target.Parse() failed, got %q, want %q", opts.Type, TargetTypeOCILayout)
	}
}

func TestTarget_Parse_oci_and_oci_path(t *testing.T) {
	opts := Target{}
	cmd := &cobra.Command{}
	opts.ApplyFlags(cmd.Flags())
	cmd.SetArgs([]string{"--oci-layout", "foo", "--oci-layout-path", "foo"})
	if err := cmd.Execute(); err != nil {
		t.Errorf("cmd.Execute() error = %v", err)
	}
	err := opts.Parse(cmd)
	if err == nil {
		t.Errorf("expect Target.Parse() to fail but not")
	}
	if !strings.Contains(err.Error(), "cannot be used at the same time") {
		t.Errorf("expect error message to contain 'cannot be used at the same time' but not")
	}
}

func TestTarget_Parse_to_oci_and_oci_path(t *testing.T) {
	opts := Target{}
	cmd := &cobra.Command{}
	opts.setFlagDetails("to", "destination")
	opts.ApplyFlags(cmd.Flags())
	cmd.SetArgs([]string{"--to-oci-layout", "foo", "--to-oci-layout-path", "foo"})
	if err := cmd.Execute(); err != nil {
		t.Errorf("cmd.Execute() error = %v", err)
	}
	err := opts.Parse(cmd)
	if err == nil {
		t.Errorf("expect Target.Parse() to fail but not")
	}
	if !strings.Contains(err.Error(), "cannot be used at the same time") {
		t.Errorf("expect error message to contain 'cannot be used at the same time' but not")
	}
}

func TestTarget_Parse_oci_header_flags(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		args   []string
	}{
		{"oci layout", "", []string{"--oci-layout", "--header", "foo:bar"}},
		{"oci layout path", "", []string{"--oci-layout-path", "foo", "--header", "foo:bar"}},
		{"source oci layout path", "from", []string{"--from-oci-layout-path", "foo", "--from-header", "foo:bar"}},
		{"destination oci layout path", "to", []string{"--to-oci-layout-path", "foo", "--to-header", "foo:bar"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := Target{RawReference: "mocked/test"}
			cmd := &cobra.Command{}
			opts.setFlagDetails(tt.prefix, "test")
			opts.ApplyFlags(cmd.Flags())
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("cmd.Execute() error = %v", err)
			}
			err := opts.Parse(cmd)
			if err == nil || !strings.Contains(err.Error(), "custom header flags cannot be used on an OCI image layout target") {
				t.Errorf("Target.Parse() error = %v, want an error about custom header flags", err)
			}
		})
	}
}

func TestTarget_Parse_remote(t *testing.T) {
	opts := Target{
		RawReference: "mocked/test",
		IsOCILayout:  false,
	}
	cmd := &cobra.Command{}
	ApplyFlags(&opts, cmd.Flags())
	if err := opts.Parse(cmd); err != nil {
		t.Errorf("Target.Parse() error = %v", err)
	}
	if opts.Type != TargetTypeRemote {
		t.Errorf("Target.Parse() failed, got %q, want %q", opts.Type, TargetTypeRemote)
	}
}

func TestTarget_Parse_remote_err(t *testing.T) {
	opts := Target{
		RawReference: "/test",
		IsOCILayout:  false,
	}
	cmd := &cobra.Command{}
	ApplyFlags(&opts, cmd.Flags())
	if err := opts.Parse(cmd); err == nil {
		t.Errorf("expect Target.Parse() to fail but not")
	}
}

func Test_parseOCILayoutReference(t *testing.T) {
	opts := Target{
		RawReference: "/test",
		IsOCILayout:  false,
	}
	tests := []struct {
		name    string
		raw     string
		want    string
		want1   string
		wantErr bool
	}{
		{"Empty input", "", "", "", true},
		{"Empty path and tag", ":", "", "", true},
		{"Empty path and digest", "@", "", "", true},
		{"Empty path and valid digest", "@sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a", "", "", true},
		{"Empty digest", "path@", "path", "", false},
		{"Empty tag", "path:", "path", "", false},
		{"path and digest", "path@digest", "path", "digest", false},
		{"path and tag", "path:tag", "path", "tag", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts.RawReference = tt.raw
			err := opts.parseOCILayoutReference()
			if (err != nil) != tt.wantErr {
				t.Errorf("parseOCILayoutReference() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if opts.Path != tt.want {
				t.Errorf("parseOCILayoutReference() got = %v, want %v", opts.Path, tt.want)
			}
			if opts.Reference != tt.want1 {
				t.Errorf("parseOCILayoutReference() got1 = %v, want %v", opts.Reference, tt.want1)
			}
		})
	}
}

func TestTarget_ModifyError_ociLayout(t *testing.T) {
	errClient := errors.New("client error")
	opts := &Target{}
	cmd := &cobra.Command{}
	modified, got := opts.ModifyError(cmd, errClient)

	if modified {
		t.Errorf("expect error not to be modified but received true")
	}
	if !errors.Is(got, errClient) {
		t.Errorf("unexpected output from Target.ModifyError() = %v", got)
	}
	if want := "Error:"; cmd.ErrPrefix() != want {
		t.Errorf("unexpected error prefix set on command: %q, want %q", cmd.ErrPrefix(), want)
	}
}

func TestTarget_ModifyError_NotFound(t *testing.T) {
	// test errdef.ErrNotFound error returned by oci layout and remote
	tests := []struct {
		name            string
		targetType      string
		rawReference    string
		wantErrPrefix   string
		wantModifiedErr error
		wantModified    bool
		isOCILayout     bool
	}{
		{
			name:          "not found",
			targetType:    TargetTypeOCILayout,
			rawReference:  "oci-dir:latest",
			wantErrPrefix: "Error:",
			wantModified:  false,
			isOCILayout:   true,
		},
		{
			name:          "remote not found",
			targetType:    TargetTypeRemote,
			rawReference:  "localhost:5000/test:latest",
			wantErrPrefix: oerrors.RegistryErrorPrefix,
			wantModified:  true,
			isOCILayout:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &Target{
				Type:         tt.targetType,
				RawReference: tt.rawReference,
				IsOCILayout:  tt.isOCILayout,
			}
			cmd := &cobra.Command{}
			originalErr := fmt.Errorf("not found: %w", errdef.ErrNotFound)
			modified, got := opts.ModifyError(cmd, originalErr)
			if modified != tt.wantModified {
				t.Errorf("Target.ModifyError() modified = %v, want %v", modified, tt.wantModified)
			}
			if !errors.Is(got, originalErr) {
				t.Errorf("Target.ModifyError() got = %v, want %v", got, originalErr)
			}
			if cmd.ErrPrefix() != tt.wantErrPrefix {
				t.Errorf("Target.ModifyError() cmd.ErrPrefix() = %q, want %q", cmd.ErrPrefix(), tt.wantErrPrefix)
			}
		})
	}
}

func TestTarget_ModifyError_errResponse(t *testing.T) {
	errResp := &errcode.ErrorResponse{
		URL:        &url.URL{Host: "localhost:5000"},
		StatusCode: http.StatusUnauthorized,
		Errors: errcode.Errors{
			errcode.Error{
				Code:    "NAME_INVALID",
				Message: "invalid name",
			},
		},
	}

	opts := &Target{
		Type:         TargetTypeRemote,
		RawReference: "localhost:5000/test:v1",
	}
	cmd := &cobra.Command{}
	modified, got := opts.ModifyError(cmd, errResp)

	if !modified {
		t.Errorf("expected error to be modified but received %v", modified)
	}
	if got.Error() != errResp.Errors.Error() {
		t.Errorf("unexpected output from Target.ModifyError() = %v", got)
	}
	if cmd.ErrPrefix() != oerrors.RegistryErrorPrefix {
		t.Errorf("unexpected error prefix set on command: %q, want %q", cmd.ErrPrefix(), oerrors.RegistryErrorPrefix)
	}
}

func TestTarget_ModifyError_errInvalidReference(t *testing.T) {
	errResp := &errcode.ErrorResponse{
		URL:        &url.URL{Host: "registry-1.docker.io"},
		StatusCode: http.StatusUnauthorized,
		Errors: errcode.Errors{
			errcode.Error{
				Code:    "000",
				Message: "mocked message",
				Detail:  map[string]string{"mocked key": "mocked value"},
			},
		},
	}
	opts := &Target{
		Type:         TargetTypeRemote,
		RawReference: "invalid-reference",
	}
	cmd := &cobra.Command{}
	modified, got := opts.ModifyError(cmd, errResp)

	if modified {
		t.Errorf("expect error not to be modified but received true")
	}
	if !errors.Is(got, errResp) {
		t.Errorf("unexpected output from Target.ModifyError() = %v", got)
	}
	if want := "Error:"; cmd.ErrPrefix() != want {
		t.Errorf("unexpected error prefix set on command: %q, want %q", cmd.ErrPrefix(), want)
	}
}

func TestTarget_ModifyError_errHostNotMatching(t *testing.T) {
	errResp := &errcode.ErrorResponse{
		URL:        &url.URL{Host: "registry-1.docker.io"},
		StatusCode: http.StatusUnauthorized,
		Errors: errcode.Errors{
			errcode.Error{
				Code:    "000",
				Message: "mocked message",
				Detail:  map[string]string{"mocked key": "mocked value"},
			},
		},
	}

	opts := &Target{
		Type:         TargetTypeRemote,
		RawReference: "registry-2.docker.io/test:tag",
	}
	cmd := &cobra.Command{}
	modified, _ := opts.ModifyError(cmd, errResp)
	if modified {
		t.Errorf("expect error not to be modified but received true")
	}
	if want := "Error:"; cmd.ErrPrefix() != want {
		t.Errorf("unexpected error prefix set on command: %q, want %q", cmd.ErrPrefix(), want)
	}
}

func TestTarget_ModifyError_dockerHint(t *testing.T) {
	type fields struct {
		Remote       Remote
		RawReference string
		Type         string
		Reference    string
		Path         string
		IsOCILayout  bool
	}
	errs := errcode.Errors{
		errcode.Error{
			Code:    "000",
			Message: "mocked message",
			Detail:  map[string]string{"mocked key": "mocked value"},
		},
	}
	tests := []struct {
		name        string
		fields      fields
		err         error
		modifiedErr *oerrors.Error
	}{
		{
			"namespace already exists",
			fields{
				Type:         TargetTypeRemote,
				RawReference: "docker.io/library/alpine:latest",
			},
			&errcode.ErrorResponse{
				URL:        &url.URL{Host: "registry-1.docker.io"},
				StatusCode: http.StatusUnauthorized,
				Errors:     errs,
			},
			&oerrors.Error{Err: errs},
		},
		{
			"no namespace",
			fields{
				Type:         TargetTypeRemote,
				RawReference: "docker.io",
			},
			&errcode.ErrorResponse{
				URL:        &url.URL{Host: "registry-1.docker.io"},
				StatusCode: http.StatusUnauthorized,
				Errors:     errs,
			},
			&oerrors.Error{Err: errs},
		},
		{
			"not 401",
			fields{
				Type:         TargetTypeRemote,
				RawReference: "docker.io",
			},
			&errcode.ErrorResponse{
				URL:        &url.URL{Host: "registry-1.docker.io"},
				StatusCode: http.StatusConflict,
				Errors:     errs,
			},
			&oerrors.Error{Err: errs},
		},
		{
			"should hint",
			fields{
				Type:         TargetTypeRemote,
				RawReference: "docker.io/alpine",
				Path:         "oras test",
			},
			&errcode.ErrorResponse{
				URL:        &url.URL{Host: "registry-1.docker.io"},
				StatusCode: http.StatusUnauthorized,
				Errors:     errs,
			},
			&oerrors.Error{
				Err:            errs,
				Recommendation: "Namespace seems missing. Do you mean ` docker.io/library/alpine`?",
			},
		},
	}

	cmd := &cobra.Command{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &Target{
				Remote:       tt.fields.Remote,
				RawReference: tt.fields.RawReference,
				Type:         tt.fields.Type,
				Reference:    tt.fields.Reference,
				Path:         tt.fields.Path,
				IsOCILayout:  tt.fields.IsOCILayout,
			}
			modified, got := opts.ModifyError(cmd, tt.err)
			var gotErr *oerrors.Error
			if !errors.As(got, &gotErr) {
				t.Errorf("expecting error to be *oerrors.Error but received %T", got)
			}
			if gotErr.Err.Error() != tt.modifiedErr.Err.Error() || gotErr.Usage != tt.modifiedErr.Usage || gotErr.Recommendation != tt.modifiedErr.Recommendation {
				t.Errorf("Target.ModifyError() error = %v, wantErr %v", gotErr, tt.modifiedErr)
			}
			if !modified {
				t.Errorf("Failed to modify %v", tt.err)
			}
		})
	}
}

func TestTarget_EnsureReferenceNotEmpty(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		allowTag  bool
		wantErr   bool
	}{
		{
			name:      "reject tag when digest is required",
			reference: "test",
			allowTag:  false,
			wantErr:   true,
		},
		{
			name:      "accept digest when digest is required",
			reference: "sha256:" + strings.Repeat("a", 64),
			allowTag:  false,
			wantErr:   false,
		},
		{
			name:      "accept tag when tags are allowed",
			reference: "test",
			allowTag:  true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := Target{
				RawReference: "localhost:5000/repo:" + tt.reference,
				Reference:    tt.reference,
			}

			rootCmd := &cobra.Command{Use: "oras"}
			blobCmd := &cobra.Command{Use: "blob"}
			fetchCmd := &cobra.Command{Use: "fetch"}

			rootCmd.AddCommand(blobCmd)
			blobCmd.AddCommand(fetchCmd)

			err := target.EnsureReferenceNotEmpty(fetchCmd, tt.allowTag)
			if (err != nil) != tt.wantErr {
				t.Fatalf("EnsureReferenceNotEmpty() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErr && !strings.Contains(err.Error(), "no digest specified") {
				t.Errorf("EnsureReferenceNotEmpty() error = %v, want digest error", err)
			}
		})
	}
}
