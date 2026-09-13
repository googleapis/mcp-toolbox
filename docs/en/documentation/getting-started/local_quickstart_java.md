---
title: "Java Quickstart (Local)"
type: docs
weight: 5
description: >
  How to get started running MCP Toolbox locally with [Java](https://github.com/googleapis/mcp-toolbox-sdk-java) and PostgreSQL.
sample_filters: ["Java", "Quickstart", "Local"]
is_sample: true
---

## Before you begin

This guide assumes you have already done the following:

1. Installed [Java (JDK 17 or higher)][install-java].
1. Installed [Maven (3.8 or higher)][install-maven] or [Gradle][install-gradle].
1. Installed [PostgreSQL 16+ and the `psql` client][install-postgres].

[install-java]: https://adoptium.net/
[install-maven]: https://maven.apache.org/download.cgi
[install-gradle]: https://gradle.org/install/
[install-postgres]: https://www.postgresql.org/download/

### Cloud Setup (Optional)

{{< regionInclude "quickstart/shared/cloud_setup.md" "cloud_setup" >}}

## Step 1: Set up your database

{{< regionInclude "quickstart/shared/database_setup.md" "database_setup" >}}

## Step 2: Install and configure MCP Toolbox

{{< regionInclude "quickstart/shared/configure_toolbox.md" "configure_toolbox" >}}

## Step 3: Connect your application to MCP Toolbox

In this section, we will write and run a Java application that will load the Tools
from MCP Toolbox.

1. In a new terminal, add the SDK dependency to your project:

    {{< tabpane persist=header >}}
{{< tab header="Maven" lang="xml" >}}
<dependency>
    <groupId>com.google.cloud.mcp</groupId>
    <artifactId>mcp-toolbox-sdk-java</artifactId>
    <version>1.0.0</version>
</dependency>
{{< /tab >}}

{{< tab header="Gradle" lang="groovy" >}}
dependencies {
    implementation("com.google.cloud.mcp:mcp-toolbox-sdk-java:1.0.0")
}
{{< /tab >}}
    {{< /tabpane >}}

2. Create a new file named `Quickstart.java` and copy the following code:

{{< include "quickstart/java/core/Quickstart.java" "java" >}}

3. Run your application, and observe the results:

    {{< tabpane persist=header >}}
{{< tab header="Maven" lang="bash" >}}
mvn compile exec:java -Dexec.mainClass="Quickstart"
{{< /tab >}}

{{< tab header="Gradle" lang="bash" >}}
./gradlew run
{{< /tab >}}
    {{< /tabpane >}}

{{< notice info >}}
For more information, visit the [Java SDK
repo](https://github.com/googleapis/mcp-toolbox-sdk-java).
{{</ notice >}}
