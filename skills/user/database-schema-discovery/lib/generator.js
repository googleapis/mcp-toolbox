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

const fs = require('fs');
const yaml = require('yaml');

function mapDataType(dataType) {
    const type = (dataType || '').toLowerCase();
    if (type.includes('int') || type.includes('serial')) return 'integer';
    if (type.includes('numeric') || type.includes('decimal') || type.includes('real') || type.includes('float') || type.includes('double')) return 'float';
    if (type.includes('bool') || type.includes('bit')) return 'boolean';
    return 'string';
}

function sanitizeName(name) {
    return name.replace(/[^a-zA-Z0-9_]/g, '_').toLowerCase();
}

function quoteIdent(name, sourceType) {
    if (sourceType === 'mysql') return `\`${name}\``;
    if (sourceType === 'mssql') return `[${name}]`;
    return `"${name}"`; // postgres, sqlite
}

function generateTools(tables, sourceName, sourceType) {
    const tools = {};
    const toolType = `${sourceType}-sql`;

    for (const [tableNameRaw, columns] of Object.entries(tables)) {
        const pkColumns = columns.filter(c => c.is_primary_key);
        const pk = pkColumns.length === 1 ? pkColumns[0] : null;
        
        const tableName = quoteIdent(tableNameRaw, sourceType);
        const safeTableName = sanitizeName(tableNameRaw);

        // 1. List tool
        tools[`list_${safeTableName}`] = {
            type: toolType,
            source: sourceName,
            description: `Retrieve a list of records from ${tableNameRaw}`,
            statement: `SELECT * FROM ${tableName} LIMIT $1 OFFSET $2`,
            parameters: [
                { name: 'limit', type: 'integer', description: 'Maximum number of records to return', required: false, default: 100 },
                { name: 'offset', type: 'integer', description: 'Number of records to skip', required: false, default: 0 }
            ]
        };

        if (pk) {
            const pkName = quoteIdent(pk.column_name, sourceType);
            
            // 2. Get tool
            tools[`get_${safeTableName}_by_id`] = {
                type: toolType,
                source: sourceName,
                description: `Retrieve a single record from ${tableNameRaw} by its primary key`,
                statement: `SELECT * FROM ${tableName} WHERE ${pkName} = $1`,
                parameters: [{ name: pk.column_name, type: mapDataType(pk.data_type), description: `Primary key ${pk.column_name}`, required: true }]
            };

            // 3. Update tool
            const updateCols = columns.filter(c => !c.is_primary_key);
            if (updateCols.length > 0) {
                const setClauses = updateCols.map((c, i) => `${quoteIdent(c.column_name, sourceType)} = COALESCE($${i + 1}, ${quoteIdent(c.column_name, sourceType)})`);
                const updateParams = updateCols.map((c, i) => ({
                    name: c.column_name,
                    type: mapDataType(c.data_type),
                    description: `Field ${c.column_name}`,
                    required: false
                }));
                // Add PK to the end
                updateParams.push({ name: pk.column_name, type: mapDataType(pk.data_type), description: `Primary key ${pk.column_name}`, required: true });

                tools[`update_${safeTableName}_by_id`] = {
                    type: toolType,
                    source: sourceName,
                    description: `Update an existing record in ${tableNameRaw} by its primary key`,
                    statement: `UPDATE ${tableName} SET ${setClauses.join(', ')} WHERE ${pkName} = $${updateCols.length + 1}`,
                    parameters: updateParams
                };
            }

            // 4. Delete tool
            tools[`delete_${safeTableName}_by_id`] = {
                type: toolType,
                source: sourceName,
                description: `Delete a record from ${tableNameRaw} by its primary key`,
                statement: `DELETE FROM ${tableName} WHERE ${pkName} = $1`,
                parameters: [{ name: pk.column_name, type: mapDataType(pk.data_type), description: `Primary key ${pk.column_name}`, required: true }]
            };
        }

        // 5. Insert tool
        const insertCols = columns.filter(c => {
            const dt = (c.data_type || '').toLowerCase();
            const def = (c.column_default || '').toLowerCase();
            const isSinglePk = pkColumns.length === 1 && c.is_primary_key;
            return !(isSinglePk && (dt.includes('serial') || dt.includes('autoincrement') || def.includes('nextval') || dt === 'integer'));
        });

        if (insertCols.length > 0) {
            const placeholders = insertCols.map((_, i) => `$${i + 1}`);
            const insertParams = insertCols.map(c => ({
                name: c.column_name,
                type: mapDataType(c.data_type),
                description: `Field ${c.column_name}`,
                required: (!c.is_nullable && !c.column_default)
            }));

            tools[`insert_${safeTableName}`] = {
                type: toolType,
                source: sourceName,
                description: `Insert a new record into ${tableNameRaw}`,
                statement: `INSERT INTO ${tableName} (${insertCols.map(c => quoteIdent(c.column_name, sourceType)).join(', ')}) VALUES (${placeholders.join(', ')})`,
                parameters: insertParams
            };
        }
    }

    return { tools };
}

function writeToolsYaml(toolsConfig) {
    let existingConfig = {};
    if (fs.existsSync('tools.yaml')) {
        existingConfig = yaml.parse(fs.readFileSync('tools.yaml', 'utf8')) || {};
    }
    existingConfig.tools = { ...existingConfig.tools, ...toolsConfig.tools };
    fs.writeFileSync('tools.yaml', yaml.stringify(existingConfig));
}

module.exports = {
    generateTools,
    writeToolsYaml,
    quoteIdent,
    sanitizeName
};
