---
title: "Cloud Storage Source"
linkTitle: "Source"
type: docs
weight: 1
description: >
  Cloud Storage is Google Cloud's managed service for storing unstructured objects (files) in buckets. Toolbox connects at the project level, allowing tools to list buckets, list objects, read object metadata and content, mutate objects, and transfer objects between Cloud Storage and the server filesystem.
no_list: true
---

## About

[Cloud Storage][gcs-docs] is Google Cloud's managed service for storing
unstructured data (blobs) in containers called *buckets*. Buckets live in a GCP
project; objects are addressed by `gs://<bucket>/<object>`.

If you are new to Cloud Storage, you can try the
[quickstart][gcs-quickstart] to create a bucket and upload your first objects.

The Cloud Storage source is configured at the **project** level. Individual
tools take a `bucket` parameter, so a single configured source can operate
against any bucket the underlying credentials are authorized for.

[gcs-docs]: https://cloud.google.com/storage/docs
[gcs-quickstart]: https://cloud.google.com/storage/docs/discover-object-storage-console

## Available Tools

{{< list-tools >}}

## Requirements

### IAM Permissions

Cloud Storage uses [Identity and Access Management (IAM)][iam-overview] to
control access to buckets and objects. Toolbox uses your
[Application Default Credentials (ADC)][adc] to authorize and authenticate when
interacting with Cloud Storage.

In addition to [setting the ADC for your server][set-adc], ensure the IAM
identity has the appropriate role for the tools being exposed. Common roles:

- `roles/storage.bucketViewer` — read-only access to bucket metadata, including
  listing buckets with `cloud-storage-list-buckets` and reading bucket metadata
  with `cloud-storage-get-bucket-metadata`.
- `roles/storage.objectViewer` — read-only access to objects and object
  metadata, sufficient for `cloud-storage-list-objects`,
  `cloud-storage-get-object-metadata`, `cloud-storage-read-object`, and
  `cloud-storage-download-object`.
- `roles/storage.objectUser` — read and write access to objects, sufficient for
  `cloud-storage-upload-object`, `cloud-storage-write-object`, and
  `cloud-storage-copy-object`.
- `roles/storage.admin` — full control, including bucket management

Object mutation tools require the corresponding object permissions:

- `cloud-storage-upload-object`, `cloud-storage-write-object`, and
  `cloud-storage-copy-object` require object create or update permissions on
  the destination object.
- `cloud-storage-move-object` requires `storage.objects.move` and
  `storage.objects.create` in the same bucket. If the destination object
  already exists, `storage.objects.delete` is also required.
- `cloud-storage-delete-object` requires object delete permission.
- `cloud-storage-create-bucket` requires bucket create permission in the
  configured project.
- `cloud-storage-get-bucket-iam-policy` requires permission to read bucket IAM
  policy.
- `cloud-storage-delete-bucket` requires bucket delete permission, and the
  target bucket must be empty.

See [Cloud Storage IAM roles][gcs-iam] for the full list.

Tools that read from or write to local files operate on the filesystem of the
Toolbox server process, not the client machine. The server process must have
the corresponding local file permissions.

### Local Filesystem Access

`allowedLocalRoots` confines `cloud-storage-upload-object` and
`cloud-storage-download-object` to the directories you list. A path is accepted
only if it stays inside an allowed root both as written and after symbolic
links are resolved, so a link placed inside a root cannot be used to read or
overwrite a file outside it. Links that resolve back inside an allowed root
continue to work.

Two limits are worth planning around:

- Hard links are indistinguishable from ordinary files, so a hard link created
  inside an allowed root to a file outside it is still readable.
- The check runs just before the file is opened. A process that can write to an
  allowed root can, in principle, swap a path between those two moments.

Treat `allowedLocalRoots` as a guardrail layered on top of OS permissions
rather than a replacement for them: run Toolbox as a user that only has access
to the files it needs. If `allowedLocalRoots` is omitted, every absolute path
the server process can reach is allowed.

[iam-overview]: https://cloud.google.com/storage/docs/access-control/iam
[adc]: https://cloud.google.com/docs/authentication#adc
[set-adc]: https://cloud.google.com/docs/authentication/provide-credentials-adc
[gcs-iam]: https://cloud.google.com/storage/docs/access-control/iam-roles

## Example

```yaml
kind: source
name: my-gcs-source
type: "cloud-storage"
project: "my-project-id"
allowedBuckets:
  - "my-app-bucket"
  - "my-backup-bucket"
allowedLocalRoots:
  - "/workspace"
```

## Reference

| **field** | **type** | **required** | **description**                                                                 |
|-----------|:--------:|:------------:|---------------------------------------------------------------------------------|
| type      |  string  |     true     | Must be "cloud-storage".                                                        |
| project   |  string  |     true     | Id of the GCP project the configured source is associated with (e.g. "my-project-id"). |
| allowedBuckets | []string |    false     | List of GCS bucket names allowed for operations. If omitted, all buckets are allowed. |
| allowedLocalRoots | []string |    false     | List of absolute local filesystem directories allowed for file uploads and downloads. Paths must stay inside a root once symbolic links are resolved. If omitted, all paths are allowed. |
