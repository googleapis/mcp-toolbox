// Copyright 2025 Google LLC
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

package tests

/* Configurations for RunToolInvokeTest()  */

// InvokeTestConfig represents the various configuration options for RunToolInvokeTest()
type InvokeTestConfig struct {
	myAuthToolWant           string
	myToolId3NameAliceWant   string
	myToolById4Want          string
	nullWant                 string
	myArrayToolWant          string
	supportSelect1Want       bool
	supportOptionalNullParam bool
	supportArrayParam        bool
	supportClientAuth        bool
	supportSelect1Auth       bool
	isMCP                    bool
}

type InvokeTestOption func(*InvokeTestConfig)

// WithMCP enables the MCP routing for standard Tool Invoke tests
func WithMCP() InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.isMCP = true
	}
}

// WithMyAuthToolWant represents the response value for my-auth-tool.
// e.g. tests.RunToolInvokeTest(t, select1Want, tests.WithMyAuthToolWant("custom"))
func WithMyAuthToolWant(s string) InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.myAuthToolWant = s
	}
}

// WithMyToolId3NameAliceWant represents the response value for my-tool with id=3 and name=Alice.
// e.g. tests.RunToolInvokeTest(t, select1Want, tests.WithMyToolId3NameAliceWant("custom"))
func WithMyToolId3NameAliceWant(s string) InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.myToolId3NameAliceWant = s
	}
}

// WithMyArrayToolWant represents the response value for my-array-tool.
// e.g. tests.RunToolInvokeTest(t, select1Want, tests.WithMyArrayToolWant("custom"))
func WithMyArrayToolWant(s string) InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.myArrayToolWant = s
	}
}

// WithMyToolById4Want represents the response value for my-tool-by-id with id=4.
// This response includes a null value column.
// e.g. tests.RunToolInvokeTest(t, select1Want, tests.WithMyToolById4Want("custom"))
func WithMyToolById4Want(s string) InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.myToolById4Want = s
	}
}

// WithNullWant represents a response value of null string.
// e.g. tests.RunToolInvokeTest(t, select1Want, tests.WithNullWant("custom"))
func WithNullWant(s string) InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.nullWant = s
	}
}

// DisableOptionalNullParamTest disables tests for optional null parameters.
// e.g. tests.RunToolInvokeTest(t, select1Want, tests.DisableOptionalNullParamTest())
func DisableOptionalNullParamTest() InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.supportOptionalNullParam = false
	}
}

// DisableArrayTest disables tests for sources that do not support array.
// e.g. tests.RunToolInvokeTest(t, select1Want, tests.DisableArrayTest())
func DisableArrayTest() InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.supportArrayParam = false
	}
}

// DisableSelect1Test disables tests for sources that do not support SELECT 1 query.
// e.g. tests.RunToolInvokeTest(t, "", tests.DisableSelect1Test())
func DisableSelect1Test() InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.supportSelect1Want = false
	}
}

// DisableSelect1AuthTest disables auth tests for sources that do not support SELECT 1 query.
// e.g. tests.RunToolInvokeTest(t, "", tests.DisableSelect1AuthTest())
func DisableSelect1AuthTest() InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.supportSelect1Auth = false
	}
}

// EnableClientAuthTest runs the client authorization tests.
// Only enable it if your source supports the `useClientOAuth` configuration.
// Currently, this should only be used with the BigQuery tests.
func EnableClientAuthTest() InvokeTestOption {
	return func(c *InvokeTestConfig) {
		c.supportClientAuth = true
	}
}

/* Configurations for RunMCPToolCallMethod()  */

// MCPTestConfig represents the various configuration options for mcp tool call tests.
type MCPTestConfig struct {
	myToolId3NameAliceWant string
	mySecureToolWant       string
	mcpSelect1Want         string
	supportClientAuth      bool
	supportSelect1Auth     bool
}

type McpTestOption func(*MCPTestConfig)

// WithMcpMyToolId3NameAliceWant represents the response value for my-tool with id=3 and name=Alice.
// e.g. tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, tests.WithMcpMyToolId3NameAliceWant("custom"))
func WithMcpMyToolId3NameAliceWant(s string) McpTestOption {
	return func(c *MCPTestConfig) {
		c.myToolId3NameAliceWant = s
	}
}

// WithMcpMySecureToolWant represents the response body matching string for my-secure-tool.
// e.g. tests.RunMCPToolCallMethod(t, mcpMyFailToolWant, mcpSelect1Want, tests.WithMcpMySecureToolWant(`[{"ID":"1","NAME":"Alice"},{"ID":"3","NAME":"Sid"}]`))
func WithMcpMySecureToolWant(want string) McpTestOption {
	return func(c *MCPTestConfig) {
		c.mySecureToolWant = want
	}
}

// EnableMcpClientAuthTest runs the client authorization tests.
// Only enable it if your source supports the `useClientOAuth` configuration.
// Currently, this should only be used with the BigQuery tests.
func EnableMcpClientAuthTest() McpTestOption {
	return func(c *MCPTestConfig) {
		c.supportClientAuth = true
	}
}

// DisableMcpSelect1AuthTest disables the auth tool tests which use select 1.
func DisableMcpSelect1AuthTest() McpTestOption {
	return func(c *MCPTestConfig) {
		c.supportSelect1Auth = false
	}
}

func WithMcpSelect1Want(want string) McpTestOption {
	return func(c *MCPTestConfig) {
		c.mcpSelect1Want = want
	}
}

