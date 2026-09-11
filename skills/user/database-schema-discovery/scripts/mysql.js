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



async function discoverMysql(connectionString) {
    const mysql = require('mysql2/promise');
    const conn = await mysql.createConnection(connectionString);
    const [rows] = await conn.execute(`
        SELECT TABLE_NAME as table_name, COLUMN_NAME as column_name, DATA_TYPE as data_type, 
               IS_NULLABLE as is_nullable, COLUMN_DEFAULT as column_default, COLUMN_KEY as column_key, EXTRA as extra
        FROM information_schema.columns 
        WHERE table_schema = database()
        ORDER BY TABLE_NAME, ORDINAL_POSITION
    `);
    await conn.end();

    const tables = {};
    for (const row of rows) {
        if (!tables[row.table_name]) tables[row.table_name] = [];
        tables[row.table_name].push({
            column_name: row.column_name,
            data_type: row.data_type,
            is_nullable: row.is_nullable === 'YES',
            column_default: row.column_default,
            is_primary_key: row.column_key === 'PRI'
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
        const tables = await discoverMysql(argv.connectionString);
        const toolsConfig = generateTools(tables, argv.sourceName, 'mysql');
        writeToolsYaml(toolsConfig);
        console.log(`Generated tools for ${Object.keys(tables).length} MySQL tables in tools.yaml`);
    } catch (err) {
        console.error('Discovery failed:', err.message);
        process.exitCode = 1;
    }
}

if (require.main === module) {
    main();
}

module.exports = { discoverMysql, main };
