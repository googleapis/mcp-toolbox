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

const { generateTools, quoteIdent, sanitizeName } = require('../lib/generator');

describe('generator', () => {
    describe('sanitizeName', () => {
        it('should sanitize table names with spaces', () => {
            expect(sanitizeName('my users')).toBe('my_users');
            expect(sanitizeName('order details')).toBe('order_details');
        });
    });

    describe('quoteIdent', () => {
        it('should quote postgres identifiers', () => {
            expect(quoteIdent('my users', 'postgres')).toBe('"my users"');
        });
        it('should quote mysql identifiers', () => {
            expect(quoteIdent('my users', 'mysql')).toBe('\`my users\`');
        });
        it('should quote mssql identifiers', () => {
            expect(quoteIdent('my users', 'mssql')).toBe('[my users]');
        });
    });

    describe('generateTools', () => {
        it('should generate tools for a valid table', () => {
            const tables = {
                'my users': [
                    { column_name: 'id', data_type: 'integer', is_primary_key: true },
                    { column_name: 'user name', data_type: 'text', is_primary_key: false, is_nullable: false }
                ]
            };

            const result = generateTools(tables, 'my_source', 'postgres');
            const tools = result.tools;

            expect(tools).toHaveProperty('list_my_users');
            expect(tools).toHaveProperty('get_my_users_by_id');
            expect(tools).toHaveProperty('update_my_users_by_id');
            expect(tools).toHaveProperty('delete_my_users_by_id');
            expect(tools).toHaveProperty('insert_my_users');

            expect(tools['list_my_users'].statement).toContain('SELECT * FROM "my users"');
            expect(tools['insert_my_users'].statement).toContain('INSERT INTO "my users" ("user name")');
            
            // Check params
            expect(tools['insert_my_users'].parameters.length).toBe(1);
            expect(tools['insert_my_users'].parameters[0].name).toBe('user name');
        });

        it('should skip insert for table with no insertable columns', () => {
            const tables = {
                'logs': [
                    { column_name: 'id', data_type: 'serial', is_primary_key: true }
                ]
            };

            const result = generateTools(tables, 'my_source', 'postgres');
            const tools = result.tools;
            expect(tools).not.toHaveProperty('insert_logs');
        });
    });
});
