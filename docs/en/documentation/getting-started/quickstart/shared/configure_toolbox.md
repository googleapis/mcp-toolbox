<!-- This file has been used in local_quickstart.md, local_quickstart_go.md & local_quickstart_js.md -->
<!-- [START configure_toolbox] -->

In this section, we will download Toolbox, configure our tools in a
`tools.yaml`, and then run the Toolbox server.

1.  Download the latest version of Toolbox as a binary:

    {{< notice tip >}}
    Select the
    [correct binary](https://github.com/googleapis/mcp-toolbox/releases)
    corresponding to your OS and CPU architecture.
    {{< /notice >}}
      <!-- {x-release-please-start-version} -->

    ```bash
    export OS="linux/amd64" # one of linux/amd64, darwin/arm64, darwin/amd64, windows/amd64, or windows/arm64
    curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.11.0/$OS/toolbox
    ```

      <!-- {x-release-please-end} -->

1.  [Optional] Verify the downloaded binary's authenticity and integrity:

    We recommend verifying the digital signature of the downloaded binary before running it.

    {{< tabpane persist=header >}}
    {{< tab header="Linux" lang="bash" >}}

    # 1. Download the detached GPG signature file (.asc)

    <!-- {x-release-please-start-version} -->
    curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.11.0/$OS/toolbox.asc
    <!-- {x-release-please-end} -->

    # 2. Import Google's public GPG signing key

    curl -fsSL https://dl.google.com/linux/linux_signing_key.pub | gpg --import

    # 3. Verify the signature against the downloaded binary

    gpg --verify toolbox.asc toolbox
    {{< /tab >}}
    {{< tab header="macOS" lang="bash" >}}

    # Verify the code signature

    codesign -v --verbose=4 toolbox

    {{< /tab >}}
    {{< tab header="Windows" lang="powershell" >}}

    # Verify the Authenticode digital signature

    Get-AuthenticodeSignature .\toolbox.exe | Format-List

    {{< /tab >}}
    {{< /tabpane >}}

1.  Make the binary executable (on Linux and macOS):

    ```bash
    chmod +x toolbox
    ```

1.  Write the following into a `tools.yaml` file. Be sure to update any fields
    such as `user`, `password`, or `database` that you may have customized in the
    previous step.

    {{< notice tip >}}
    In practice, use environment variable replacement with the format ${ENV_NAME}
    instead of hardcoding your secrets into the configuration file.
    {{< /notice >}}

    ```yaml
    kind: source
    name: my-pg-source
    type: postgres
    host: 127.0.0.1
    port: 5432
    database: toolbox_db
    user: toolbox_user
    password: my-password
    ---
    kind: tool
    name: search-hotels-by-name
    type: postgres-sql
    source: my-pg-source
    description: Search for hotels based on name.
    parameters:
      - name: name
        type: string
        description: The name of the hotel.
    statement: SELECT * FROM hotels WHERE name ILIKE '%' || $1 || '%';
    ---
    kind: tool
    name: search-hotels-by-location
    type: postgres-sql
    source: my-pg-source
    description: Search for hotels based on location.
    parameters:
      - name: location
        type: string
        description: The location of the hotel.
    statement: SELECT * FROM hotels WHERE location ILIKE '%' || $1 || '%';
    ---
    kind: tool
    name: book-hotel
    type: postgres-sql
    source: my-pg-source
    description: >-
      Book a hotel by its ID. If the hotel is successfully booked, returns a NULL, raises an error if not.
    parameters:
      - name: hotel_id
        type: string
        description: The ID of the hotel to book.
    statement: UPDATE hotels SET booked = B'1' WHERE id = $1;
    ---
    kind: tool
    name: update-hotel
    type: postgres-sql
    source: my-pg-source
    description: >-
      Update a hotel's check-in and check-out dates by its ID. Returns a message
      indicating  whether the hotel was successfully updated or not.
    parameters:
      - name: hotel_id
        type: string
        description: The ID of the hotel to update.
      - name: checkin_date
        type: string
        description: The new check-in date of the hotel.
      - name: checkout_date
        type: string
        description: The new check-out date of the hotel.
    statement: >-
      UPDATE hotels SET checkin_date = CAST($2 as date), checkout_date = CAST($3
      as date) WHERE id = $1;
    ---
    kind: tool
    name: cancel-hotel
    type: postgres-sql
    source: my-pg-source
    description: Cancel a hotel by its ID.
    parameters:
      - name: hotel_id
        type: string
        description: The ID of the hotel to cancel.
    statement: UPDATE hotels SET booked = B'0' WHERE id = $1;
    ---
    kind: toolset
    name: my-toolset
    tools:
      - search-hotels-by-name
      - search-hotels-by-location
      - book-hotel
      - update-hotel
      - cancel-hotel
    ```

    For more info on tools, check out the `Resources` section of the docs.

1.  Run the Toolbox server, pointing to the `tools.yaml` file created earlier:

    ```bash
    ./toolbox --config "tools.yaml"
    ```

    {{< notice note >}}
    Toolbox enables dynamic reloading by default. To disable, use the
    `--disable-reload` flag.
    {{< /notice >}}
    <!-- [END configure_toolbox] -->
