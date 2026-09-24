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



async function discoverPostgres(connectionString) {
    const { Client } = require('pg');
    const client = new Client({ connectionString });
    await client.connect();
    
    const res = await client.query(`
        SELECT c.table_name, c.column_name, c.data_type, c.is_nullable, c.column_default,
        EXISTS (
            SELECT 1 FROM information_schema.key_column_usage kcu
            JOIN information_schema.table_constraints tc ON kcu.constraint_name = tc.constraint_name
            WHERE tc.constraint_type = 'PRIMARY KEY' 
              AND kcu.table_name = c.table_name 
              AND kcu.column_name = c.column_name
              AND kcu.table_schema = c.table_schema
        ) as is_primary_key
        FROM information_schema.columns c
        WHERE c.table_schema = 'public'
        ORDER BY c.table_name, c.ordinal_position
    `);
    
    await client.end();
    
    const tables = {};
    for (const row of res.rows) {
        if (!tables[row.table_name]) tables[row.table_name] = [];
        tables[row.table_name].push({
            column_name: row.column_name,
            data_type: row.data_type,
            is_nullable: row.is_nullable === 'YES',
            column_default: row.column_default,
            is_primary_key: row.is_primary_key
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
        const tables = await discoverPostgres(argv.connectionString);
        const toolsConfig = generateTools(tables, argv.sourceName, 'postgres');
        writeToolsYaml(toolsConfig);
        console.log(`Generated tools for ${Object.keys(tables).length} Postgres tables in tools.yaml`);
    } catch (err) {
        console.error('Discovery failed:', err.message);
        process.exitCode = 1;
    }
}

if (require.main === module) {
    main();
}

module.exports = { discoverPostgres, main };
