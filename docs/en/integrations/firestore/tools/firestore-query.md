---
title: "firestore-query"
type: docs
weight: 1
description: >
  Query a Firestore collection with parameterizable filters and Firestore native JSON value types
---

## About

The `firestore-query` tool allows you to query Firestore collections with
dynamic, parameterizable filters that support Firestore's native JSON value
types. This tool is designed for querying single collection, which is the
standard pattern in Firestore. The collection path itself can be parameterized,
making it flexible for various use cases. This tool is particularly useful when
you need to create reusable query templates with parameters that can be
substituted at runtime.

### Key Features

- **Parameterizable Queries**: Use Go template syntax to create dynamic queries
- **Dynamic Collection Paths**: The collection path can be parameterized for
  flexibility
- **Native JSON Value Types**: Support for Firestore's typed values
  (stringValue, integerValue, doubleValue, etc.)
- **Complex Filter Logic**: Support for AND/OR logical operators in filters
- **Template Substitution**: Dynamic collection paths, filters, and ordering
- **Query Analysis**: Optional query performance analysis with explain metrics
  (non-parameterizable)

**Developer Note**: This tool serves as the general querying foundation that
developers can use to create custom tools with specific query patterns.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

### Configuration Parameters

| Parameter        | Type    | Required | Description                                                                                                 |
|------------------|---------|----------|-------------------------------------------------------------------------------------------------------------|
| `type`           | string  | Yes      | Must be `firestore-query`                                                                                   |
| `source`         | string  | Yes      | Name of the Firestore source to use                                                                         |
| `description`    | string  | Yes      | Description of what this tool does                                                                          |
| `collectionPath` | string  | Yes      | Path to the collection to query (supports templates)                                                        |
| `filters`        | string  | No       | JSON string defining query filters (supports templates)                                                     |
| `select`         | array   | No       | Fields to select from documents(supports templates - string or array)                                       |
| `orderBy`        | object  | No       | Ordering configuration with `field` and `direction`(supports templates for the value of field or direction) |
| `limit`          | integer | No       | Maximum number of documents to return (default: 100) (supports templates)                                   |
| `analyzeQuery`   | boolean | No       | Whether to analyze query performance (default: false)                                                       |
| `parameters`     | array   | Yes      | Parameter definitions for template substitution                                                             |

### Runtime Parameters

Runtime parameters are defined in the `parameters` array and can be used in
templates throughout the configuration.

### Filter Format

#### Simple Filter

```json
{
  "field": "age",
  "op": ">",
  "value": {"integerValue": "25"}
}
```

#### AND Filter

```json
{
  "and": [
    {"field": "status", "op": "==", "value": {"stringValue": "active"}},
    {"field": "age", "op": ">=", "value": {"integerValue": "18"}}
  ]
}
```

#### OR Filter

```json
{
  "or": [
    {"field": "role", "op": "==", "value": {"stringValue": "admin"}},
    {"field": "role", "op": "==", "value": {"stringValue": "moderator"}}
  ]
}
```

#### Nested Filters

```json
{
  "or": [
    {"field": "type", "op": "==", "value": {"stringValue": "premium"}},
    {
      "and": [
        {"field": "type", "op": "==", "value": {"stringValue": "standard"}},
        {"field": "credits", "op": ">", "value": {"integerValue": "1000"}}
      ]
    }
  ]
}
```

## Example

### Basic Configuration

```yaml
kind: tool
name: query_countries
type: firestore-query
source: my-firestore-source
description: Query countries with dynamic filters
collectionPath: "countries"
filters: |
  {
    "field": "continent",
    "op": "==",
    "value": {"stringValue": {{json .continent}}}
  }
parameters:
  - name: continent
    type: string
    description: Continent to filter by
    required: true
```

### Advanced Configuration with Complex Filters

```yaml
kind: tool
name: advanced_query
type: firestore-query
source: my-firestore-source
description: Advanced query with complex filters
collectionPath: "{{.collection}}"
filters: |
  {
    "or": [
      {"field": "status", "op": "==", "value": {"stringValue": {{json .status}}}},
      {
        "and": [
          {"field": "priority", "op": ">", "value": {"integerValue": {{json .priority}}}},
          {"field": "area", "op": "<", "value": {"doubleValue": {{.maxArea}}}},
          {"field": "active", "op": "==", "value": {"booleanValue": {{.isActive}}}}
        ]
      }
    ]
  }
select:
  - name
  - status
  - priority
orderBy:
  field: "{{.sortField}}"
  direction: "{{.sortDirection}}"
limit: 100
analyzeQuery: true
parameters:
  - name: collection
    type: string
    description: Collection to query
    required: true
  - name: status
    type: string
    description: Status to filter by
    required: true
  - name: priority
    type: string
    description: Minimum priority value
    required: true
  - name: maxArea
    type: float
    description: Maximum area value
    required: true
  - name: isActive
    type: boolean
    description: Filter by active status
    required: true
  - name: sortField
    type: string
    description: Field to sort by
    required: false
    default: "createdAt"
  - name: sortDirection
    type: string
    description: Sort direction (ASCENDING or DESCENDING)
    required: false
    default: "DESCENDING"
```

### Firestore Native Value Types

The tool supports all Firestore native JSON value types:

