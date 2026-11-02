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

package credential_test

import (
	"testing"

	"github.com/oras-project/oras-go/v3/registry/remote/credentials"
	"oras.land/oras/internal/credential"
)

func Test_Credential_emptyCredential(t *testing.T) {
	cred := credential.Credential("", "")
	if cred != credentials.EmptyCredential {
		t.Fatalf("Expect empty credential but got %v", cred)
	}
}

func Test_Credential_usernamePassword(t *testing.T) {
	expected := credentials.Credential{
		Username: "username",
		Password: "password",
	}
	cred := credential.Credential(expected.Username, expected.Password)
	if cred != expected {
		t.Fatalf("Expected credential to be '%v' but got '%v'", expected, cred)
	}
}

func Test_Credential_refreshToken(t *testing.T) {
	expected := credentials.Credential{
		RefreshToken: "mocked",
	}
	cred := credential.Credential("", expected.RefreshToken)
	if cred != expected {
		t.Fatalf("Expected credential to be '%v' but got '%v'", expected, cred)
	}
}
