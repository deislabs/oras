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

package errors

import (
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/registry/remote/auth"
	"github.com/oras-project/oras-go/v3/registry/remote/errcode"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestCheckMutuallyExclusiveFlags(t *testing.T) {
	fs := &pflag.FlagSet{}
	var foo, bar, hello bool
	fs.BoolVar(&foo, "foo", false, "foo test")
	fs.BoolVar(&bar, "bar", false, "bar test")
	fs.BoolVar(&hello, "hello", false, "hello test")
	fs.Lookup("foo").Changed = true
	fs.Lookup("bar").Changed = true
	tests := []struct {
		name             string
		exclusiveFlagSet []string
		wantErr          bool
	}{
		{
			"--foo and --bar should not be used at the same time",
			[]string{"foo", "bar"},
			true,
		},
		{
			"--foo and --hello are not used at the same time",
			[]string{"foo", "hello"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckMutuallyExclusiveFlags(fs, tt.exclusiveFlagSet...); (err != nil) != tt.wantErr {
				t.Errorf("CheckMutuallyExclusiveFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckRequiredTogetherFlags(t *testing.T) {
	fs := &pflag.FlagSet{}
	var foo, bar, hello, world bool
	fs.BoolVar(&foo, "foo", false, "foo test")
	fs.BoolVar(&bar, "bar", false, "bar test")
	fs.BoolVar(&hello, "hello", false, "hello test")
	fs.BoolVar(&world, "world", false, "world test")
	fs.Lookup("foo").Changed = true
	fs.Lookup("bar").Changed = true
	tests := []struct {
		name                  string
		requiredTogetherFlags []string
		wantErr               bool
	}{
		{
			"--foo and --bar are both used, no error is returned",
			[]string{"foo", "bar"},
			false,
		},
		{
			"--foo and --hello are not both used, an error is returned",
			[]string{"foo", "hello"},
			true,
		},
		{
			"none of --hello and --world is used, no error is returned",
			[]string{"hello", "world"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := CheckRequiredTogetherFlags(fs, tt.requiredTogetherFlags...); (err != nil) != tt.wantErr {
				t.Errorf("CheckRequiredTogetherFlags() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReportErrResp(t *testing.T) {
	// Test case with empty errors
	emptyErrorsResp := &errcode.ErrorResponse{
		Errors:     []errcode.Error{},
		StatusCode: 401,
		URL:        &url.URL{Host: "localhost:5000"},
		Method:     "GET",
	}

	// Test case with non-empty errors
	nonEmptyErrorsResp := &errcode.ErrorResponse{
		Errors: []errcode.Error{
			{
				Code:    "UNAUTHORIZED",
				Message: "authentication required",
			},
			{
				Code:    "INVALID_CREDENTIALS",
				Message: "invalid credentials provided",
				Detail:  "please check your username and password",
			},
		},
		StatusCode: 401,
		URL:        &url.URL{Host: "localhost:5000"},
		Method:     "GET",
	}

	tests := []struct {
		name    string
		errResp *errcode.ErrorResponse
		wantErr error
	}{
		{
			name:    "empty errors",
			errResp: emptyErrorsResp,
			wantErr: emptyErrorsResp,
		},
		{
			name:    "non-empty errors",
			errResp: nonEmptyErrorsResp,
			wantErr: nonEmptyErrorsResp.Errors,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReportErrResp(tt.errResp)
			if got.Error() != tt.wantErr.Error() {
				t.Errorf("ReportErrResp() = %v, want %v", got, tt.wantErr)
			}
		})
	}
}

func TestUnwrapCopyError(t *testing.T) {
	// Create a regular error
	regularErr := fmt.Errorf("regular error")

	// Create an oras.CopyError with an inner error
	innerErr := fmt.Errorf("inner error")
	copyErr := &oras.CopyError{Err: innerErr}

	tests := []struct {
		name     string
		inputErr error
		wantErr  error
	}{
		{
			name:     "nil error",
			inputErr: nil,
			wantErr:  nil,
		},
		{
			name:     "regular error",
			inputErr: regularErr,
			wantErr:  regularErr,
		},
		{
			name:     "copy error",
			inputErr: copyErr,
			wantErr:  innerErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := UnwrapCopyError(tt.inputErr)
			if !errors.Is(gotErr, tt.wantErr) {
				t.Errorf("UnwrapCopyError() = %v, want %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestUnsupportedFormatTypeError_Error(t *testing.T) {
	err := UnsupportedFormatTypeError("yaml")
	if got, want := err.Error(), "unsupported format type: yaml"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestError_Error(t *testing.T) {
	inner := errors.New("inner error")
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "error only",
			err:  &Error{Err: inner},
			want: "inner error",
		},
		{
			name: "with usage",
			err:  &Error{Err: inner, Usage: "oras push <name>"},
			want: "inner error\nUsage: oras push <name>",
		},
		{
			name: "with recommendation",
			err:  &Error{Err: inner, Recommendation: "Please try again"},
			want: "inner error\nPlease try again",
		},
		{
			name: "usage before recommendation",
			err:  &Error{Err: inner, Usage: "oras push <name>", Recommendation: "Please try again"},
			want: "inner error\nUsage: oras push <name>\nPlease try again",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
			if !errors.Is(tt.err, inner) {
				t.Error("errors.Is(err, inner) = false, want true: Unwrap must expose Err")
			}
		})
	}
}

// newTestCommand returns the "push" command of "oras blob push", so that
// CommandPath and its parent's CommandPath have more than one segment.
func newTestCommand() *cobra.Command {
	root := &cobra.Command{Use: "oras"}
	blob := &cobra.Command{Use: "blob"}
	push := &cobra.Command{Use: "push [flags] <name>[@digest] <file>"}
	root.AddCommand(blob)
	blob.AddCommand(push)
	return push
}

func TestCheckArgs(t *testing.T) {
	cmd := newTestCommand()
	check := CheckArgs(func(args []string) (bool, string) {
		return len(args) == 2, "exactly 2 arguments"
	}, "the reference and the file")

	if err := check(cmd, []string{"localhost:5000/repo", "blob.txt"}); err != nil {
		t.Fatalf("CheckArgs() error = %v, want nil", err)
	}

	err := check(cmd, []string{"localhost:5000/repo"})
	var cliErr *Error
	if !errors.As(err, &cliErr) {
		t.Fatalf("CheckArgs() error = %v, want *Error", err)
	}
	if got, want := cliErr.Err.Error(), `"oras blob push" requires exactly 2 arguments but got 1`; got != want {
		t.Errorf("Err = %q, want %q", got, want)
	}
	if got, want := cliErr.Usage, "oras blob push [flags] <name>[@digest] <file>"; got != want {
		t.Errorf("Usage = %q, want %q", got, want)
	}
	if got, want := cliErr.Recommendation, `Please specify exactly 2 arguments as the reference and the file. Run "oras blob push -h" for more options and examples`; got != want {
		t.Errorf("Recommendation = %q, want %q", got, want)
	}
}

type modifierFunc func(cmd *cobra.Command, err error) (bool, error)

func (f modifierFunc) ModifyError(cmd *cobra.Command, err error) (bool, error) {
	return f(cmd, err)
}

func TestCommand(t *testing.T) {
	runErr := errors.New("run failed")
	modifiedErr := errors.New("modified")

	t.Run("error is passed to the modifier", func(t *testing.T) {
		var gotErr error
		cmd := newTestCommand()
		cmd.RunE = func(*cobra.Command, []string) error { return runErr }
		cmd = Command(cmd, modifierFunc(func(_ *cobra.Command, err error) (bool, error) {
			gotErr = err
			return true, modifiedErr
		}))
		if err := cmd.RunE(cmd, nil); !errors.Is(err, modifiedErr) {
			t.Errorf("RunE() error = %v, want %v", err, modifiedErr)
		}
		if !errors.Is(gotErr, runErr) {
			t.Errorf("modifier received %v, want %v", gotErr, runErr)
		}
	})

	t.Run("modifier is skipped on success", func(t *testing.T) {
		cmd := newTestCommand()
		cmd.RunE = func(*cobra.Command, []string) error { return nil }
		cmd = Command(cmd, modifierFunc(func(*cobra.Command, error) (bool, error) {
			t.Error("modifier called for a successful run")
			return true, modifiedErr
		}))
		if err := cmd.RunE(cmd, nil); err != nil {
			t.Errorf("RunE() error = %v, want nil", err)
		}
	})
}

func TestTrimErrBasicCredentialNotFound(t *testing.T) {
	target := auth.ErrBasicCredentialNotFound
	// oras-go wraps the credential error with the request that needed it.
	requestErr := fmt.Errorf("GET %q: %w", "https://localhost:5000/v2/repo/manifests/v1", target)
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "not wrapped",
			err:  target,
			want: "basic credential not found",
		},
		{
			name: "request line is trimmed",
			err:  requestErr,
			want: "basic credential not found",
		},
		{
			name: "outer context is kept",
			err:  fmt.Errorf("Error from source registry for %q: %w", "localhost:5000/repo:v1", requestErr),
			want: `Error from source registry for "localhost:5000/repo:v1": basic credential not found`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TrimErrBasicCredentialNotFound(tt.err)
			if got.Error() != tt.want {
				t.Errorf("TrimErrBasicCredentialNotFound() = %q, want %q", got.Error(), tt.want)
			}
			if !errors.Is(got, target) {
				t.Error("errors.Is(got, auth.ErrBasicCredentialNotFound) = false, want true")
			}
		})
	}
}

func Test_reWrap_midNotInOuter(t *testing.T) {
	inner := errors.New("inner")
	got := reWrap(errors.New("outer"), errors.New("mid"), inner)
	if !errors.Is(got, inner) {
		t.Errorf("reWrap() = %v, want inner", got)
	}
}

func TestNewErrEmptyTagOrDigest(t *testing.T) {
	tests := []struct {
		name               string
		needsTag           bool
		wantErr            string
		wantRecommendation string
	}{
		{
			name:               "tag or digest",
			needsTag:           true,
			wantErr:            `"localhost:5000/repo": no tag or digest specified`,
			wantRecommendation: `Please specify a reference in the form of "<name>:<tag>" or "<name>@<digest>". Run "oras blob push -h" for more options and examples`,
		},
		{
			name:               "digest only",
			needsTag:           false,
			wantErr:            `"localhost:5000/repo": no digest specified`,
			wantRecommendation: `Please specify a reference in the form of "<name>@<digest>". Run "oras blob push -h" for more options and examples`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewErrEmptyTagOrDigest("localhost:5000/repo", newTestCommand(), tt.needsTag)
			var cliErr *Error
			if !errors.As(err, &cliErr) {
				t.Fatalf("NewErrEmptyTagOrDigest() = %v, want *Error", err)
			}
			if cliErr.OperationType != OperationTypeParseArtifactReference {
				t.Errorf("OperationType = %v, want OperationTypeParseArtifactReference", cliErr.OperationType)
			}
			if got := cliErr.Err.Error(); got != tt.wantErr {
				t.Errorf("Err = %q, want %q", got, tt.wantErr)
			}
			if got, want := cliErr.Usage, "oras blob push [flags] <name>[@digest] <file>"; got != want {
				t.Errorf("Usage = %q, want %q", got, want)
			}
			if cliErr.Recommendation != tt.wantRecommendation {
				t.Errorf("Recommendation = %q, want %q", cliErr.Recommendation, tt.wantRecommendation)
			}
		})
	}
}