| Type      | Format                                               | Example                                                          |
|-----------|------------------------------------------------------|------------------------------------------------------------------|
| String    | `{"stringValue": "text"}`                            | `{"stringValue": {{json .name}}}`                                |
| Integer   | `{"integerValue": "123"}` or `{"integerValue": 123}` | `{"integerValue": {{json .age}}}` or `{"integerValue": {{.age}}}` |
| Double    | `{"doubleValue": 45.67}`                             | `{"doubleValue": {{.price}}}`                                    |
| Boolean   | `{"booleanValue": true}`                             | `{"booleanValue": {{.active}}}`                                  |
| Null      | `{"nullValue": null}`                                | `{"nullValue": null}`                                            |
| Timestamp | `{"timestampValue": "RFC3339"}`                      | `{"timestampValue": {{json .date}}}`                             |
| GeoPoint  | `{"geoPointValue": {"latitude": 0, "longitude": 0}}` | See below                                                        |
| Array     | `{"arrayValue": {"values": [...]}}`                  | See below                                                        |
| Map       | `{"mapValue": {"fields": {...}}}`                    | See below                                                        |

#### Complex Type Examples

**GeoPoint:**

```json
{
  "field": "location",
  "op": "==",
  "value": {
    "geoPointValue": {
      "latitude": 37.7749,
      "longitude": -122.4194
    }
  }
}
```

**Array:**

```json
{
  "field": "tags",
  "op": "array-contains",
  "value": {"stringValue": {{json .tag}}}
}
```

### Supported Operators

- `<` - Less than
- `<=` - Less than or equal
- `>` - Greater than
- `>=` - Greater than or equal
- `==` - Equal
- `!=` - Not equal
- `array-contains` - Array contains value
- `array-contains-any` - Array contains any of the values
- `in` - Value is in array
- `not-in` - Value is not in array

### Example 1: Query with Dynamic Collection Path

```yaml
kind: tool
name: user_documents
type: firestore-query
source: my-firestore
description: Query user-specific documents
collectionPath: "users/{{.userId}}/documents"
filters: |
  {
    "field": "type",
    "op": "==",
    "value": {"stringValue": {{json .docType}}}
  }
parameters:
  - name: userId
    type: string
    description: User ID
    required: true
  - name: docType
    type: string
    description: Document type to filter
    required: true
```

### Example 2: Complex Geographic Query

```yaml
kind: tool
name: location_search
type: firestore-query
source: my-firestore
description: Search locations by area and population
collectionPath: "cities"
filters: |
  {
    "and": [
      {"field": "country", "op": "==", "value": {"stringValue": {{json .country}}}},
      {"field": "population", "op": ">", "value": {"integerValue": {{json .minPopulation}}}},
      {"field": "area", "op": "<", "value": {"doubleValue": {{.maxArea}}}}
    ]
  }
orderBy:
  field: "population"
  direction: "DESCENDING"
limit: 50
parameters:
  - name: country
    type: string
    description: Country code
    required: true
  - name: minPopulation
    type: string
    description: Minimum population (as string for large numbers)
    required: true
  - name: maxArea
    type: float
    description: Maximum area in square kilometers
    required: true
```

### Example 3: Time-based Query with Analysis

```yaml
kind: tool
name: activity_log
type: firestore-query
source: my-firestore
description: Query activity logs within time range
collectionPath: "logs"
filters: |
  {
    "and": [
      {"field": "timestamp", "op": ">=", "value": {"timestampValue": {{json .startTime}}}},
      {"field": "timestamp", "op": "<=", "value": {"timestampValue": {{json .endTime}}}},
      {"field": "severity", "op": "in", "value": {"arrayValue": {"values": [
        {"stringValue": "ERROR"},
        {"stringValue": "CRITICAL"}
      ]}}}
    ]
  }
select:
  - timestamp
  - message
  - severity
  - userId
orderBy:
  field: "timestamp"
  direction: "DESCENDING"
analyzeQuery: true
parameters:
  - name: startTime
    type: string
    description: Start time in RFC3339 format
    required: true
  - name: endTime
    type: string
    description: End time in RFC3339 format
    required: true
```

## Advanced Usage

### Invoking the Tool

```bash
# Using curl
curl -X POST http://localhost:5000/api/tool/your-tool-name/invoke \
  -H "Content-Type: application/json" \
  -d '{
    "continent": "Europe",
    "minPopulation": "1000000",
    "maxArea": 500000.5,
    "isActive": true
  }'
```

### Response Format

**Without analyzeQuery:**

```json
[
  {
    "id": "doc1",
    "path": "countries/doc1",
    "data": {
      "name": "France",
      "continent": "Europe",
      "population": 67000000,
      "area": 551695
    },
    "createTime": "2024-01-01T00:00:00Z",
    "updateTime": "2024-01-15T10:30:00Z"
  }
]
```

**With analyzeQuery:**

```json
{
  "documents": [...],
  "explainMetrics": {
    "planSummary": {
      "indexesUsed": [...]
    },
    "executionStats": {
      "resultsReturned": 10,
      "executionDuration": "15ms",
      "readOperations": 10
    }
  }
}
```

### Best Practices

1. **Use Typed Values**: Always use Firestore's native JSON value types for
   proper type handling
2. **String Numbers for Large Integers**: Use string representation for large
   integers to avoid precision loss
3. **Template Security**: Validate all template parameters to prevent injection
   attacks
4. **Index Optimization**: Use `analyzeQuery` to identify missing indexes
5. **Limit Results**: Always set a reasonable `limit` to prevent excessive data
   retrieval
6. **Field Selection**: Use `select` to retrieve only necessary fields

### Technical Notes

- Queries operate on a single collection (the standard Firestore pattern)
- Maximum of 100 filters per query (configurable)
- Template parameters must be properly escaped in JSON contexts
- Complex nested queries may require composite indexes

## Additional Resources

- [firestore-query-collection](firestore-query-collection.md) -
  Non-parameterizable query tool
- [Firestore Source Configuration](../source.md)
- [Firestore Query
  Documentation](https://firebase.google.com/docs/firestore/query-data/queries)
