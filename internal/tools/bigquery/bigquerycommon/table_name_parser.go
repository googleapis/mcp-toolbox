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

package bigquerycommon

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// parserState defines the state of the SQL parser's state machine.
type parserState int

const (
	stateNormal parserState = iota
	// String states
	stateInSingleQuoteString
	stateInDoubleQuoteString
	stateInTripleSingleQuoteString
	stateInTripleDoubleQuoteString
	stateInRawSingleQuoteString
	stateInRawDoubleQuoteString
	stateInRawTripleSingleQuoteString
	stateInRawTripleDoubleQuoteString
	// Comment states
	stateInSingleLineCommentDash
	stateInSingleLineCommentHash
	stateInMultiLineComment
)

// SQL statement verbs
const (
	verbCreate = "create"
	verbAlter  = "alter"
	verbDrop   = "drop"
	verbSelect = "select"
	verbInsert = "insert"
	verbUpdate = "update"
	verbDelete = "delete"
	verbMerge  = "merge"
)

var datasetLevelInformationSchemaViews = map[string]bool{
	"tables":                  true,
	"table_options":           true,
	"columns":                 true,
	"column_field_paths":      true,
	"views":                   true,
	"routines":                true,
	"routine_options":         true,
	"parameters":              true,
	"partitions":              true,
	"key_column_usage":        true,
	"table_constraints":       true,
	"check_constraints":       true,
	"referential_constraints": true,
	"search_indexes":          true,
	"vector_indexes":          true,
	"materialized_views":      true,
	"table_snapshots":         true,
}

// stmtBoundaryTokens are the tokens that can immediately precede the first
// keyword of a statement. A CALL in any of these positions is a procedure
// invocation, not a column or table named "call".
var stmtBoundaryTokens = map[string]bool{
	"": true, "begin": true, "then": true, "else": true, "do": true, "loop": true,
}

// createModifiers are tokens that may appear between CREATE and the object type,
// e.g. CREATE OR REPLACE TEMP TABLE FUNCTION f().
var createModifiers = map[string]bool{
	"or": true, "replace": true, "temp": true, "temporary": true,
	"if": true, "not": true, "exists": true, "table": true,
	"aggregate": true, "external": true, "materialized": true,
}

var tableFollowsKeywords = map[string]bool{
	"from":   true,
	"join":   true,
	"update": true,
	"into":   true, // INSERT INTO, MERGE INTO
	"table":  true, // CREATE TABLE, ALTER TABLE
	"model":  true, // ML.GET_INSIGHTS(MODEL ...)
	"view":   true, // DROP VIEW ...
	"using":  true, // MERGE ... USING
	"insert": true, // INSERT my_table
	"merge":  true, // MERGE my_table
}

var tableContextExitKeywords = map[string]bool{
	"where":     true,
	"group":     true, // GROUP BY
	"order":     true, // ORDER BY
	"having":    true,
	"limit":     true,
	"window":    true,
	"union":     true,
	"intersect": true,
	"except":    true,
	"on":        true, // JOIN ... ON
	"set":       true, // UPDATE ... SET
	"when":      true, // MERGE ... WHEN
}

var sqlStatementVerbs = map[string]bool{
	verbCreate: true,
	verbAlter:  true,
	verbDrop:   true,
	verbSelect: true,
	verbInsert: true,
	verbUpdate: true,
	verbDelete: true,
	verbMerge:  true,
}

var schemaOperationVerbs = map[string]bool{
	verbCreate: true,
	verbAlter:  true,
	verbDrop:   true,
}

