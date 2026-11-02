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

package crypto

import (
	"crypto/x509"
	"errors"
	"os"
)

// LoadCertPool returns a new cert pool loaded from one or more cert files.
func LoadCertPool(paths ...string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for _, path := range paths {
		pemBytes, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if ok := pool.AppendCertsFromPEM(pemBytes); !ok {
			return nil, errors.New("Failed to load certificate in file: " + path)
		}
	}
	return pool, nil
}
