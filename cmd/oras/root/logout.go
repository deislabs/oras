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

package root

import (
	"github.com/oras-project/oras-go/v3/registry/remote"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"oras.land/oras/cmd/oras/internal/argument"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/internal/credential"
)

type logoutOptions struct {
	hostname string

	debug   bool
	configs []string
}

func logoutCmd() *cobra.Command {
	var opts logoutOptions
	cmd := &cobra.Command{
		Use:   "logout [flags] <registry>",
		Short: "Log out from a remote registry",
		Long: `Log out from a remote registry

Example - Log out from a registry:
  oras logout localhost:5000

Example - Log out from a registry with a custom auth config path:
  oras logout --registry-config ~/.config/oras/auth.json localhost:5000
`,
		Args: oerrors.CheckArgs(argument.Exactly(1), "the registry you want to log out"),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.hostname = args[0]
			return runLogout(cmd, opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.debug, "debug", "d", false, "debug mode")
	cmd.Flags().StringArrayVarP(&opts.configs, "registry-config", "", nil, "auth config path")
	return cmd
}

func runLogout(cmd *cobra.Command, opts logoutOptions) error {
	ctx := cmd.Context()
	if opts.debug {
		logrus.SetLevel(logrus.DebugLevel)
	}

	store, err := credential.NewStore(opts.configs...)
	if err != nil {
		return err
	}
	return remote.Logout(ctx, store, opts.hostname)
}
