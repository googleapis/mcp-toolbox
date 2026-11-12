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



async function discoverSqlite(dbPath) {
    const sqlite3 = require('sqlite3').verbose();
    return new Promise((resolve, reject) => {
        const db = new sqlite3.Database(dbPath, sqlite3.OPEN_READONLY, (err) => {
            if (err) return reject(err);
        });

        db.all("SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'", (err, tableRows) => {
            if (err) {
                db.close();
                return reject(err);
            }

            const tables = {};
            let pending = tableRows.length;
            if (pending === 0) {
                db.close();
                return resolve(tables);
            }

            for (const tRow of tableRows) {
                const tableName = tRow.name;
                tables[tableName] = [];
                db.all(`PRAGMA table_info("${tableName}")`, (err, cols) => {
                    if (err) {
                        db.close();
                        return reject(err);
                    }
                    for (const c of cols) {
                        tables[tableName].push({
                            column_name: c.name,
                            data_type: c.type,
                            is_nullable: c.notnull === 0,
                            column_default: c.dflt_value,
                            is_primary_key: c.pk > 0
                        });
                    }
                    pending--;
                    if (pending === 0) {
                        db.close();
                        resolve(tables);
                    }
                });
            }
        });
    });
}

async function main(args = process.argv) {
    const argv = yargs(hideBin(args))
      .option('source-name', { type: 'string', demandOption: true, description: 'Name of the source in the tools.yaml' })
      .option('db-path', { type: 'string', demandOption: true, description: 'File path to SQLite database' })
      .parse();

    try {
        const tables = await discoverSqlite(argv.dbPath);
        const toolsConfig = generateTools(tables, argv.sourceName, 'sqlite');
        writeToolsYaml(toolsConfig);
        console.log(`Generated tools for ${Object.keys(tables).length} SQLite tables in tools.yaml`);
    } catch (err) {
        console.error('Discovery failed:', err.message);
        process.exitCode = 1;
    }
}

if (require.main === module) {
    main();
}

module.exports = { discoverSqlite, main };
