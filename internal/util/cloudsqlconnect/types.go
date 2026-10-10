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

// Package cloudsql provides shared utilities for Cloud SQL connection tools.
// This package is used by both cloudsqlpg and cloudsqlmysql tool packages
// to provide consistent connection recommendations, validation, and code generation.
package cloudsqlconnect

import (
	"fmt"
	"strings"
)

// DatabaseType represents the type of database.
type DatabaseType string

const (
	// PostgreSQL database type
	PostgreSQL DatabaseType = "postgres"
	// MySQL database type
	MySQL DatabaseType = "mysql"
	// SQLServer (Cloud SQL for SQL Server) database type
	SQLServer DatabaseType = "sqlserver"
)

// AllDatabaseTypes lists every Cloud SQL engine this package supports.
var AllDatabaseTypes = []DatabaseType{PostgreSQL, MySQL, SQLServer}

// ComputeType represents the type of compute environment.
type ComputeType string

const (
	// ComputeGCE represents Google Compute Engine VMs
	ComputeGCE ComputeType = "gce"
	// ComputeGKE represents Google Kubernetes Engine clusters
	ComputeGKE ComputeType = "gke"
	// ComputeCloudRun represents Cloud Run services
	ComputeCloudRun ComputeType = "cloudrun"
	// ComputeLocal represents local development environments
	ComputeLocal ComputeType = "local"
)

// ConnectionMethod represents a method to connect to Cloud SQL.
type ConnectionMethod string

const (
	// MethodAuthProxy uses Cloud SQL Auth Proxy for secure connections
	MethodAuthProxy ConnectionMethod = "auth_proxy"
	// MethodConnector uses Cloud SQL Connector libraries
	MethodConnector ConnectionMethod = "connector"
	// MethodDirectPrivateIP uses direct private IP connection
	MethodDirectPrivateIP ConnectionMethod = "direct_private_ip"
	// MethodUnixSocket uses Unix socket (Cloud Run built-in)
	MethodUnixSocket ConnectionMethod = "unix_socket"
)

// Connection method display names for user-facing messages
const (
	MethodNameAuthProxy       = "Cloud SQL Auth Proxy"
	MethodNameConnector       = "Cloud SQL Connector Library"
	MethodNameDirectPrivateIP = "Direct Private IP Connection"
	MethodNameUnixSocket      = "Built-in Cloud SQL Connection (Unix Socket)"
)

// Language represents a programming language for code generation.
type Language string

const (
	// LangPython represents Python language
	LangPython Language = "python"
	// LangNodeJS represents Node.js/JavaScript
	LangNodeJS Language = "nodejs"
	// LangJava represents Java
	LangJava Language = "java"
	// LangGo represents Go
	LangGo Language = "go"
)

// AvailableLanguages lists all supported languages.
var AvailableLanguages = []Language{LangPython, LangNodeJS, LangJava, LangGo}

// IsValidLanguage checks if the given language is supported.
func IsValidLanguage(lang string) bool {
	normalizedLang := Language(strings.ToLower(lang))
	for _, l := range AvailableLanguages {
		if l == normalizedLang {
			return true
		}
	}
	return false
}

// ValidateLanguage returns an error if the language is not supported.
func ValidateLanguage(lang string) error {
	if lang == "" {
		return nil // empty is valid (means no code snippet requested)
	}
	if !IsValidLanguage(lang) {
		return fmt.Errorf("unsupported language %q: supported languages are %v", lang, AvailableLanguages)
	}
	return nil
}

// ValidationCheck represents a single validation check result.
type ValidationCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "pass", "fail", "warn", "info"
	Message string `json:"message"`
}

// ValidationResult represents the result of network validation.
type ValidationResult struct {
	Valid           bool              `json:"valid"`
	Checks          []ValidationCheck `json:"checks"`
	Issues          []string          `json:"issues,omitempty"`
	Recommendations []string          `json:"recommendations,omitempty"`
}

// ConnectionRecommendation represents a recommended connection method.
type ConnectionRecommendation struct {
	Method         ConnectionMethod `json:"method"`
	Name           string           `json:"name"`
	Description    string           `json:"description"`
	Priority       int              `json:"priority"`
	Security       string           `json:"security"`
	Complexity     string           `json:"complexity"`
	Performance    string           `json:"performance"`
	Requirements   []string         `json:"requirements"`
	Considerations []string         `json:"considerations,omitempty"`
}