var nonAliasKeywords = map[string]bool{
	// Join keywords & modifiers
	"inner": true, "outer": true, "left": true, "right": true, "full": true, "cross": true, "join": true, "natural": true,
	// Query clauses & operators
	"where": true, "group": true, "order": true, "having": true, "limit": true, "offset": true,
	"window": true, "qualify": true, "union": true, "intersect": true, "except": true,
	"on": true, "using": true, "set": true, "when": true, "then": true, "else": true, "end": true, "case": true,
	"for": true, "system_time": true, "of": true, "tablesample": true, "pivot": true, "unpivot": true,
	"unnest": true, "asc": true, "desc": true, "nulls": true, "first": true, "last": true,
	"all": true, "distinct": true, "by": true, "and": true, "or": true, "not": true, "in": true, "is": true,
	// Data types
	"int64": true, "int": true, "smallint": true, "integer": true, "bigint": true, "tinyint": true, "byteint": true,
	"numeric": true, "bignumeric": true, "decimal": true, "bigdecimal": true,
	"float64": true, "string": true, "bytes": true, "bool": true, "boolean": true,
	"date": true, "datetime": true, "time": true, "timestamp": true, "interval": true,
	"geography": true, "json": true, "array": true, "struct": true,
}

func isReservedNonTableToken(k string) bool {
	return tableContextExitKeywords[k] || tableFollowsKeywords[k] || nonAliasKeywords[k] || k == "select" || k == "with"
}

const maxParseDepth = 64

// hasPrefix checks if the runes starting at offset match the given prefix.
func hasPrefix(r []rune, offset int, prefix string) bool {
	if offset+len(prefix) > len(r) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if r[offset+i] != rune(prefix[i]) {
			return false
		}
	}
	return true
}

// hasPrefixFold checks if the runes starting at offset match the given prefix, ignoring case (ASCII only).
func hasPrefixFold(r []rune, offset int, prefix string) bool {
	if offset+len(prefix) > len(r) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		rChar := r[offset+i]
		pChar := rune(prefix[i])
		if rChar >= 'A' && rChar <= 'Z' {
			rChar += 32
		}
		if pChar >= 'A' && pChar <= 'Z' {
			pChar += 32
		}
		if rChar != pChar {
			return false
		}
	}
	return true
}

// ParseResult holds the outcome of a lexical scan of a query.
type ParseResult struct {
	// TableIDs are fully-qualified project.dataset.table references found in the query.
	TableIDs []string
	// UnqualifiedRefs are single-part table references (e.g. `FROM my_table`) that are
	// neither a CTE nor an alias. Their dataset can only be resolved at execution time
	// (via the session's default dataset), so they cannot be statically validated.
	UnqualifiedRefs []string
}

// TableParserDetailed parses a SQL query and returns both fully-qualified table IDs and
// single-part unqualified table references found in the query.
func TableParserDetailed(sql, defaultProjectID string) (ParseResult, error) {
	tableIDSet := make(map[string]struct{})
	unqualifiedCandidates := make(map[string]struct{})
	aliases := make(map[string]struct{})
	cteNames := make(map[string]struct{})
	runes := []rune(sql)

	if _, err := parseSQL(runes, defaultProjectID, tableIDSet, unqualifiedCandidates, aliases, cteNames, false, 0); err != nil {
		return ParseResult{}, err
	}

	tableIDs := make([]string, 0, len(tableIDSet))
	for id := range tableIDSet {
		tableIDs = append(tableIDs, id)
	}
	sort.Strings(tableIDs)

	unqualifiedRefs := make([]string, 0, len(unqualifiedCandidates))
	for ref := range unqualifiedCandidates {
		if _, ok := aliases[ref]; !ok {
			unqualifiedRefs = append(unqualifiedRefs, ref)
		}
	}
	sort.Strings(unqualifiedRefs)

	return ParseResult{
		TableIDs:        tableIDs,
		UnqualifiedRefs: unqualifiedRefs,
	}, nil
}

// TableParser parses a SQL query and returns a list of table IDs that it references.
// It is intended as a conservative fallback for when a dry run cannot be performed or analyzed.
func TableParser(sql, defaultProjectID string) ([]string, error) {
	res, err := TableParserDetailed(sql, defaultProjectID)
	if err != nil {
		return nil, err
	}
	return res.TableIDs, nil
}