/* Configurations for RunExecuteSqlToolInvokeTest()  */

// ExecuteSqlTestConfig represents the various configuration options for RunExecuteSqlToolInvokeTest()
type ExecuteSqlTestConfig struct {
	select1Statement string
	createWant       string
	dropWant         string
	selectEmptyWant  string
	isMCP            bool
}

type ExecuteSqlOption func(*ExecuteSqlTestConfig)

// WithSelect1Statement represents the database's statement for `SELECT 1`.
// e.g. tests.RunExecuteSqlToolInvokeTest(t, createTableStatement, select1Want, tests.WithSelect1Statement("custom"))
func WithSelect1Statement(s string) ExecuteSqlOption {
	return func(c *ExecuteSqlTestConfig) {
		c.select1Statement = s
	}
}

// WithMCPSql enables the MCP routing for ExecuteSql tests
func WithMCPSql() ExecuteSqlOption {
	return func(c *ExecuteSqlTestConfig) {
		c.isMCP = true
	}
}

// WithExecuteCreateWant represents the expected response for a CREATE TABLE statement.
func WithExecuteCreateWant(s string) ExecuteSqlOption {
	return func(c *ExecuteSqlTestConfig) {
		c.createWant = s
	}
}

// WithExecuteDropWant represents the expected response for a DROP TABLE statement.
func WithExecuteDropWant(s string) ExecuteSqlOption {
	return func(c *ExecuteSqlTestConfig) {
		c.dropWant = s
	}
}

// WithExecuteSelectEmptyWant represents the expected response for a SELECT from an empty table.
func WithExecuteSelectEmptyWant(s string) ExecuteSqlOption {
	return func(c *ExecuteSqlTestConfig) {
		c.selectEmptyWant = s
	}
}

/* Configurations for RunToolInvokeWithTemplateParameters()  */

// TemplateParameterTestConfig represents the various configuration options for template parameter tests.
type TemplateParameterTestConfig struct {
	ddlWant         string
	selectAllWant   string
	selectId1Want   string
	selectNameWant  string
	selectEmptyWant string
	insert1Want     string

	nameFieldArray string
	nameColFilter  string
	createColArray string

	supportDdl          bool
	supportInsert       bool
	supportSelectFields bool
	isMCP               bool
}

type TemplateParamOption func(*TemplateParameterTestConfig)

// WithMCPTemplate flags the test harness to route the request through the local MCP server.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableName, tests.WithMCPTemplate())
func WithMCPTemplate() TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.isMCP = true
	}
}

// WithDdlWant represents the response value of ddl statements.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithDdlWant("custom"))
func WithDdlWant(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.ddlWant = s
	}
}

// WithSelectAllWant represents the response value of select-templateParams-tool.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithSelectAllWant("custom"))
func WithSelectAllWant(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.selectAllWant = s
	}
}

// WithTmplSelectId1Want represents the response value of select-templateParams-combined-tool with id=1.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithTmplSelectId1Want("custom"))
func WithTmplSelectId1Want(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.selectId1Want = s
	}
}

// WithTmplSelectNameWant represents the response value of select-filter-templateParams-combined-tool with name.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithTmplSelectNameWant("custom"))
func WithTmplSelectNameWant(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.selectNameWant = s
	}
}

// WithSelectEmptyWant represents the response value of select-templateParams-combined-tool with no results.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithSelectEmptyWant("custom"))
func WithSelectEmptyWant(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.selectEmptyWant = s
	}
}

// WithInsert1Want represents the response value of insert-table-templateParams-tool.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithInsert1Want("custom"))
func WithInsert1Want(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.insert1Want = s
	}
}

// WithNameFieldArray represents fields array parameter for select-fields-templateParams-tool.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithNameFieldArray("custom"))
func WithNameFieldArray(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.nameFieldArray = s
	}
}

// WithNameColFilter represents the columnFilter parameter for select-filter-templateParams-combined-tool.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithNameColFilter("custom"))
func WithNameColFilter(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.nameColFilter = s
	}
}

// WithCreateColArray represents the columns array parameter for create-table-templateParams-tool.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.WithCreateColArray("custom"))
func WithCreateColArray(s string) TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.createColArray = s
	}
}

// DisableDdlTest disables tests for ddl statements for sources that do not support ddl.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.DisableDdlTest())
func DisableDdlTest() TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.supportDdl = false
	}
}

// DisableInsertTest disables tests of insert statements for sources that do not support insert.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.DisableInsertTest())
func DisableInsertTest() TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.supportInsert = false
	}
}

// DisableInsertTest disables tests of select-fields-templateParams-tool test.
// e.g. tests.RunToolInvokeWithTemplateParameters(t, tableNameTemplateParam, tests.DisableSelectFilterTest())
func DisableSelectFilterTest() TemplateParamOption {
	return func(c *TemplateParameterTestConfig) {
		c.supportSelectFields = false
	}
}

/* Configurations for RunMySQL...Test()  */

// ToolExecConfig holds the configuration for executing prebuilt tool tests.
type ToolExecConfig struct {
	isMCP bool
}

// ToolExecOption is a functional option used to configure a ToolExecConfig.
type ToolExecOption func(*ToolExecConfig)

// WithMCPExec flags the test harness to route the request through the local MCP server
// instead of the Native Toolbox REST API.
func WithMCPExec() ToolExecOption {
	return func(c *ToolExecConfig) {
		c.isMCP = true
	}
}