// SetupStep represents a step in the setup process.
type SetupStep struct {
	Order       int    `json:"order"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Command     string `json:"command,omitempty"`
}

// CodeSnippet represents generated code for connecting to Cloud SQL.
type CodeSnippet struct {
	Language     Language `json:"language"`
	Code         string   `json:"code"`
	Dependencies []string `json:"dependencies"`
	Notes        []string `json:"notes,omitempty"`
}

// EnvironmentConfig represents environment-specific configuration.
type EnvironmentConfig struct {
	// Common config
	EnvironmentVariables map[string]string `json:"environmentVariables"`

	// Auth Proxy config
	AuthProxyCommand string `json:"authProxyCommand,omitempty"`

	// GKE-specific config
	KubernetesServiceAccount string `json:"kubernetesServiceAccount,omitempty"`
	SidecarYAML              string `json:"sidecarYaml,omitempty"`
	SecretYAML               string `json:"secretYaml,omitempty"`

	// Cloud Run-specific config
	CloudRunFlags []string `json:"cloudRunFlags,omitempty"`
}

// ConnectionStrings contains connection string templates. Credential
// placeholders use UPPERCASE tokens (USER, PASS) so they're obviously
// not real values.
type ConnectionStrings struct {
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	SocketPath string `json:"socketPath,omitempty"`
	DSN        string `json:"dsn,omitempty"`
	JDBC       string `json:"jdbc,omitempty"`
}

// BuildConnectionStrings returns engine-aware DSN/JDBC templates for the
// given recommended connection method.
func BuildConnectionStrings(method ConnectionMethod, dbType DatabaseType, sqlInfo *CloudSQLInstanceInfo, dbName, connName string) ConnectionStrings {
	port := GetDatabasePort(dbType)
	cs := ConnectionStrings{Port: port}

	switch method {
	case MethodDirectPrivateIP:
		cs.Host = sqlInfo.PrivateIPAddress
		cs.DSN = formatDSN(dbType, sqlInfo.PrivateIPAddress, port, dbName)
		cs.JDBC = formatJDBC(dbType, sqlInfo.PrivateIPAddress, port, dbName)
	case MethodAuthProxy:
		cs.Host = "127.0.0.1"
		cs.DSN = formatDSN(dbType, "127.0.0.1", port, dbName)
		cs.JDBC = formatJDBC(dbType, "127.0.0.1", port, dbName)
	case MethodConnector:
		// Connector libraries take the instance connection name and database
		// name programmatically; there's no single DSN/JDBC string.
		cs.DSN = fmt.Sprintf("Instance: %s, Database: %s", connName, dbName)
	case MethodUnixSocket:
		cs.SocketPath = fmt.Sprintf("/cloudsql/%s", connName)
		// Unix sockets are only meaningful for Postgres/MySQL on Cloud Run.
		switch dbType {
		case PostgreSQL:
			cs.DSN = fmt.Sprintf("postgresql://USER:PASS@/%s?host=%s", dbName, cs.SocketPath)
		case MySQL:
			cs.DSN = fmt.Sprintf("mysql://USER:PASS@unix(%s)/%s", cs.SocketPath, dbName)
		}
	}

	return cs
}

func formatDSN(dbType DatabaseType, host string, port int, dbName string) string {
	switch dbType {
	case PostgreSQL:
		return fmt.Sprintf("postgresql://USER:PASS@%s:%d/%s", host, port, dbName)
	case MySQL:
		return fmt.Sprintf("mysql://USER:PASS@%s:%d/%s", host, port, dbName)
	case SQLServer:
		// go-mssqldb / common ADO.NET style URL.
		return fmt.Sprintf("sqlserver://USER:PASS@%s:%d?database=%s", host, port, dbName)
	}
	return ""
}

func formatJDBC(dbType DatabaseType, host string, port int, dbName string) string {
	switch dbType {
	case PostgreSQL:
		return fmt.Sprintf("jdbc:postgresql://%s:%d/%s", host, port, dbName)
	case MySQL:
		return fmt.Sprintf("jdbc:mysql://%s:%d/%s", host, port, dbName)
	case SQLServer:
		return fmt.Sprintf("jdbc:sqlserver://%s:%d;databaseName=%s", host, port, dbName)
	}
	return ""
}

// ConnectResult is the comprehensive result returned by connect tools.
type ConnectResult struct {
	// Instance information
	InstanceConnectionName string       `json:"instanceConnectionName"`
	Project                string       `json:"project"`
	Region                 string       `json:"region"`
	DatabaseType           DatabaseType `json:"databaseType"`
	DatabaseVersion        string       `json:"databaseVersion"`

	// Compute information
	ComputeType     ComputeType `json:"computeType"`
	ComputeResource string      `json:"computeResource"`
	ComputeLocation string      `json:"computeLocation,omitempty"`

	// Network validation
	Validation ValidationResult `json:"validation"`

	// Recommendations
	RecommendedMethod  ConnectionRecommendation   `json:"recommendedMethod"`
	AlternativeMethods []ConnectionRecommendation `json:"alternativeMethods,omitempty"`

	// Configuration
	ConnectionStrings ConnectionStrings `json:"connectionStrings"`
	EnvironmentConfig EnvironmentConfig `json:"environmentConfig"`

	// Setup instructions
	SetupSteps []SetupStep `json:"setupSteps"`

	// Code snippet (only if language was specified)
	CodeSnippet        *CodeSnippet `json:"codeSnippet,omitempty"`
	AvailableLanguages []Language   `json:"availableLanguages"`

	// Required permissions and APIs
	RequiredIAMRoles []string `json:"requiredIamRoles"`
	RequiredAPIs     []string `json:"requiredApis"`
}

// CloudSQLInstanceInfo contains information about a Cloud SQL instance.
type CloudSQLInstanceInfo struct {
	Name               string
	Project            string
	Region             string
	ConnectionName     string
	DatabaseVersion    string
	DatabaseType       DatabaseType
	PublicIPAddress    string
	PrivateIPAddress   string
	PublicIPEnabled    bool
	PrivateIPEnabled   bool
	VPCNetwork         string
	RequireSSL         bool
	AuthorizedNetworks []string
}

// GCEInstanceInfo contains information about a GCE VM instance.
type GCEInstanceInfo struct {
	Name           string
	Zone           string
	Project        string
	InternalIP     string
	ExternalIP     string
	VPCNetwork     string
	Subnetwork     string
	ServiceAccount string
	HasExternalIP  bool
}

// GKEClusterInfo contains information about a GKE cluster.
type GKEClusterInfo struct {
	Name             string
	Location         string
	Project          string
	VPCNetwork       string
	Subnetwork       string
	WorkloadIdentity bool
	PrivateCluster   bool
	VPCNative        bool
}

// CloudRunServiceInfo contains information about a Cloud Run service.
type CloudRunServiceInfo struct {
	Name              string
	Region            string
	Project           string
	ServiceAccount    string
	VPCConnector      string
	DirectVPCEgress   bool
	CloudSQLInstances []string
}

// GetDatabasePort returns the default listening port for a Cloud SQL engine.
func GetDatabasePort(dbType DatabaseType) int {
	switch dbType {
	case PostgreSQL:
		return 5432
	case MySQL:
		return 3306
	case SQLServer:
		return 1433
	default:
		return 5432
	}
}

// DefaultDatabaseName returns the conventional default database name for an engine.
func DefaultDatabaseName(dbType DatabaseType) string {
	switch dbType {
	case PostgreSQL:
		return "postgres"
	case MySQL:
		return "mysql"
	case SQLServer:
		return "master"
	default:
		return ""
	}
}

// ParseDatabaseType maps a Cloud SQL Admin API DatabaseVersion string
// (e.g. "POSTGRES_15", "MYSQL_8_0", "SQLSERVER_2022_STANDARD") to a DatabaseType.
// Unknown or empty versions fall back to PostgreSQL; callers that need to
// reject unknown values should use ParseDatabaseTypeStrict.
func ParseDatabaseType(version string) DatabaseType {
	upper := strings.ToUpper(version)
	switch {
	case strings.HasPrefix(upper, "POSTGRES"):
		return PostgreSQL
	case strings.HasPrefix(upper, "MYSQL"):
		return MySQL
	case strings.HasPrefix(upper, "SQLSERVER"):
		return SQLServer
	default:
		return PostgreSQL
	}
}

// ParseDatabaseTypeStrict is like ParseDatabaseType but returns an error
// instead of silently defaulting when the DatabaseVersion prefix is not one
// of the engines this package supports. Use it when auto-detecting the
// engine from a live sqladmin response so a new Cloud SQL engine (or a
// malformed value) surfaces a clear diagnostic instead of producing wrong
// output.
func ParseDatabaseTypeStrict(version string) (DatabaseType, error) {
	upper := strings.ToUpper(version)
	switch {
	case strings.HasPrefix(upper, "POSTGRES"):
		return PostgreSQL, nil
	case strings.HasPrefix(upper, "MYSQL"):
		return MySQL, nil
	case strings.HasPrefix(upper, "SQLSERVER"):
		return SQLServer, nil
	default:
		return "", fmt.Errorf("unsupported DatabaseVersion %q: expected a value starting with POSTGRES, MYSQL, or SQLSERVER", version)
	}
}