// parseSQL is the core recursive function that processes SQL strings.
// It uses a state machine to find table names, unqualified references, and check statement safety.
func parseSQL(
	runes []rune,
	defaultProjectID string,
	tableIDSet map[string]struct{},
	unqualifiedCandidates map[string]struct{},
	aliases map[string]struct{},
	cteNames map[string]struct{},
	inSubquery bool,
	depth int,
) (int, error) {
	if depth > maxParseDepth {
		return 0, fmt.Errorf("query nesting is too deep to analyze safely (max %d levels)", maxParseDepth)
	}

	state := stateNormal
	expectingTable, expectingAlias, expectingCTE := false, false, false
	pendingCreate := false
	caseDepth := 0
	var lastTableKeyword, lastToken, statementVerb string

	for i := 0; i < len(runes); {
		char := runes[i]

		switch state {
		case stateNormal:
			if hasPrefix(runes, i, "--") {
				state = stateInSingleLineCommentDash
				i += 2
				continue
			}
			if char == '#' {
				state = stateInSingleLineCommentHash
				i++
				continue
			}
			if hasPrefix(runes, i, "/*") {
				state = stateInMultiLineComment
				i += 2
				continue
			}
			if char == ',' {
				if lastTableKeyword == "from" {
					expectingTable = true
					expectingAlias = false
				} else if statementVerb == "with" {
					expectingCTE = true
					expectingAlias = false
				}
				i++
				continue
			}
			if char == '(' {
				if expectingTable || expectingCTE || lastToken == "as" {
					consumed, err := parseSQL(runes[i+1:], defaultProjectID, tableIDSet, unqualifiedCandidates, aliases, cteNames, true, depth+1)
					if err != nil {
						return 0, err
					}
					i += consumed + 1
					if lastTableKeyword != "from" {
						expectingTable = false
					}
					expectingAlias = true
					expectingCTE = false
					continue
				}
			}
			if char == ')' {
				if inSubquery {
					return i + 1, nil
				}
			}
			if char == ';' {
				statementVerb = ""
				lastToken = ""
				lastTableKeyword = ""
				expectingTable = false
				expectingAlias = false
				expectingCTE = false
				pendingCreate = false
				caseDepth = 0
				i++
				continue
			}

			// Raw strings must be checked before regular strings.
			if hasPrefixFold(runes, i, "r'''") {
				state = stateInRawTripleSingleQuoteString
				i += 4
				continue
			}
			if hasPrefixFold(runes, i, `r"""`) {
				state = stateInRawTripleDoubleQuoteString
				i += 4
				continue
			}
			if hasPrefixFold(runes, i, "r'") {
				state = stateInRawSingleQuoteString
				i += 2
				continue
			}
			if hasPrefixFold(runes, i, `r"`) {
				state = stateInRawDoubleQuoteString
				i += 2
				continue
			}
			if hasPrefix(runes, i, "'''") {
				state = stateInTripleSingleQuoteString
				i += 3
				continue
			}
			if hasPrefix(runes, i, `"""`) {
				state = stateInTripleDoubleQuoteString
				i += 3
				continue
			}
			if char == '\'' {
				state = stateInSingleQuoteString
				i++
				continue
			}
			if char == '"' {
				state = stateInDoubleQuoteString
				i++
				continue
			}

			if unicode.IsLetter(char) || char == '`' || char == '_' {
				parts, consumed, err := parseIdentifierSequence(runes[i:])
				if err != nil {
					return 0, err
				}
				if consumed == 0 {
					i++
					continue
				}

				keyword := strings.ToLower(parts[0])
				fullID := strings.ToLower(strings.Join(parts, "."))

				// Check for EXTERNAL_QUERY
				for _, part := range parts {
					if strings.EqualFold(part, "EXTERNAL_QUERY") {
						return 0, fmt.Errorf("EXTERNAL_QUERY is not allowed when dataset restrictions are in place")
					}
				}

				// Check for INFORMATION_SCHEMA
				infoSchemaIdx := -1
				for idx, part := range parts {
					if strings.EqualFold(part, "INFORMATION_SCHEMA") {
						infoSchemaIdx = idx
						break
					}
				}
				if infoSchemaIdx != -1 {
					viewName := parts[len(parts)-1]
					if !datasetLevelInformationSchemaViews[strings.ToLower(viewName)] {
						return 0, fmt.Errorf("querying non-dataset-level INFORMATION_SCHEMA view %q is not allowed when dataset restrictions are in place", viewName)
					}
					if infoSchemaIdx == 0 {
						return 0, fmt.Errorf("querying INFORMATION_SCHEMA views without a dataset prefix is not allowed when dataset restrictions are in place")
					}
					if infoSchemaIdx > 2 {
						return 0, fmt.Errorf("invalid INFORMATION_SCHEMA query path %q", strings.Join(parts, "."))
					}
					parts = parts[:infoSchemaIdx+1]
					fullID = strings.ToLower(strings.Join(parts, "."))
				}

				// Security check for restricted statements
				if keyword == "immediate" && lastToken == "execute" {
					return 0, fmt.Errorf("EXECUTE IMMEDIATE is not allowed when dataset restrictions are in place")
				}
				if keyword == "create" {
					pendingCreate = true
				} else if pendingCreate {
					if keyword == "procedure" || keyword == "function" {
						kind := "CREATE " + strings.ToUpper(keyword)
						if keyword == "function" && lastToken == "table" {
							kind = "CREATE TABLE FUNCTION"
						}
						return 0, fmt.Errorf("unanalyzable statements like '%s' are not allowed", kind)
					}
					if !createModifiers[keyword] {
						pendingCreate = false
					}
				}

				if keyword == "case" {
					caseDepth++
				} else if keyword == "end" && caseDepth > 0 {
					caseDepth--
				}

				isBoundary := stmtBoundaryTokens[lastToken]
				if caseDepth > 0 && (lastToken == "then" || lastToken == "else") {
					isBoundary = false
				}
				if keyword == "call" && isBoundary {
					return 0, fmt.Errorf("CALL is not allowed when dataset restrictions are in place")
				}

				prev := lastToken
				if prev == "create or" || prev == "create or replace" {
					prev = verbCreate
				}
				if schemaOperationVerbs[prev] && (keyword == "schema" || keyword == "dataset") {
					return 0, fmt.Errorf("dataset-level operations like '%s %s' are not allowed", strings.ToUpper(prev), strings.ToUpper(keyword))
				}

				if len(parts) == 1 && keyword == "set" && (statementVerb == "" || statementVerb == "set") {
					nextIdx := i + consumed
					for nextIdx < len(runes) && (unicode.IsSpace(runes[nextIdx]) || runes[nextIdx] == '\n' || runes[nextIdx] == '\r' || runes[nextIdx] == '\t') {
						nextIdx++
					}
					if hasPrefix(runes, nextIdx, "@@") {
						return 0, fmt.Errorf("session variable assignment ('SET @@') is not allowed when dataset restrictions are in place")
					}
				}

				// Resolve aliases and identify table references.
				isKnownAlias := false
				if _, ok := aliases[fullID]; ok {
					isKnownAlias = true
				}
				if !isKnownAlias && len(parts) > 1 {
					// Only CTE names shadow multi-part references.
					for j := 1; j < len(parts); j++ {
						prefix := strings.ToLower(strings.Join(parts[:j], "."))
						if _, ok := cteNames[prefix]; ok {
							isKnownAlias = true
							break
						}
					}
				}

				if expectingCTE {
					cteNames[fullID] = struct{}{}
					aliases[fullID] = struct{}{}
					expectingCTE = false
				} else if expectingAlias {
					if len(parts) == 1 && (tableContextExitKeywords[keyword] || tableFollowsKeywords[keyword] ||
						nonAliasKeywords[keyword] || keyword == "select" || keyword == "with") {
						expectingAlias = false
					} else {
						aliases[fullID] = struct{}{}
						expectingAlias = false
						isKnownAlias = true
					}
				}

				// Re-check aliases after potential registration.
				if !isKnownAlias {
					if _, ok := aliases[fullID]; ok {
						isKnownAlias = true
					}
				}

				if expectingTable && !isKnownAlias {
					if len(parts) >= 2 {
						tableID, err := formatTableID(parts, defaultProjectID)
						if err != nil {
							return 0, err
						}
						if tableID != "" {
							// If it's a system function (AI.FORECAST, etc.), don't treat it as a table.
							isSystem := false
							p := strings.Split(tableID, ".")
							if len(p) == 3 && IsSystemResource(p[1], p[2]) {
								isSystem = true
							}
							if !isSystem {
								tableIDSet[tableID] = struct{}{}
							}
						}
					} else if len(parts) == 1 && !isReservedNonTableToken(keyword) {
						unqualifiedCandidates[strings.ToLower(parts[0])] = struct{}{}
					}
					// For most keywords, we expect only one table.
					if lastTableKeyword != "from" {
						expectingTable = false
					}
					expectingAlias = true
				}

				if len(parts) == 1 {
					if keyword == "with" {
						expectingCTE = true
						statementVerb = "with"
					} else if keyword == "as" {
						if statementVerb != "with" {
							expectingAlias = true
						}
						expectingTable = false
					} else if _, ok := tableFollowsKeywords[keyword]; ok {
						expectingTable = true
						lastTableKeyword = keyword
						expectingAlias = false
					} else if _, ok := tableContextExitKeywords[keyword]; ok {
						expectingTable = false
						lastTableKeyword = ""
						expectingAlias = false
					}
					if lastToken == "create" && keyword == "or" {
						lastToken = "create or"
					} else if lastToken == "create or" && keyword == "replace" {
						lastToken = "create or replace"
					} else {
						lastToken = keyword
					}
					// Also track statement verb for schema checks
					if sqlStatementVerbs[keyword] {
						if statementVerb == "" || statementVerb == "with" {
							statementVerb = keyword
						}
					}
				} else {
					lastToken = ""
				}
				i += consumed
				continue
			}
			i++
		case stateInSingleQuoteString:
			if char == '\\' {
				i += 2
				continue
			}
			if char == '\'' {
				state = stateNormal
			}
			i++
		case stateInDoubleQuoteString:
			if char == '\\' {
				i += 2
				continue
			}
			if char == '"' {
				state = stateNormal
			}
			i++
		case stateInTripleSingleQuoteString:
			if hasPrefix(runes, i, "'''") {
				state = stateNormal
				i += 3
			} else {
				i++
			}
		case stateInTripleDoubleQuoteString:
			if hasPrefix(runes, i, `"""`) {
				state = stateNormal
				i += 3
			} else {
				i++
			}
		case stateInSingleLineCommentDash, stateInSingleLineCommentHash:
			if char == '\n' {
				state = stateNormal
			}
			i++
		case stateInMultiLineComment:
			if hasPrefix(runes, i, "*/") {
				state = stateNormal
				i += 2
			} else {
				i++
			}
		case stateInRawSingleQuoteString:
			if char == '\'' {
				state = stateNormal
			}
			i++
		case stateInRawDoubleQuoteString:
			if char == '"' {
				state = stateNormal
			}
			i++
		case stateInRawTripleSingleQuoteString:
			if hasPrefix(runes, i, "'''") {
				state = stateNormal
				i += 3
			} else {
				i++
			}
		case stateInRawTripleDoubleQuoteString:
			if hasPrefix(runes, i, `"""`) {
				state = stateNormal
				i += 3
			} else {
				i++
			}
		}
	}
	if inSubquery {
		return 0, fmt.Errorf("unclosed subquery parenthesis")
	}
	return len(runes), nil
}

