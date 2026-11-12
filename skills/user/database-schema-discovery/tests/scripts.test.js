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

const { discoverPostgres } = require('../scripts/postgres');
const { discoverMysql } = require('../scripts/mysql');
const { discoverMssql } = require('../scripts/mssql');
const { discoverSqlite } = require('../scripts/sqlite');

jest.mock('yargs/yargs', () => () => ({
    option: jest.fn().mockReturnThis(),
    parse: jest.fn().mockReturnValue({})
}));
jest.mock('yargs/helpers', () => ({
    hideBin: jest.fn()
}));

jest.mock('pg', () => {
    return {
        Client: jest.fn().mockImplementation(() => {
            return {
                connect: jest.fn().mockResolvedValue(),
                end: jest.fn().mockResolvedValue(),
                query: jest.fn().mockResolvedValue({
                    rows: [
                        { table_name: 'users', column_name: 'id', data_type: 'integer', is_nullable: 'NO', is_primary_key: true }
                    ]
                })
            };
        })
    };
});

jest.mock('mysql2/promise', () => {
    return {
        createConnection: jest.fn().mockResolvedValue({
            end: jest.fn().mockResolvedValue(),
            execute: jest.fn().mockResolvedValue([
                [
                    { table_name: 'users', column_name: 'id', data_type: 'int', is_nullable: 'NO', column_key: 'PRI' }
                ]
            ])
        })
    };
});

jest.mock('mssql', () => {
    return {
        connect: jest.fn().mockResolvedValue(),
        close: jest.fn().mockResolvedValue(),
        query: jest.fn().mockResolvedValue({
            recordset: [
                { table_name: 'users', column_name: 'id', data_type: 'int', is_nullable: false, is_primary_key: true }
            ]
        })
    };
});

jest.mock('sqlite3', () => {
    return {
        verbose: () => ({
            OPEN_READONLY: 1,
            Database: jest.fn().mockImplementation((path, mode, cb) => {
                cb(null);
                return {
                    all: jest.fn((query, callback) => {
                        if (query.includes("sqlite_master")) {
                            callback(null, [{ name: 'users' }]);
                        } else {
                            callback(null, [{ name: 'id', type: 'INTEGER', notnull: 1, pk: 1 }]);
                        }
                    }),
                    close: jest.fn()
                };
            })
        })
    };
});

describe('Discovery Scripts', () => {
    it('discoverPostgres should return parsed tables', async () => {
        const tables = await discoverPostgres('postgres://localhost');
        expect(tables.users).toBeDefined();
        expect(tables.users[0].column_name).toBe('id');
        expect(tables.users[0].is_primary_key).toBe(true);
    });

    it('discoverMysql should return parsed tables', async () => {
        const tables = await discoverMysql('mysql://localhost');
        expect(tables.users).toBeDefined();
        expect(tables.users[0].column_name).toBe('id');
        expect(tables.users[0].is_primary_key).toBe(true);
    });

    it('discoverMssql should return parsed tables', async () => {
        const tables = await discoverMssql('mssql://localhost');
        expect(tables.users).toBeDefined();
        expect(tables.users[0].column_name).toBe('id');
        expect(tables.users[0].is_primary_key).toBe(true);
    });

    it('discoverSqlite should return parsed tables', async () => {
        const tables = await discoverSqlite('test.db');
        expect(tables.users).toBeDefined();
        expect(tables.users[0].column_name).toBe('id');
        expect(tables.users[0].is_primary_key).toBe(true);
    });
});
