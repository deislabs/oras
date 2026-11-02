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
	"strings"

	"github.com/oras-project/oras-go/v3"
	"github.com/oras-project/oras-go/v3/registry/remote/auth"
	"github.com/oras-project/oras-go/v3/registry/remote/errcode"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// OperationType stands for certain type of operations.
type OperationType int

const (
	// OperationTypeParseArtifactReference represents parsing artifact
	// reference operation.
	OperationTypeParseArtifactReference OperationType = iota + 1
)

// RegistryErrorPrefix is the commandline prefix for errors from registry.
const RegistryErrorPrefix = "Error response from registry:"

// UnsupportedFormatTypeError generates the error message for an invalid type.
type UnsupportedFormatTypeError string

// Error implements the error interface.
func (e UnsupportedFormatTypeError) Error() string {
	return "unsupported format type: " + string(e)
}

// Error is the error type for CLI error messaging.
type Error struct {
	OperationType  OperationType
	Err            error
	Usage          string
	Recommendation string
}

// Unwrap implements the errors.Wrapper interface.
func (o *Error) Unwrap() error {
	return o.Err
}

// Error implements the error interface.
func (o *Error) Error() string {
	ret := o.Err.Error()
	if o.Usage != "" {
		ret += fmt.Sprintf("\nUsage: %s", o.Usage)
	}
	if o.Recommendation != "" {
		ret += fmt.Sprintf("\n%s", o.Recommendation)
	}
	return ret
}

// CheckArgs checks the args with the checker function.
func CheckArgs(checker func(args []string) (bool, string), Usage string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if ok, text := checker(args); !ok {
			return &Error{
				Err:            fmt.Errorf(`%q requires %s but got %d`, cmd.CommandPath(), text, len(args)),
				Usage:          fmt.Sprintf("%s %s", cmd.Parent().CommandPath(), cmd.Use),
				Recommendation: fmt.Sprintf(`Please specify %s as %s. Run "%s -h" for more options and examples`, text, Usage, cmd.CommandPath()),
			}
		}
		return nil
	}
}

// Modifier modifies the error during cmd execution.
type Modifier interface {
	ModifyError(cmd *cobra.Command, err error) (modified bool, modifiedErr error)
}

// Command returns an error-handled cobra command.
func Command(cmd *cobra.Command, handler Modifier) *cobra.Command {
	runE := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		err := runE(cmd, args)
		if err != nil {
			_, err = handler.ModifyError(cmd, err)
			return err
		}
		return nil
	}
	return cmd
}

// ReportErrResp returns the inner error message from errResp.Errors.
// If errResp.Errors is empty, it returns the original errResp.
func ReportErrResp(errResp *errcode.ErrorResponse) error {
	if len(errResp.Errors) == 0 {
		// Example error string:
		// GET "registry.example.com/v2/_catalog": response status code 401: 401
		return errResp
	}
	// Example error string:
	// unauthorized: authentication required
	return errResp.Errors
}

// UnwrapCopyError extracts the underlying error from an oras.CopyError.
// If err is of type *oras.CopyError, it returns the inner error (copyErr.Err).
// Otherwise, it returns the original error unchanged.
func UnwrapCopyError(err error) error {
	var copyErr *oras.CopyError
	if errors.As(err, &copyErr) {
		return copyErr.Err
	}
	return err
}

// TrimErrBasicCredentialNotFound trims the credentials from err.
// Caller should make sure the err is auth.ErrBasicCredentialNotFound.
func TrimErrBasicCredentialNotFound(err error) error {
	toTrim := err
	inner := err
	for {
		var unwrapper interface{ Unwrap() error }
		var multiUnwrapper interface{ Unwrap() []error }
		if errors.As(inner, &unwrapper) {
			toTrim = inner
			inner = unwrapper.Unwrap()
			continue
		} else if errors.As(inner, &multiUnwrapper) {
			for _, errItem := range multiUnwrapper.Unwrap() {
				if errors.Is(errItem, auth.ErrBasicCredentialNotFound) {
					toTrim = errItem
					inner = errItem
					break
				}
			}
			continue
		}
		break
	}
	return reWrap(err, toTrim, auth.ErrBasicCredentialNotFound)
}

// reWrap re-wraps outer to inner by trimming out mid, returns inner if extraction fails.
// +---------- outer ----------+      +------ outer ------+
// |         +---- mid ----+   |      |                   |
// |         |    inner    |   |  =>  |       inner       |
// |         +-------------+   |      |                   |
// +---------------------------+      +-------------------+
func reWrap(outer, mid, inner error) error {
	msgOuter := outer.Error()
	msgMid := mid.Error()
	if idx := strings.Index(msgOuter, msgMid); idx > 0 {
		return fmt.Errorf("%s%w", msgOuter[:idx], inner)
	}
	return inner
}

// NewErrEmptyTagOrDigest creates a new error based on the reference string.
func NewErrEmptyTagOrDigest(ref string, cmd *cobra.Command, needsTag bool) error {
	form := `"<name>@<digest>"`
	errMsg := `no digest specified`
	if needsTag {
		form = fmt.Sprintf(`"<name>:<tag>" or %s`, form)
		errMsg = "no tag or digest specified"
	}
	return &Error{
		OperationType:  OperationTypeParseArtifactReference,
		Err:            fmt.Errorf(`"%s": %s`, ref, errMsg),
		Usage:          fmt.Sprintf("%s %s", cmd.Parent().CommandPath(), cmd.Use),
		Recommendation: fmt.Sprintf(`Please specify a reference in the form of %s. Run "%s -h" for more options and examples`, form, cmd.CommandPath()),
	}
}

// CheckMutuallyExclusiveFlags checks if any mutually exclusive flags are used
// at the same time, returns an error when detecting used exclusive flags.
func CheckMutuallyExclusiveFlags(fs *pflag.FlagSet, exclusiveFlagSet ...string) error {
	changedFlags, _ := checkChangedFlags(fs, exclusiveFlagSet...)
	if len(changedFlags) >= 2 {
		flags := strings.Join(changedFlags, ", ")
		return fmt.Errorf("%s cannot be used at the same time", flags)
	}
	return nil
}

// CheckRequiredTogetherFlags checks if any flags required together are all used,
// returns an error when detecting any flags not used while other flags have been used.
func CheckRequiredTogetherFlags(fs *pflag.FlagSet, requiredTogetherFlags ...string) error {
	changed, unchanged := checkChangedFlags(fs, requiredTogetherFlags...)
	unchangedCount := len(unchanged)
	if unchangedCount != 0 && unchangedCount != len(requiredTogetherFlags) {
		changed := strings.Join(changed, ", ")
		unchanged := strings.Join(unchanged, ", ")
		return fmt.Errorf("%s must be used in conjunction with %s", changed, unchanged)
	}
	return nil
}

func checkChangedFlags(fs *pflag.FlagSet, flagSet ...string) (changedFlags []string, unchangedFlags []string) {
	for _, flagName := range flagSet {
		if fs.Changed(flagName) {
			changedFlags = append(changedFlags, fmt.Sprintf("--%s", flagName))
		} else {
			unchangedFlags = append(unchangedFlags, fmt.Sprintf("--%s", flagName))
		}
	}
	return
}