// IsAnyTableExplicitlyReferenced performs a lexical audit of the SQL to see if any of the target tables
// are explicitly named as identifiers. It correctly ignores names inside comments or strings.
func IsAnyTableExplicitlyReferenced(sql, defaultProjectID string, targetTableIDs []string) (bool, error) {
	type targetInfo struct {
		cleanTarget string
	}
	targets := make([]targetInfo, 0, len(targetTableIDs))
	for _, id := range targetTableIDs {
		lower := strings.ToLower(id)
		targets = append(targets, targetInfo{
			cleanTarget: strings.ReplaceAll(lower, "`", ""),
		})
	}
	cleanDefaultProjectID := strings.ReplaceAll(strings.ToLower(defaultProjectID), "`", "")

	runes := []rune(sql)
	state := stateNormal

	for i := 0; i < len(runes); {
		char := runes[i]

		switch state {
		case stateNormal:
			if hasPrefix(runes, i, "--") {
				state = stateInSingleLineCommentDash
				i += 2
				continue
			}
			if char == '#' {
				state = stateInSingleLineCommentHash
				i++
				continue
			}
			if hasPrefix(runes, i, "/*") {
				state = stateInMultiLineComment
				i += 2
				continue
			}

			// Handle various BigQuery string literal formats.
			if hasPrefixFold(runes, i, "r'''") {
				state = stateInRawTripleSingleQuoteString
				i += 4
				continue
			}
			if hasPrefixFold(runes, i, `r"""`) {
				state = stateInRawTripleDoubleQuoteString
				i += 4
				continue
			}
			if hasPrefixFold(runes, i, "r'") {
				state = stateInRawSingleQuoteString
				i += 2
				continue
			}
			if hasPrefixFold(runes, i, `r"`) {
				state = stateInRawDoubleQuoteString
				i += 2
				continue
			}
			if hasPrefix(runes, i, "'''") {
				state = stateInTripleSingleQuoteString
				i += 3
				continue
			}
			if hasPrefix(runes, i, `"""`) {
				state = stateInTripleDoubleQuoteString
				i += 3
				continue
			}
			if char == '\'' {
				state = stateInSingleQuoteString
				i++
				continue
			}
			if char == '"' {
				state = stateInDoubleQuoteString
				i++
				continue
			}

			if unicode.IsLetter(char) || char == '`' || char == '_' {
				parts, consumed, err := parseIdentifierSequence(runes[i:])
				if err != nil {
					return false, err
				}
				if consumed > 0 {
					fullID := strings.ToLower(strings.Join(parts, "."))
					for _, target := range targets {
						cleanTarget := target.cleanTarget
						// Exact match or as a prefix for column references.
						if fullID == cleanTarget || strings.HasPrefix(fullID, cleanTarget+".") {
							return true, nil
						}
						// Try matching with the default project ID prefix.
						if cleanDefaultProjectID != "" {
							withDefault := cleanDefaultProjectID + "." + fullID
							if withDefault == cleanTarget || strings.HasPrefix(withDefault, cleanTarget+".") {
								return true, nil
							}
						}
					}
					i += consumed
					continue
				}
			}

		case stateInSingleQuoteString:
			if char == '\\' {
				i += 2
				continue
			}
			if char == '\'' {
				state = stateNormal
			}
		case stateInDoubleQuoteString:
			if char == '\\' {
				i += 2
				continue
			}
			if char == '"' {
				state = stateNormal
			}
		case stateInTripleSingleQuoteString:
			if hasPrefix(runes, i, "'''") {
				state = stateNormal
				i += 3
				continue
			}
		case stateInTripleDoubleQuoteString:
			if hasPrefix(runes, i, `"""`) {
				state = stateNormal
				i += 3
				continue
			}
		case stateInSingleLineCommentDash, stateInSingleLineCommentHash:
			if char == '\n' {
				state = stateNormal
			}
		case stateInMultiLineComment:
			if hasPrefix(runes, i, "*/") {
				state = stateNormal
				i += 2
				continue
			}
		case stateInRawSingleQuoteString:
			if char == '\'' {
				state = stateNormal
			}
		case stateInRawDoubleQuoteString:
			if char == '"' {
				state = stateNormal
			}
		case stateInRawTripleSingleQuoteString:
			if hasPrefix(runes, i, "'''") {
				state = stateNormal
				i += 3
				continue
			}
		case stateInRawTripleDoubleQuoteString:
			if hasPrefix(runes, i, `"""`) {
				state = stateNormal
				i += 3
				continue
			}
		}
		i++
	}

	return false, nil
}

