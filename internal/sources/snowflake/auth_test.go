// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package snowflake

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/snowflakedb/gosnowflake/v2"
	"github.com/youmark/pkcs8"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("unable to generate key: %s", err)
	}
	return key
}

func testKeyPEM(t *testing.T, key *rsa.PrivateKey, passphrase string) string {
	t.Helper()
	if passphrase == "" {
		der, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("unable to marshal key: %s", err)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	}
	der, err := pkcs8.MarshalPrivateKey(key, []byte(passphrase), nil)
	if err != nil {
		t.Fatalf("unable to marshal encrypted key: %s", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "ENCRYPTED PRIVATE KEY", Bytes: der}))
}

func testKeyFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "rsa_key.p8")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("unable to write key file: %s", err)
	}
	return path
}

func baseConfig() Config {
	return Config{
		Name:     "my-snowflake-instance",
		Type:     SourceType,
		Account:  "my-account",
		User:     "my_user",
		Database: "my_db",
		Schema:   "my_schema",
	}
}

func TestDsnPasswordAuth(t *testing.T) {
	cfg := baseConfig()
	// Characters that need escaping in a connection string. "+" and "%41" are
	// decoded by a DSN parser unless they are escaped when the DSN is built.
	cfg.Password = "p@ss:w/ord?&=+%41"

	dsn, err := cfg.dsn()
	if err != nil {
		t.Fatalf("unable to build dsn: %s", err)
	}
	got, err := gosnowflake.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("unable to parse dsn: %s", err)
	}
	if got.Password != cfg.Password {
		t.Errorf("incorrect password: want %q, got %q", cfg.Password, got.Password)
	}
	if got.Authenticator != gosnowflake.AuthTypeSnowflake {
		t.Errorf("incorrect authenticator: want %v, got %v", gosnowflake.AuthTypeSnowflake, got.Authenticator)
	}
	if got.Warehouse != "COMPUTE_WH" || got.Role != "ACCOUNTADMIN" {
		t.Errorf("incorrect defaults: got warehouse %q, role %q", got.Warehouse, got.Role)
	}
}

func TestDsnKeyPairAuth(t *testing.T) {
	key := testKey(t)
	tcs := []struct {
		desc string
		in   func(c *Config)
	}{
		{
			desc: "inline private key",
			in:   func(c *Config) { c.PrivateKey = testKeyPEM(t, key, "") },
		},
		{
			desc: "private key path",
			in:   func(c *Config) { c.PrivateKeyPath = testKeyFile(t, testKeyPEM(t, key, "")) },
		},
		{
			desc: "encrypted private key path",
			in: func(c *Config) {
				c.PrivateKeyPath = testKeyFile(t, testKeyPEM(t, key, "my_passphrase"))
				c.PrivateKeyPassphrase = "my_passphrase"
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := baseConfig()
			tc.in(&cfg)

			dsn, err := cfg.dsn()
			if err != nil {
				t.Fatalf("unable to build dsn: %s", err)
			}
			got, err := gosnowflake.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("unable to parse dsn: %s", err)
			}
			if got.Authenticator != gosnowflake.AuthTypeJwt {
				t.Errorf("incorrect authenticator: want %v, got %v", gosnowflake.AuthTypeJwt, got.Authenticator)
			}
			if got.PrivateKey == nil || !got.PrivateKey.Equal(key) {
				t.Errorf("incorrect private key in dsn")
			}
			if got.Password != "" {
				t.Errorf("password should be unset, got %q", got.Password)
			}
		})
	}
}

func TestFailDsn(t *testing.T) {
	key := testKey(t)
	keyPEM := testKeyPEM(t, key, "")

	tcs := []struct {
		desc string
		in   func(c *Config)
		err  string
	}{
		{
			desc: "password and private key",
			in: func(c *Config) {
				c.Password = "my_pass"
				c.PrivateKey = keyPEM
			},
			err: "only one of 'password', 'privateKey' or 'privateKeyPath' can be set",
		},
		{
			desc: "private key and private key path",
			in: func(c *Config) {
				c.PrivateKey = keyPEM
				c.PrivateKeyPath = testKeyFile(t, keyPEM)
			},
			err: "only one of 'privateKey' or 'privateKeyPath' can be set",
		},
		{
			desc: "private key path does not exist",
			in:   func(c *Config) { c.PrivateKeyPath = filepath.Join(t.TempDir(), "missing.p8") },
			err:  "unable to read private key file",
		},
		{
			desc: "private key is not pem",
			in:   func(c *Config) { c.PrivateKey = "not-a-pem-key" },
			err:  "unable to decode private key: no PEM block found",
		},
		{
			desc: "incorrect passphrase",
			in: func(c *Config) {
				c.PrivateKey = testKeyPEM(t, key, "my_passphrase")
				c.PrivateKeyPassphrase = "wrong_passphrase"
			},
			err: "unable to parse private key",
		},
		{
			desc: "passphrase for unencrypted private key",
			in: func(c *Config) {
				c.PrivateKey = keyPEM
				c.PrivateKeyPassphrase = "my_passphrase"
			},
			err: "unable to parse private key",
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			cfg := baseConfig()
			tc.in(&cfg)

			if _, err := cfg.dsn(); err == nil {
				t.Fatalf("expect building the dsn to fail")
			} else if !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("unexpected error: got %q, want it to contain %q", err, tc.err)
			}
		})
	}
}
