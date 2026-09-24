#!/usr/bin/env node
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

const yargs = require('yargs/yargs');
const { hideBin } = require('yargs/helpers');
const { generateTools, writeToolsYaml } = require('../lib/generator');



async function discoverMssql(connectionString) {
    const sql = require('mssql');
    await sql.connect(connectionString);
    const res = await sql.query(`
        SELECT t.name AS table_name, c.name AS column_name, type_name(c.user_type_id) AS data_type,
               c.is_nullable, OBJECT_DEFINITION(c.default_object_id) AS column_default,
               ISNULL((SELECT 1 FROM sys.index_columns ic JOIN sys.indexes i ON ic.object_id = i.object_id AND ic.index_id = i.index_id
                       WHERE i.is_primary_key = 1 AND ic.object_id = c.object_id AND ic.column_id = c.column_id), 0) AS is_primary_key
        FROM sys.tables t
        JOIN sys.columns c ON t.object_id = c.object_id
        ORDER BY t.name, c.column_id
    `);
    await sql.close();

    const tables = {};
    for (const row of res.recordset) {
        if (!tables[row.table_name]) tables[row.table_name] = [];
        tables[row.table_name].push({
            column_name: row.column_name,
            data_type: row.data_type,
            is_nullable: row.is_nullable,
            column_default: row.column_default,
            is_primary_key: !!row.is_primary_key
        });
    }
    return tables;
}

async function main(args = process.argv) {
    const argv = yargs(hideBin(args))
      .option('source-name', { type: 'string', demandOption: true, description: 'Name of the source in the tools.yaml' })
      .option('connection-string', { type: 'string', demandOption: true, description: 'Database connection string' })
      .parse();

    try {
        const tables = await discoverMssql(argv.connectionString);
        const toolsConfig = generateTools(tables, argv.sourceName, 'mssql');
        writeToolsYaml(toolsConfig);
        console.log(`Generated tools for ${Object.keys(tables).length} MSSQL tables in tools.yaml`);
    } catch (err) {
        console.error('Discovery failed:', err.message);
        process.exitCode = 1;
    }
}

if (require.main === module) {
    main();
}

module.exports = { discoverMssql, main };