// parseIdentifierSequence parses a sequence of dot-separated identifiers.
// It returns the parts of the identifier, the number of characters consumed, and an error.
func parseIdentifierSequence(runes []rune) ([]string, int, error) {
	var parts []string
	var totalConsumed int
	for {
		// Skip whitespace and comments before identifier part
		for {
			originalConsumed := totalConsumed
			for totalConsumed < len(runes) && unicode.IsSpace(runes[totalConsumed]) {
				totalConsumed++
			}
			if hasPrefix(runes, totalConsumed, "/*") {
				endIdx := indexRunes(runes[totalConsumed:], "*/")
				if endIdx != -1 {
					totalConsumed += endIdx + 2
				}
			} else if hasPrefix(runes, totalConsumed, "--") || (totalConsumed < len(runes) && runes[totalConsumed] == '#') {
				endIdx := indexRunes(runes[totalConsumed:], "\n")
				if endIdx != -1 {
					totalConsumed += endIdx + 1
				} else {
					totalConsumed = len(runes)
				}
			}
			if totalConsumed == originalConsumed {
				break
			}
		}
		if totalConsumed >= len(runes) {
			break
		}

		var part string
		var consumed int

		if runes[totalConsumed] == '`' {
			end := indexRunes(runes[totalConsumed+1:], "`")
			if end == -1 {
				return nil, 0, fmt.Errorf("unclosed backtick identifier")
			}
			part = string(runes[totalConsumed+1 : totalConsumed+end+1])
			consumed = end + 2
		} else if unicode.IsLetter(runes[totalConsumed]) || runes[totalConsumed] == '_' {
			end := totalConsumed
			for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsNumber(runes[end]) || runes[end] == '_' || runes[end] == '-') {
				end++
			}
			part = string(runes[totalConsumed:end])
			consumed = end - totalConsumed
		} else {
			break
		}

		parts = append(parts, strings.Split(part, ".")...)
		totalConsumed += consumed

		// Skip whitespace and comments between parts (before potential dot)
		for {
			originalConsumed := totalConsumed
			for totalConsumed < len(runes) && unicode.IsSpace(runes[totalConsumed]) {
				totalConsumed++
			}
			if hasPrefix(runes, totalConsumed, "/*") {
				endIdx := indexRunes(runes[totalConsumed:], "*/")
				if endIdx != -1 {
					totalConsumed += endIdx + 2
				}
			} else if hasPrefix(runes, totalConsumed, "--") || (totalConsumed < len(runes) && runes[totalConsumed] == '#') {
				endIdx := indexRunes(runes[totalConsumed:], "\n")
				if endIdx != -1 {
					totalConsumed += endIdx + 1
				} else {
					totalConsumed = len(runes)
				}
			}
			if totalConsumed == originalConsumed {
				break
			}
		}

		if totalConsumed >= len(runes) || runes[totalConsumed] != '.' {
			break
		}
		totalConsumed++
	}

	return parts, totalConsumed, nil
}

func formatTableID(parts []string, defaultProjectID string) (string, error) {
	if len(parts) == 4 && strings.Contains(parts[1], ":") {
		parts = []string{parts[0] + "." + parts[1], parts[2], parts[3]}
	}
	if len(parts) < 2 || len(parts) > 3 {
		// Not a table identifier (could be a CTE, column, etc.).
		return "", nil
	}

	if len(parts) == 3 { // project.dataset.table
		return strings.Join(parts, "."), nil
	}

	// dataset.table
	if defaultProjectID == "" {
		return "", fmt.Errorf("query contains table '%s' without project ID, and no default project ID is provided", strings.Join(parts, "."))
	}
	return fmt.Sprintf("%s.%s", defaultProjectID, strings.Join(parts, ".")), nil
}

func indexRunes(r []rune, sub string) int {
	subRunes := []rune(sub)
	if len(subRunes) == 0 {
		return 0
	}
	for i := 0; i <= len(r)-len(subRunes); i++ {
		match := true
		for j := 0; j < len(subRunes); j++ {
			if r[i+j] != subRunes[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
