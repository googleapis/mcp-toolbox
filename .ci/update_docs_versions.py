#!/usr/bin/env python3
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
import re
import sys

def main():
    # Read version from command line argument if provided, otherwise from cmd/version.txt
    if len(sys.argv) > 1 and sys.argv[1].strip():
        version_raw = sys.argv[1].strip()
    else:
        version_file_path = os.path.join(os.path.dirname(__file__), '../cmd/version.txt')
        if not os.path.exists(version_file_path):
            print(f"Error: Version file {version_file_path} not found.")
            sys.exit(1)
            
        with open(version_file_path, 'r') as f:
            version_raw = f.read().strip()
            
        if not version_raw:
            print("Error: Version file is empty.")
            sys.exit(1)
        
    # Ensure it starts with 'v'
    version = version_raw if version_raw.startswith('v') else f'v{version_raw}'
    print(f"Target version: {version}")

    hugo_dir = os.path.join(os.path.dirname(__file__), '../.hugo')
    hugo_cf_toml_path = os.path.join(hugo_dir, 'hugo.cloudflare.toml')

    # Update hugo.cloudflare.toml
    if os.path.exists(hugo_cf_toml_path):
        update_toml_file(hugo_cf_toml_path, version, remove_oldest=True)
    else:
        print(f"Warning: {hugo_cf_toml_path} not found.")

def update_toml_file(file_path, version, remove_oldest):
    print(f"Processing {file_path}...")
    with open(file_path, 'r') as f:
        content = f.read()

    # Check if version already exists
    version_pattern = f'version\\s*=\\s*"{re.escape(version)}"'
    if re.search(version_pattern, content):
        print(f"Version {version} already exists in {os.path.basename(file_path)}. No change needed.")
        return

    # Insert new version block right after the specific comment
    comment_marker = '# The order of versions in this file is mirrored into the dropdown'
    if comment_marker not in content:
        print(f"Error: Could not find comment marker in {os.path.basename(file_path)}.")
        sys.exit(1)

    new_block = f'[[params.versions]]\n  version = "{version}"\n  url = "https://mcp-toolbox.dev/{version}/"'
    target_str = comment_marker
    replacement_str = f"{comment_marker}\n\n{new_block}"
    
    updated_content = content.replace(target_str, replacement_str, 1)

    if remove_oldest:
        # We need to remove the last [[params.versions]] block.
        version_block_pattern = r'\[\[params\.versions\]\][\s\S]*?(?=\[|$)'
        matches = list(re.finditer(version_block_pattern, updated_content))
        print(f"Found {len(matches)} version blocks in {os.path.basename(file_path)}.")
        
        # We keep at most 7 version blocks (dev + 6 released versions).
        if len(matches) > 7:
            last_match = matches[-1]
            start, end = last_match.span()
            before = updated_content[:start]
            after = updated_content[end:]
            
            before = before.rstrip() + '\n\n'
            after = after.lstrip()
            
            updated_content = before + after
            print(f"Removed the oldest version block from {os.path.basename(file_path)}.")

    with open(file_path, 'w') as f:
        f.write(updated_content)
    print(f"Successfully updated {os.path.basename(file_path)}.")

if __name__ == '__main__':
    main()
