# Changelog

## [1.11.0](https://github.com/googleapis/mcp-toolbox/compare/v1.10.0...v1.11.0) (2026-09-04)


### Features

* **alloydb:** Provide actionable error when read-only mode is used on pre-PG17 ([#3902](https://github.com/googleapis/mcp-toolbox/issues/3902)) ([28ace11](https://github.com/googleapis/mcp-toolbox/commit/28ace115cfb02358ea2a3031fb5d26c71e9d583e))
* **sources:** Add ConnectOnce, a helper for connecting on first use ([#3905](https://github.com/googleapis/mcp-toolbox/issues/3905)) ([16c31fa](https://github.com/googleapis/mcp-toolbox/commit/16c31faf23f360bc900c49bc75847163ecf60e79))


### Bug Fixes

* **docs/cloudgda:** Document context fields, fix PSV example and links ([#3919](https://github.com/googleapis/mcp-toolbox/issues/3919)) ([ae47535](https://github.com/googleapis/mcp-toolbox/commit/ae47535434cb3052ca4645400c823d1d8735a06e))
* **sources:** Release the handle when a source fails to connect ([#3921](https://github.com/googleapis/mcp-toolbox/issues/3921)) ([0001190](https://github.com/googleapis/mcp-toolbox/commit/0001190a73a9742b1787f237be352030dbfbea5b))
* **test/alloydbainl:** Use explicit SQL alias prompt in integration test ([#3916](https://github.com/googleapis/mcp-toolbox/issues/3916)) ([596eaf9](https://github.com/googleapis/mcp-toolbox/commit/596eaf92d0059883090e59c3aca7163ed213ae95))

## [1.10.0](https://github.com/googleapis/mcp-toolbox/compare/v1.9.0...v1.10.0) (2026-08-27)


### Features

* **falkordb:** Add FalkorDB source and tools ([#3692](https://github.com/googleapis/mcp-toolbox/issues/3692)) ([a94702c](https://github.com/googleapis/mcp-toolbox/commit/a94702c13121736e0ceb05425af43a0b953ac5b5))
* **mcp:** Add Secure Parameters support as Toolbox experimental extension ([#3394](https://github.com/googleapis/mcp-toolbox/issues/3394)) ([9750d2d](https://github.com/googleapis/mcp-toolbox/commit/9750d2da4b1dc08761ab2b5510454e1a386ebce8))
* **server/mcp:** Support com.google.cloud/toolbox.v1 extension in v20260728 ([#3801](https://github.com/googleapis/mcp-toolbox/issues/3801)) ([f4f7da6](https://github.com/googleapis/mcp-toolbox/commit/f4f7da605245ff9e8d491d0c55591ba3b400623b))
* **skill:** Add fix-failing-tests skill for mcp-toolbox ([#3821](https://github.com/googleapis/mcp-toolbox/issues/3821)) ([168e69c](https://github.com/googleapis/mcp-toolbox/commit/168e69c048d65aa15b926f9dd7680245949cc57a))
* **sources:** Support native read-only mode and dynamic tool annotations ([#3872](https://github.com/googleapis/mcp-toolbox/issues/3872)) ([c257022](https://github.com/googleapis/mcp-toolbox/commit/c257022fed2cc5e9a286bf9fd78e91d76f9ff3b8)), refs [#3615](https://github.com/googleapis/mcp-toolbox/issues/3615) [#3816](https://github.com/googleapis/mcp-toolbox/issues/3816) [#3618](https://github.com/googleapis/mcp-toolbox/issues/3618) [#3851](https://github.com/googleapis/mcp-toolbox/issues/3851) [#3619](https://github.com/googleapis/mcp-toolbox/issues/3619) [#3617](https://github.com/googleapis/mcp-toolbox/issues/3617)
* **tools/firestore-mongodb:** Add tools for execute mql and get schema ([#3826](https://github.com/googleapis/mcp-toolbox/issues/3826)) ([4a85d75](https://github.com/googleapis/mcp-toolbox/commit/4a85d75a97f60eb601a42ceb29000e7d5d0ad8ae))
* **tool/mongodb:** Allow collection to be specified at runtime ([#3715](https://github.com/googleapis/mcp-toolbox/issues/3715)) ([7626eaf](https://github.com/googleapis/mcp-toolbox/commit/7626eaf51e0c0a901110670e5bfb721942043480))


### Bug Fixes

* **cloud-storage:** Resolve symlinks when enforcing local path boundaries ([#3810](https://github.com/googleapis/mcp-toolbox/issues/3810)) ([c2ada64](https://github.com/googleapis/mcp-toolbox/commit/c2ada6421f718cb861c7ccd5f0e8cd7e841a407f))
* **config:** Compare env var offsets in rune space when skipping comments ([#3856](https://github.com/googleapis/mcp-toolbox/issues/3856)) ([2e76934](https://github.com/googleapis/mcp-toolbox/commit/2e769343332cf84084a162e23f556490faa20d32))
* Merge prebuilt tools when reloading custom config ([#3864](https://github.com/googleapis/mcp-toolbox/issues/3864)) ([5a6d865](https://github.com/googleapis/mcp-toolbox/commit/5a6d865eff939ace8c803d3ce8831aa83d00a750))
* Normalize postgres UUIDs to strings ([#3806](https://github.com/googleapis/mcp-toolbox/issues/3806)) ([3b02f1d](https://github.com/googleapis/mcp-toolbox/commit/3b02f1d86ab774c1e0fb13d0de7b6b428da78b83))
* **postgres:** Filter background processes in postgres-list-active-queries ([#3885](https://github.com/googleapis/mcp-toolbox/issues/3885)) ([3d9e62a](https://github.com/googleapis/mcp-toolbox/commit/3d9e62a979be951bb04aeaf31aa4505598031f4a))

## [1.9.0](https://github.com/googleapis/mcp-toolbox/compare/v1.8.0...v1.9.0) (2026-08-14)


### Features

* **groups:** Add ttlMs and cacheScope customization to config ([#3805](https://github.com/googleapis/mcp-toolbox/issues/3805)) ([a5d4947](https://github.com/googleapis/mcp-toolbox/commit/a5d49472bad85e8955dc83852e65c5cd92f351a3))
* **migrate:** Convert toolset to group kind during migration ([#3704](https://github.com/googleapis/mcp-toolbox/issues/3704)) ([0adeaa5](https://github.com/googleapis/mcp-toolbox/commit/0adeaa51c4e132fe36553b24f88e8f62df90bfaa))
* **server/mcp:** Introduce generic client extension registry ([#3723](https://github.com/googleapis/mcp-toolbox/issues/3723)) ([016245c](https://github.com/googleapis/mcp-toolbox/commit/016245c21c254a05409a41845e0a8799518363a0))
* **skill:** Add review-prs skill for mcp-toolbox ([#3743](https://github.com/googleapis/mcp-toolbox/issues/3743)) ([5b7bacc](https://github.com/googleapis/mcp-toolbox/commit/5b7bacc73b9284160b73c4c3f7a53214c653e64a))
* **source/bigquery:** Add apiEndpoint field to override BigQuery API host ([#3437](https://github.com/googleapis/mcp-toolbox/issues/3437)) ([4da1600](https://github.com/googleapis/mcp-toolbox/commit/4da1600df9971789a0970d174be3c2ed1368f7c1))
* **source/databaseinsights:** Add databaseinsights source ([#3461](https://github.com/googleapis/mcp-toolbox/issues/3461)) ([3b9615d](https://github.com/googleapis/mcp-toolbox/commit/3b9615d020f0f7ba1d971f8c94ea7d6bc79f8468))
* **sources/spanner:** Rename execute_sql_dql to execute_sql_readonly ([#3776](https://github.com/googleapis/mcp-toolbox/issues/3776)) ([cf5a0c8](https://github.com/googleapis/mcp-toolbox/commit/cf5a0c8fbf52f1e09fc565109682cf52b6ebd553))
* **tools/bigtable:** Add admin lifecycle and listing tools ([#3596](https://github.com/googleapis/mcp-toolbox/issues/3596)) ([801d589](https://github.com/googleapis/mcp-toolbox/commit/801d5899665c120200d64bac45172741a785ae5d))
* **tools/bigtable:** Bigtable-list-schemas MCP tool ([#3683](https://github.com/googleapis/mcp-toolbox/issues/3683)) ([9228c61](https://github.com/googleapis/mcp-toolbox/commit/9228c61518a8828ad51bb83ad7eb820208104859))
* **tools/databaseinsights:** Add Advanced Query Insights tools for AlloyDB ([#3722](https://github.com/googleapis/mcp-toolbox/issues/3722)) ([74d18ae](https://github.com/googleapis/mcp-toolbox/commit/74d18ae0b3c4009436e0727bf7e16d714b9c41a4))
* **tools/looker:** Add additional tools to allow dashboards to be modified, and their layouts altered. ([#3597](https://github.com/googleapis/mcp-toolbox/issues/3597)) ([b2b80fb](https://github.com/googleapis/mcp-toolbox/commit/b2b80fbea64b6284f68b49c0ad3270ed79655282))
* **tools:** Add cloud-sql-connect-gce for pg, mysql, mssql ([#3740](https://github.com/googleapis/mcp-toolbox/issues/3740)) ([ca58fa4](https://github.com/googleapis/mcp-toolbox/commit/ca58fa4b525d6726b9792a9f6303fbcc26c9ca3f))


### Bug Fixes

* **auth/mcp:** Derive PRM URL from Toolbox URL ([#3765](https://github.com/googleapis/mcp-toolbox/issues/3765)) ([aa30842](https://github.com/googleapis/mcp-toolbox/commit/aa308422ad6dd73a014722c3ebf9628d7aa9cc8f))
* **config:** Ignore environment variables in YAML comments ([#3807](https://github.com/googleapis/mcp-toolbox/issues/3807)) ([79aa732](https://github.com/googleapis/mcp-toolbox/commit/79aa73247d35286e1cc4309883d539cf9a470686)), refs [#3793](https://github.com/googleapis/mcp-toolbox/issues/3793)
* **mcp:** Return Tool execution error for invalid input param ([#3799](https://github.com/googleapis/mcp-toolbox/issues/3799)) ([8120197](https://github.com/googleapis/mcp-toolbox/commit/81201978a7a1d2a786eb3707ddaa7b090dd1c454))
* **prebuilt/cloud-storage:** Declare tool collections as groups ([#3764](https://github.com/googleapis/mcp-toolbox/issues/3764)) ([7d468be](https://github.com/googleapis/mcp-toolbox/commit/7d468be107dfe476d77bd7f937b5dd9c61e5cdc8))
* **server/mcp:** Disallow client overriding URL bound parameters ([#3798](https://github.com/googleapis/mcp-toolbox/issues/3798)) ([f15a9c7](https://github.com/googleapis/mcp-toolbox/commit/f15a9c7082215bd8e9990395d01b5e4fa3b36c69))
* **server:** Avoid a nil-flusher panic in the SSE handler ([#3520](https://github.com/googleapis/mcp-toolbox/issues/3520)) ([947f42f](https://github.com/googleapis/mcp-toolbox/commit/947f42f3e8a07362466566043045491d2318db29))
* **tools/bigquery:** Keep the provider error classification in bigquery-execute-sql ([#3738](https://github.com/googleapis/mcp-toolbox/issues/3738)) ([42570b8](https://github.com/googleapis/mcp-toolbox/commit/42570b833656fdf71cf36ba3333a2419f48730d2))
* **tools/looker:** Scope the filters quoting rule to values in query description ([#3788](https://github.com/googleapis/mcp-toolbox/issues/3788)) ([78eb0b8](https://github.com/googleapis/mcp-toolbox/commit/78eb0b8580b0eebf781ba74e4cc12d1f94a7b65b))
* **util:** Convert exponent-form JSON numbers in ConvertNumbers ([#3730](https://github.com/googleapis/mcp-toolbox/issues/3730)) ([e9713ee](https://github.com/googleapis/mcp-toolbox/commit/e9713eec3acea912e0b6a254b845bd9da04f8192))

## [1.8.0](https://github.com/googleapis/mcp-toolbox/compare/v1.7.0...v1.8.0) (2026-07-28)


### Features

* Add `groups` support ([#3605](https://github.com/googleapis/mcp-toolbox/issues/3605)) ([e75ec3b](https://github.com/googleapis/mcp-toolbox/commit/e75ec3b5c84dfad5b69f2d42ec2d3408f22e2463))
* **prebuilt:** Migrate skills-repo toolsets to `kind: group` with descriptions ([#3595](https://github.com/googleapis/mcp-toolbox/issues/3595)) ([b895b36](https://github.com/googleapis/mcp-toolbox/commit/b895b36b10eb81dc609216fc5f76ae800d1c65f4))
* **server:** Add `/healthz` endpoint for container health checks ([#3060](https://github.com/googleapis/mcp-toolbox/issues/3060)) ([d5aefbc](https://github.com/googleapis/mcp-toolbox/commit/d5aefbc9e9bd914042224daaf0d4f9257ac01c88))
* **skills:** Add `--group` flag to generate a skill from one group ([#3585](https://github.com/googleapis/mcp-toolbox/issues/3585)) ([c1abd4f](https://github.com/googleapis/mcp-toolbox/commit/c1abd4fc4fcdfa52ba20aaf7d92424ca189c7282))
* **skills:** Default `--name` to `--group`, `--toolset`, or single `--prebuilt` name ([#3586](https://github.com/googleapis/mcp-toolbox/issues/3586)) ([2b33b08](https://github.com/googleapis/mcp-toolbox/commit/2b33b08c3a220657c8ab6a3e0ce1274badc2fe15))
* **skills:** Make description optional during skills gen ([#3584](https://github.com/googleapis/mcp-toolbox/issues/3584)) ([d0a8f14](https://github.com/googleapis/mcp-toolbox/commit/d0a8f14cbec1f9770da7f82a07b4e480f5a4c6a7))
* **tool/looker:** Add `get_field_value_suggestions` tool ([#3696](https://github.com/googleapis/mcp-toolbox/issues/3696)) ([3debe81](https://github.com/googleapis/mcp-toolbox/commit/3debe8113fe2c9af012a7369658bb7a9e64f40f4))
* **tools/dataplex-update-data-product-aspects:** Add dataplex-update-data-product-aspects tool ([#3607](https://github.com/googleapis/mcp-toolbox/issues/3607)) ([f940937](https://github.com/googleapis/mcp-toolbox/commit/f94093725c35189051321778576f801748ead226))
* Update draft specs to 2026-07-28 ([#3699](https://github.com/googleapis/mcp-toolbox/issues/3699)) ([cf128ff](https://github.com/googleapis/mcp-toolbox/commit/cf128ff94c4d39aea1eb17caa706ff0b73d8c780))


### Bug Fixes

* Re-add name validation to tools name ([#3654](https://github.com/googleapis/mcp-toolbox/issues/3654)) ([944f6ce](https://github.com/googleapis/mcp-toolbox/commit/944f6ce97bc77a92a052de92b88f0fc09ac7578c))
* Re-add tool validation during startup ([#3705](https://github.com/googleapis/mcp-toolbox/issues/3705)) ([25ce953](https://github.com/googleapis/mcp-toolbox/commit/25ce953559a201183f066f566dca5fb597efca39))
* **server:** Sort default toolset alphabetically for stable ordering ([#3539](https://github.com/googleapis/mcp-toolbox/issues/3539)) ([e5da24c](https://github.com/googleapis/mcp-toolbox/commit/e5da24c5dfd2208c7e947a20e58a2e2c82236241))
* **source/cloud-sql-admin:** Validate source instance for backup-run restore ([#3555](https://github.com/googleapis/mcp-toolbox/issues/3555)) ([3ebe500](https://github.com/googleapis/mcp-toolbox/commit/3ebe500ef7b27608464e891c7458df630ea4aab6))
* **source/dataplex:** Cleanup Dataplex Data Product resources that may have leaked due to previously aborted tests ([#3627](https://github.com/googleapis/mcp-toolbox/issues/3627)) ([3fb5a3f](https://github.com/googleapis/mcp-toolbox/commit/3fb5a3f2cdf42cbd078720df3369dbc33788c6b1))
* **source/http:** Block CGNAT 100.64.0.0/10 in default SSRF guard ([#3625](https://github.com/googleapis/mcp-toolbox/issues/3625)) ([a0f36f4](https://github.com/googleapis/mcp-toolbox/commit/a0f36f42c33df52727e934549e7d6ca7576fe9ac))
* **tools/looker:** Enhance looker-run-dashboard to handle result maker structures and merge queries ([#3698](https://github.com/googleapis/mcp-toolbox/issues/3698)) ([7666bc0](https://github.com/googleapis/mcp-toolbox/commit/7666bc08debdc4f81e1c6f95f41a1dbd9db3b365))
* **tools/looker:** Log request/response traces at debug, not error ([#3552](https://github.com/googleapis/mcp-toolbox/issues/3552)) ([c3ad445](https://github.com/googleapis/mcp-toolbox/commit/c3ad4451e8592606ce686d042623f9aa8142317f))

## [1.7.0](https://github.com/googleapis/mcp-toolbox/compare/v1.6.0...v1.7.0) (2026-07-16)


#### Features

* Add `quotaProject` support for BigQuery and Looker conversational analytics ([#2610](https://github.com/googleapis/mcp-toolbox/issues/2610)) ([f3e7ca9](https://github.com/googleapis/mcp-toolbox/commit/f3e7ca9a8f49ce79f5b6fbef23c45a643c4e9d44))
* **arcadedb:** Add arcadedb source and tools ([#2961](https://github.com/googleapis/mcp-toolbox/issues/2961)) ([351de00](https://github.com/googleapis/mcp-toolbox/commit/351de00781a08999e735356624370ea1e7414419))
* **cmd/internal,docs:** Add warning log that prebuilt tools are for developer use ([#3451](https://github.com/googleapis/mcp-toolbox/issues/3451)) ([8cffcef](https://github.com/googleapis/mcp-toolbox/commit/8cffcef2b109ba913bd63b87a61f75db9e957d2f))
* **source/postgres:** Add optional connectTimeout ([#3620](https://github.com/googleapis/mcp-toolbox/issues/3620)) ([b574b07](https://github.com/googleapis/mcp-toolbox/commit/b574b071fc27f717e90b4362864b1086be0c8995))
* **tool/clickhouse-sql:** Add native vector embedding support ([#3229](https://github.com/googleapis/mcp-toolbox/issues/3229)) ([6cbe1c2](https://github.com/googleapis/mcp-toolbox/commit/6cbe1c2f13e1f57818fa3aa872c2a467daf41ad8))
* **tool/dataplex-update-data-product,tool/dataplex-create-data-asset,tool/dataplex-update-data-asset:** Add update data product, create data asset, and update data asset tools for knowledge catalog source ([#3574](https://github.com/googleapis/mcp-toolbox/issues/3574)) ([721c204](https://github.com/googleapis/mcp-toolbox/commit/721c204053d2241f82cba0f7615b16f79ab77637))
* **tools/dataplex-create-data-product:** Add dataplex-create-data-product tool ([#3504](https://github.com/googleapis/mcp-toolbox/issues/3504)) ([5cee0d2](https://github.com/googleapis/mcp-toolbox/commit/5cee0d2a6668f7f3109fb3f00202f87d7c71d7f2))
* **tools/dataplex-get-data-asset:** Add dataplex-get-data-asset tool ([#3503](https://github.com/googleapis/mcp-toolbox/issues/3503)) ([1ddfbe9](https://github.com/googleapis/mcp-toolbox/commit/1ddfbe99f16742e9270c6ac21bde4b88c1c1988c))


### Bug Fixes

* **parameters:** Report the offending value in array/map type errors ([#3512](https://github.com/googleapis/mcp-toolbox/issues/3512)) ([4034d6f](https://github.com/googleapis/mcp-toolbox/commit/4034d6f7b820962495622dbedc64fea968c14963))
* **parameters:** Return an error instead of panicking on a non-string type field ([#3516](https://github.com/googleapis/mcp-toolbox/issues/3516)) ([66a0d53](https://github.com/googleapis/mcp-toolbox/commit/66a0d53b9fd11d6ee90b28ae1c411fc8685ab990))
* **source/looker:** Dynamically resolve public host URL ([#3603](https://github.com/googleapis/mcp-toolbox/issues/3603)) ([0428afd](https://github.com/googleapis/mcp-toolbox/commit/0428afd3e75339701a9dedb5199b89a001457349))
* **tool/looker-run-dashboard:** Add support for SQL Runner query tiles ([#3594](https://github.com/googleapis/mcp-toolbox/issues/3594)) ([0975d0a](https://github.com/googleapis/mcp-toolbox/commit/0975d0ac8bc5042f51a8b01eb5514d3ea138f145))


## [1.6.0](https://github.com/googleapis/mcp-toolbox/compare/v1.5.0...v1.6.0) (2026-06-30)


### Features

* Support MCP 2026 draft specs ([#3544](https://github.com/googleapis/mcp-toolbox/issues/3544)) ([d12eaa8](https://github.com/googleapis/mcp-toolbox/commit/d12eaa856bad70b49ba2b7b9f2882cffbf81220f))
* **release:** Add digital signature to Toolbox binaries ([#3528](https://github.com/googleapis/mcp-toolbox/issues/3528)) ([3f0f0af](https://github.com/googleapis/mcp-toolbox/commit/3f0f0af29007929b01e95ee2caef4fd2015d5f12))
* **tool/cloud-storage:** Configure object operation parameters ([#3529](https://github.com/googleapis/mcp-toolbox/issues/3529)) ([d6dc5fe](https://github.com/googleapis/mcp-toolbox/commit/d6dc5fe8e47b585415bd5f237b2ba5d8276bc0e0))
* **tool/cloud-storage:** Support configurable parameters ([#3478](https://github.com/googleapis/mcp-toolbox/issues/3478)) ([bc2de2a](https://github.com/googleapis/mcp-toolbox/commit/bc2de2ab386e0ae8591c4b7f7faa6165b7edf8a2))
* **tools/dataplex-list-data-products:** Add dataplex-list-data-products tool ([#3337](https://github.com/googleapis/mcp-toolbox/issues/3337)) ([6dd669a](https://github.com/googleapis/mcp-toolbox/commit/6dd669ad9bad3e54206a82b3af122d845e2cdc63))
* **tools/dataplex-get-data-product:** Add dataplex-get-data-product tool ([#3499](https://github.com/googleapis/mcp-toolbox/issues/3499)) ([7ea7a09](https://github.com/googleapis/mcp-toolbox/commit/7ea7a095cc6dcf85520a8f39c4cd5a20062289f7))
* **tools/dataplex-list-data-assets:** Add dataplex-list-data-assets tool ([#3500](https://github.com/googleapis/mcp-toolbox/issues/3500)) ([182f933](https://github.com/googleapis/mcp-toolbox/commit/182f9332d67cc90d81f78b536081a7f47766d966))
* **tools/looker:** Support complex filter_expression parameter in queries ([#3494](https://github.com/googleapis/mcp-toolbox/issues/3494)) ([997fb8c](https://github.com/googleapis/mcp-toolbox/commit/997fb8c39a4cb60173bcc8543118057e77e0fce4))
* **tools/looker:** Support dynamic_fields parameter in queries ([#3507](https://github.com/googleapis/mcp-toolbox/issues/3507)) ([cd22b89](https://github.com/googleapis/mcp-toolbox/commit/cd22b893573f87c0d5406490b71ddf317a07dc7b))


### Bug Fixes

* **tools/gda:** Support mTLS and GOOGLE_API_USE_MTLS_ENDPOINT for GDA client ([#3460](https://github.com/googleapis/mcp-toolbox/issues/3460)) ([cc2a61e](https://github.com/googleapis/mcp-toolbox/commit/cc2a61e9a522136d07f7be9b8bc87916f32bf076))
* **tools/looker-conversational-analytics:** Validate explore_references shape instead of panicking ([#3531](https://github.com/googleapis/mcp-toolbox/issues/3531)) ([b67419d](https://github.com/googleapis/mcp-toolbox/commit/b67419d34bfc437b6eace5abaaf02ae1339d83ee))
* **tool/looker-create-view-from-table:** Correct Looker API payload structure ([#3515](https://github.com/googleapis/mcp-toolbox/issues/3515)) ([18c539c](https://github.com/googleapis/mcp-toolbox/commit/18c539c5935c2a496e7e5da68241b4307d8f3e6e))

## [1.5.0](https://github.com/googleapis/mcp-toolbox/compare/v1.4.0...v1.5.0) (2026-06-18)


### Features

* **auth/google:** Require audience or clientId for mcpEnabled ([#3450](https://github.com/googleapis/mcp-toolbox/issues/3450)) ([59f7b6e](https://github.com/googleapis/mcp-toolbox/commit/59f7b6e8eaceffca042cb7e2f2b6e5e9284b6bc3))
* Enable per source level flags for sql commenter ([#3465](https://github.com/googleapis/mcp-toolbox/issues/3465)) ([ecce6b7](https://github.com/googleapis/mcp-toolbox/commit/ecce6b7bb551b947b0951cd684cce627a4b6cf1b))
* **mcp:** Add URL parameter binding for HTTP transport ([#3112](https://github.com/googleapis/mcp-toolbox/issues/3112)) ([0cc7b37](https://github.com/googleapis/mcp-toolbox/commit/0cc7b37b733b6a99dad5281af4024b26d730106a))
* **scylladb:** Adding support for ScyllaDB source and tool ([#3119](https://github.com/googleapis/mcp-toolbox/issues/3119)) ([2dada83](https://github.com/googleapis/mcp-toolbox/commit/2dada8306c8737e445c4f8cd3d213b72713c1834))
* **server:** Add support for toolset filtering in prebuilt CLI flag ([#3245](https://github.com/googleapis/mcp-toolbox/issues/3245)) ([7cc4f65](https://github.com/googleapis/mcp-toolbox/commit/7cc4f65a8e767e0da37cf21f0ff2568b38d32b8e))
* **skills:** Generate skills offline without live source connections ([#3388](https://github.com/googleapis/mcp-toolbox/issues/3388)) ([4c860b6](https://github.com/googleapis/mcp-toolbox/commit/4c860b66b03f0ebf86205e73cd8521ad90ccebe4))
* **skills:** Tolerate missing env vars during offline skills-generate ([#3399](https://github.com/googleapis/mcp-toolbox/issues/3399)) ([ea5d3e5](https://github.com/googleapis/mcp-toolbox/commit/ea5d3e5b9e60bf808e10d21b522954d76f7741b6))
* **source/cloud-storage:** Restrict bucket and local path access ([#3454](https://github.com/googleapis/mcp-toolbox/issues/3454)) ([2c3ca5d](https://github.com/googleapis/mcp-toolbox/commit/2c3ca5d256158e32c1386cb061b9cc3cb6c119a1))
* **tools/bigquery:** Add per tool query label in BigQuery jobs ([#1975](https://github.com/googleapis/mcp-toolbox/issues/1975)) ([3f6a49f](https://github.com/googleapis/mcp-toolbox/commit/3f6a49f93116b8805e5082916f1babf39e6da749))
* **tools/dataplex:** Add tools to support metadata enrichment workflow ([#3270](https://github.com/googleapis/mcp-toolbox/issues/3270)) ([05289aa](https://github.com/googleapis/mcp-toolbox/commit/05289aaa022e35996038b061f07eb43574e38bd2))
* **tools/mysql:** Add show-query-stats and list-all-locks tools for MySQL and Cloud SQL MySQL source ([#2954](https://github.com/googleapis/mcp-toolbox/issues/2954)) ([a9693bd](https://github.com/googleapis/mcp-toolbox/commit/a9693bd804e37bab8d4f261434d82bb538563976))
* **tools:** Decouple tool initialization from sources ([#3355](https://github.com/googleapis/mcp-toolbox/issues/3355)) ([32a24e3](https://github.com/googleapis/mcp-toolbox/commit/32a24e35b5bf107bcf5e89af2a9b7af3740747ee))


### Bug Fixes

* **auth/dataplex:** Fix failing source with service account credentials ([#3369](https://github.com/googleapis/mcp-toolbox/issues/3369)) ([ba4deef](https://github.com/googleapis/mcp-toolbox/commit/ba4deef140358e5876d73d355d664f629f7aeccc))
* **bigquery:** Wire maximumBytesBilled into prebuilt config ([#3385](https://github.com/googleapis/mcp-toolbox/issues/3385)) ([4abbf6e](https://github.com/googleapis/mcp-toolbox/commit/4abbf6e82cc4af4c1903d9143337c965987475a9))
* Bound MCP HTTP body size ([#3216](https://github.com/googleapis/mcp-toolbox/issues/3216)) ([d4f4342](https://github.com/googleapis/mcp-toolbox/commit/d4f434251392fb597779a90a12c63d21533ea187)) 
* **config:** Add doc/line context to parse errors ([#2957](https://github.com/googleapis/mcp-toolbox/issues/2957)) ([4b097da](https://github.com/googleapis/mcp-toolbox/commit/4b097daa2143817e55a9e557e8c1dea054bfc7b8))
* Escape delimiter characters in applyEscape to prevent SQL injection ([#2811](https://github.com/googleapis/mcp-toolbox/issues/2811)) ([932519a](https://github.com/googleapis/mcp-toolbox/commit/932519a9551861bf5f18787dc43b20d06350343f))
* **npm:** Source binary version from cmd/version.txt ([#3417](https://github.com/googleapis/mcp-toolbox/issues/3417)) ([6ffbdec](https://github.com/googleapis/mcp-toolbox/commit/6ffbdecaea98db5c16dc9eeca8fb73e4bbc48102))
* **prebuilt/alloydb-omni:** Require password env var explicitly ([#3398](https://github.com/googleapis/mcp-toolbox/issues/3398)) ([fcbe3e7](https://github.com/googleapis/mcp-toolbox/commit/fcbe3e70d3d4e671e97e424187dba907d7c5b10b))
* **server:** Fail if MCP auth is enabled together with enable-api ([#3435](https://github.com/googleapis/mcp-toolbox/issues/3435)) ([a6ff910](https://github.com/googleapis/mcp-toolbox/commit/a6ff910a602adece11f0a6581d6211e5927f7182))
* **server:** Return errors instead of panicking in InitializeConfigs ([#3397](https://github.com/googleapis/mcp-toolbox/issues/3397)) ([f48b01d](https://github.com/googleapis/mcp-toolbox/commit/f48b01dc1775e4583a06689a2e67fb06e5dd3c68))
* **source/cloudhealthcare:** Validate pageURL parameter to prevent SSRF ([#3453](https://github.com/googleapis/mcp-toolbox/issues/3453)) ([9abf47d](https://github.com/googleapis/mcp-toolbox/commit/9abf47d55c04e3593cf68191d94ef6086c07debc))
* **source/dataplex,source/datalineage:** Specify cloud-platform scope for default credentials ([#3376](https://github.com/googleapis/mcp-toolbox/issues/3376)) ([13e8c36](https://github.com/googleapis/mcp-toolbox/commit/13e8c364de6fa598f1864134eff5b756340c8358))
* **source/http:** Implement SSRF guard ([#3448](https://github.com/googleapis/mcp-toolbox/issues/3448)) ([24d7d29](https://github.com/googleapis/mcp-toolbox/commit/24d7d29b7483d68450d34ace9f0b826caefa2af3))
* **tool/bigquery-execute-sql:** Prevent dataset restriction bypass ([#3452](https://github.com/googleapis/mcp-toolbox/issues/3452)) ([ca6d5e3](https://github.com/googleapis/mcp-toolbox/commit/ca6d5e35160f3a51ab4fc6683e0a19a77851aebd))
* **tool/mysql-get-query-plan:** Prevent query execution bypass and statement injection ([#3235](https://github.com/googleapis/mcp-toolbox/issues/3235)) ([7ed1e7b](https://github.com/googleapis/mcp-toolbox/commit/7ed1e7b88c0a19ba36ac90824966607f828ee576))
* **tool/spanner-sql,tool/spanner-execute-sql:** Use read-only annotations when readOnly is set ([#3338](https://github.com/googleapis/mcp-toolbox/issues/3338)) ([8bde0ec](https://github.com/googleapis/mcp-toolbox/commit/8bde0ec08f8bf455f319523b4faacf32bdbc65ff))

## [1.4.0](https://github.com/googleapis/mcp-toolbox/compare/v1.3.0...v1.4.0) (2026-06-04)


### Features

* **ci:** Add support for windows/arm64 binary distribution ([#3231](https://github.com/googleapis/mcp-toolbox/issues/3231)) ([10abf3b](https://github.com/googleapis/mcp-toolbox/commit/10abf3b9e195a03f535e3807b7df9883899ef7c0))
* **datalineage:** Add Data Lineage integration ([#3285](https://github.com/googleapis/mcp-toolbox/issues/3285)) ([19353c3](https://github.com/googleapis/mcp-toolbox/commit/19353c37e17ab1f3599cafa04337a32a7baec1c3))
* **server:** Ignore unknown tools at startup with `--ignore-unknown-tools` flag ([#3353](https://github.com/googleapis/mcp-toolbox/issues/3353)) ([5f0304f](https://github.com/googleapis/mcp-toolbox/commit/5f0304f71231cce322ab2a3e458af07b392a06fc))
* **tools/cloudsqlpg:** Add remaining vector assist tools for Cloud SQL Postgres ([#3203](https://github.com/googleapis/mcp-toolbox/issues/3203)) ([b514cbd](https://github.com/googleapis/mcp-toolbox/commit/b514cbd7be2dbf49276f2327059194f9e7dc1be1))
* **tools/spanner-search-catalog:** Implement search_catalog tool ([#3140](https://github.com/googleapis/mcp-toolbox/issues/3140)) ([defc086](https://github.com/googleapis/mcp-toolbox/commit/defc0860c8876fcd465728ac6ce41de8262ed572))


### Bug Fixes

* **auth/generic:** Enforce issuer presence in opaque token validation ([#3360](https://github.com/googleapis/mcp-toolbox/issues/3360)) ([1d8df0d](https://github.com/googleapis/mcp-toolbox/commit/1d8df0df590383ba56091b6e4d7c37ab7d7d9749))
* **auth:** Separate Google and Generic MCP OAuth verification ([#3341](https://github.com/googleapis/mcp-toolbox/issues/3341)) ([dfd66ee](https://github.com/googleapis/mcp-toolbox/commit/dfd66ee7de6fe9750d932d30bf3b67a2f4d2a176))
* **mcp:** Support annotations and metadata within Tools to earlier MCP schemas ([#3300](https://github.com/googleapis/mcp-toolbox/issues/3300)) ([9a88c72](https://github.com/googleapis/mcp-toolbox/commit/9a88c72792563e4868c82a4f3be55e6af25c1477))
* **oracle:** Remove trailing semicolons from prebuilt tools ([#3215](https://github.com/googleapis/mcp-toolbox/issues/3215)) ([fcad02d](https://github.com/googleapis/mcp-toolbox/commit/fcad02de73ffe9c6ecf29572f0f92674aacbe493))
* **server/auth:** Centralize tool scopes validation ([#3335](https://github.com/googleapis/mcp-toolbox/issues/3335)) ([adce4ab](https://github.com/googleapis/mcp-toolbox/commit/adce4abb27327aae4e9736581df7a544b55c939e))
* **server:** Return null id for batch request rejection ([#3333](https://github.com/googleapis/mcp-toolbox/issues/3333)) ([0b18d58](https://github.com/googleapis/mcp-toolbox/commit/0b18d58aea131baceb1c70f300879de8ecdf569e))
* **source/dataplex:** Limit search results to pageSize ([#3323](https://github.com/googleapis/mcp-toolbox/issues/3323)) ([905c1f6](https://github.com/googleapis/mcp-toolbox/commit/905c1f68fcdb848d014aeaed3193af2f94fac507)), closes [#3308](https://github.com/googleapis/mcp-toolbox/issues/3308)
* **telemetry:** Allow GCP project override ([#2960](https://github.com/googleapis/mcp-toolbox/issues/2960)) ([3c83ba5](https://github.com/googleapis/mcp-toolbox/commit/3c83ba5ab1d2ab38369e0b5c47396fabf6ecabef))
* **tool/bigquery:** Prevent `allowedDatasets` bypass in forecast query ([#3324](https://github.com/googleapis/mcp-toolbox/issues/3324)) ([45df461](https://github.com/googleapis/mcp-toolbox/commit/45df461e84e4a8ca6706f30f9a31096828f846eb))
* **tool/clickhouse:** Handle ignored ProcessParameters error ([#3340](https://github.com/googleapis/mcp-toolbox/issues/3340)) ([ddfd887](https://github.com/googleapis/mcp-toolbox/commit/ddfd88717dd1ce2c2706cc35785e481e8794479a))
* **tools/clickhouse,tools/bigquery:** Validate identifier parameters to prevent injection ([#3219](https://github.com/googleapis/mcp-toolbox/issues/3219)) ([2f45f75](https://github.com/googleapis/mcp-toolbox/commit/2f45f75525ac1b5dbbe3056e07441ef9a3bd6680))
* **tools/looker:** Escape filter values for unquoted parameters ([#3289](https://github.com/googleapis/mcp-toolbox/issues/3289)) ([1711156](https://github.com/googleapis/mcp-toolbox/commit/17111562dc2585c7372798f7f18e5ff3d32d21fe))

## [1.3.0](https://github.com/googleapis/mcp-toolbox/compare/v1.2.0...v1.3.0) (2026-05-21)


### Features

* **auth:** Implement MCP auth tool-level scopes validation ([#3049](https://github.com/googleapis/mcp-toolbox/issues/3049)) ([c528985](https://github.com/googleapis/mcp-toolbox/commit/c528985149060adb648f85b5486391bd72d6727e))
* **looker:** Propagate client IP from incoming MCP requests to downstream SDK calls ([#3253](https://github.com/googleapis/mcp-toolbox/issues/3253)) ([75da6c2](https://github.com/googleapis/mcp-toolbox/commit/75da6c21dd29d7e8e70eac1b747e3946097e7459))
* Setup SQLCommenter and allow client metadata  ([#3064](https://github.com/googleapis/mcp-toolbox/issues/3064)) ([9f1f9b3](https://github.com/googleapis/mcp-toolbox/commit/9f1f9b321dcd05cce55dbff1bbaebfc44a4c9907))
* **tool/cloudsqladmin:** Add `cloud-sql-admin-execute-sql-many` and `cloud-sql-admin-sql-many` ([#3083](https://github.com/googleapis/mcp-toolbox/issues/3083)) ([ef300a8](https://github.com/googleapis/mcp-toolbox/commit/ef300a8401e5d5458bc08186fe4d3529e4bab15a))


### Bug Fixes

* **auth/generic:** Fix generic auth expiration field and integration with `authRequired` ([#3251](https://github.com/googleapis/mcp-toolbox/issues/3251)) ([f4d16c0](https://github.com/googleapis/mcp-toolbox/commit/f4d16c09b12c4d3297a9aedca706c9830382a4e3))
* Enforce toolset/promptset boundary on tools/call and prompts/get ([#3036](https://github.com/googleapis/mcp-toolbox/issues/3036)) ([c739b80](https://github.com/googleapis/mcp-toolbox/commit/c739b805ba5ab0e156016fe7c8ce67bc1c138e5a))
* **tools/http:** Prevent path traversal and base path scope escape ([#3218](https://github.com/googleapis/mcp-toolbox/issues/3218)) ([80a6602](https://github.com/googleapis/mcp-toolbox/commit/80a66021205e032a424fff87b3dc6d92da58aa77))
* **tools/looker:** Return a 401 error to MCP client when Looker returns a 401 ([#3233](https://github.com/googleapis/mcp-toolbox/issues/3233)) ([4f409a3](https://github.com/googleapis/mcp-toolbox/commit/4f409a3283d533bddcf4756a1d58c228744b3931))
* **tools/looker:** Strip wrapping quotes from filter values for unquoted parameters ([#3273](https://github.com/googleapis/mcp-toolbox/issues/3273)) ([1e3de96](https://github.com/googleapis/mcp-toolbox/commit/1e3de96daa9bc06253d05b0caf63d499878fb70e))
* **tools:** Initialize query result slices to empty array ([#3250](https://github.com/googleapis/mcp-toolbox/issues/3250)) ([60ddf48](https://github.com/googleapis/mcp-toolbox/commit/60ddf487468bfd11c7f9346f16a33a8986f89f84))

## [1.2.0](https://github.com/googleapis/mcp-toolbox/compare/v1.1.0...v1.2.0) (2026-05-07)


### Features

* Add support for HTTPS/TLS listener ([#3126](https://github.com/googleapis/mcp-toolbox/issues/3126)) ([8bc385d](https://github.com/googleapis/mcp-toolbox/commit/8bc385d7d6fd9ed2ad13503d9feb503de0b512b1))
* **source/bigquery:** Add maximumBytesBilled source config ([#2724](https://github.com/googleapis/mcp-toolbox/issues/2724)) ([42f2d07](https://github.com/googleapis/mcp-toolbox/commit/42f2d07c83c6302feaff04ae34050d6045c71204))
* **source/cloud-storage:** Add bucket and object management tools ([#3129](https://github.com/googleapis/mcp-toolbox/issues/3129)) ([8de9bcf](https://github.com/googleapis/mcp-toolbox/commit/8de9bcf1e2521762f46459f2a55ae934aa1c5c07))
* **source/cloud-storage:** Add Cloud Storage source with list_objects and read_object tools ([#3081](https://github.com/googleapis/mcp-toolbox/issues/3081)) ([da27b37](https://github.com/googleapis/mcp-toolbox/commit/da27b3754df836132b835e5afd5d04830ee8af42))
* **source/cloud-storage:** Add write/copy/move/delete object tools ([#3139](https://github.com/googleapis/mcp-toolbox/issues/3139)) ([b225fc4](https://github.com/googleapis/mcp-toolbox/commit/b225fc44cc6df033c7fe2fdca308ad0a1e0af2ba))
* **tools/knowledge-catalog:** Search Data Quality Scans ([#2444](https://github.com/googleapis/mcp-toolbox/issues/2444)) ([1c63551](https://github.com/googleapis/mcp-toolbox/commit/1c635513a17df61fff725bf23deaffa92170057c))


### Bug Fixes

* Allow converting string literal block with list ([#3050](https://github.com/googleapis/mcp-toolbox/issues/3050)) ([36ab2a9](https://github.com/googleapis/mcp-toolbox/commit/36ab2a98f9f2d03c27eea389d2281bfc4581ffa1)), closes [#3023](https://github.com/googleapis/mcp-toolbox/issues/3023)
* **mcp:** Implement router-level logger injection for MCP auth ([#3067](https://github.com/googleapis/mcp-toolbox/issues/3067)) ([ccc7cf5](https://github.com/googleapis/mcp-toolbox/commit/ccc7cf5ee8a1bacb6b57faf41ae5a1cc3da5299e))
* Prevent test.db from being created during unit tests ([#3042](https://github.com/googleapis/mcp-toolbox/issues/3042)) ([d10d2ca](https://github.com/googleapis/mcp-toolbox/commit/d10d2caeb7c9eda7d17d6dbd9f63363b2bc23a7a))
* Remove hardcoded * allowed origin for sse ([#3054](https://github.com/googleapis/mcp-toolbox/issues/3054)) ([c4c7bd9](https://github.com/googleapis/mcp-toolbox/commit/c4c7bd917e686de68e2be866cfe3872c3439efae))
* **sources/postgres:** Apply URL encoding to query string params ([#3020](https://github.com/googleapis/mcp-toolbox/issues/3020)) ([6b860f4](https://github.com/googleapis/mcp-toolbox/commit/6b860f4486ff5b024090a8945bc6bce63547860b))
* **tool/looker-conversational-analytics:** OAuth token in GDA payload fix ([#3058](https://github.com/googleapis/mcp-toolbox/issues/3058)) ([6632d96](https://github.com/googleapis/mcp-toolbox/commit/6632d96724c5076ee44eb248d7de5c7d2d80d7b1))
* **tools/bigquery-execute-sql:** Avoid surfacing invalid queries as MCP 500s ([#3056](https://github.com/googleapis/mcp-toolbox/issues/3056)) ([7ed92c8](https://github.com/googleapis/mcp-toolbox/commit/7ed92c802313fc1b10daaa8a02457ba178ea2e22))
* **tools/looker:** Fix OAuth for Converational Analytics ([#3044](https://github.com/googleapis/mcp-toolbox/issues/3044)) ([f9e3e55](https://github.com/googleapis/mcp-toolbox/commit/f9e3e55d42ae9f5d1ecbda4fb7c9a4f3d42451b1))

## [1.1.0](https://github.com/googleapis/mcp-toolbox/compare/v1.0.0...v1.1.0) (2026-04-13)


### Features

* **tools/cloudsqlpg:** Add vector assist tools for Cloud SQL Postgres ([#2909](https://github.com/googleapis/mcp-toolbox/issues/2909)) ([7a6d849](https://github.com/googleapis/mcp-toolbox/commit/7a6d8492fa316237b55ea7570588b0c9860b616c))


### Bug Fixes

* **looker:** Convert configuration yaml to flat format ([#3022](https://github.com/googleapis/mcp-toolbox/issues/3022)) ([45c05e3](https://github.com/googleapis/mcp-toolbox/commit/45c05e37eac867c5a444d950bc51fdf1b1b687ea))

### Docs Update

* **knowledge-catalog:** Rename dataplex to knowledge-catalog across docs ([#3039](https://github.com/googleapis/mcp-toolbox/pull/3039)) ([45c05e3](https://github.com/googleapis/mcp-toolbox/commit/24ce6ce3bc6468d2b4b11a86b90ea223daa7e6cf))

## [1.0.0](https://github.com/googleapis/mcp-toolbox/compare/v0.32.0...v1.0.0) (2026-04-10)


> [!IMPORTANT]
> This is the first stable release. Please review the [UPGRADING.md](UPGRADING.md) guide for instructions on migrating from previous beta versions.


### ⚠ BREAKING CHANGES

* **tools/elasticsearch:** add vector search support and remove query passing through param ([#2891](https://github.com/googleapis/mcp-toolbox/issues/2891))
* **tools/looker:** refactor looker-git-branch tool into 5 separate tools ([#2976](https://github.com/googleapis/mcp-toolbox/issues/2976))

### Features

* **auth:** Support opaque token validation for `generic` authService ([#2944](https://github.com/googleapis/mcp-toolbox/issues/2944)) ([c924701](https://github.com/googleapis/mcp-toolbox/commit/c924701adede95877594423d78b7ae72fe0b9c82))
* **cloudsqlpg:** Run `SELECT 1` after successful connection attempt ([#2997](https://github.com/googleapis/mcp-toolbox/issues/2997)) ([6ed9700](https://github.com/googleapis/mcp-toolbox/commit/6ed9700e15f08b31e65eb0afa605f4a8ea937e66))
* **tools/bigquerysql:** Add semantic search support ([#2890](https://github.com/googleapis/mcp-toolbox/issues/2890)) ([862c396](https://github.com/googleapis/mcp-toolbox/commit/862c396cadfa1d95d12cc121312a81035c22cbad))
* **tools/elasticsearch-execute-esql:** Add Tool to execute arbitrary ES/QL queries ([#3013](https://github.com/googleapis/mcp-toolbox/issues/3013)) ([ae49fb7](https://github.com/googleapis/mcp-toolbox/commit/ae49fb737031d783b6734a0ea35488dd0f4c7ccc))
* **tools/elasticsearch:** Add vector search support and remove query passing through param ([#2891](https://github.com/googleapis/mcp-toolbox/issues/2891)) ([d44e879](https://github.com/googleapis/mcp-toolbox/commit/d44e879336f6628790e3f1dca2477cb56fe8f080))
* **tools/looker:** Refactor looker-git-branch tool into 5 separate tools ([#2976](https://github.com/googleapis/mcp-toolbox/issues/2976)) ([b2472d4](https://github.com/googleapis/mcp-toolbox/commit/b2472d4926dacc496fc6956185fb281b5e75f56f))
* **tools/mysql:** Add list-table-stats-tool to list table statistics in MySQL and Cloud SQL MySQL source. ([#2938](https://github.com/googleapis/mcp-toolbox/issues/2938)) ([dc2c2b4](https://github.com/googleapis/mcp-toolbox/commit/dc2c2b44e512e34d4d3a0b9c63b59374c37c4c4a))

## [0.32.0](https://github.com/googleapis/mcp-toolbox/compare/v0.31.0...v0.32.0) (2026-04-08)


### ⚠ BREAKING CHANGES

* update repo name ([#2968](https://github.com/googleapis/mcp-toolbox/issues/2968))

### Features

* Add MCP tool annotations to all remaining tools ([#2221](https://github.com/googleapis/mcp-toolbox/issues/2221)) ([ea09db9](https://github.com/googleapis/mcp-toolbox/commit/ea09db90ce3ed78225dc246cedefd30064a88fad))
* **bigquery:** Add conversational analytics tools for Data Agents ([#2517](https://github.com/googleapis/mcp-toolbox/issues/2517)) ([2490a4b](https://github.com/googleapis/mcp-toolbox/commit/2490a4b4fb3c9232270f6f4347b8556d2d6e0390))
* **embeddingModel:** Add Backend API selection fields ([#2592](https://github.com/googleapis/mcp-toolbox/issues/2592)) ([912aa9e](https://github.com/googleapis/mcp-toolbox/commit/912aa9edd7bc3ce932828003fbd67d1a3b9c2348))
* **skills:** Add Claude Code support to generated scripts ([#2966](https://github.com/googleapis/mcp-toolbox/issues/2966)) ([a1609e1](https://github.com/googleapis/mcp-toolbox/commit/a1609e10a2eaf4ea68eae36acec3eed355b8a052))
* **skills:** Add codex user agent ([#2973](https://github.com/googleapis/mcp-toolbox/issues/2973)) ([070e939](https://github.com/googleapis/mcp-toolbox/commit/070e9399c02f088d43175ce6bf343378beb7f584))
* **skills:** Tool invocation via npx ([#2916](https://github.com/googleapis/mcp-toolbox/issues/2916)) ([377dc5b](https://github.com/googleapis/mcp-toolbox/commit/377dc5b00145a0044eef39314dd6b0ef5966fcd7))
* **sources/singlestore:** Add ConnectionParams to SingleStore Config ([#2555](https://github.com/googleapis/mcp-toolbox/issues/2555)) ([73e2a8c](https://github.com/googleapis/mcp-toolbox/commit/73e2a8c5cb5ef095a53df9be1d600cc7d508ea19))
* **tool/dataplex-lookup-context:** Relax project constraint and enforce location ([#2952](https://github.com/googleapis/mcp-toolbox/issues/2952)) ([7ebfdf1](https://github.com/googleapis/mcp-toolbox/commit/7ebfdf1cfc7debac020de51be1aa01c81628879e))
* **tools/looker:** Looker agent management from MCP ([#2830](https://github.com/googleapis/mcp-toolbox/issues/2830)) ([649d4ad](https://github.com/googleapis/mcp-toolbox/commit/649d4ad26f0d4d883ae3dbcc0c7f8069adc0d639))
* **ui:** Update to use `/mcp` endpoint ([#2829](https://github.com/googleapis/mcp-toolbox/issues/2829)) ([c3059c2](https://github.com/googleapis/mcp-toolbox/commit/c3059c233502a1e99abb4d87e4b9bfe7c6ea7a4a))


### Bug Fixes

* **bigquery:** Add impersonateServiceAccount to prebuilt config ([#2770](https://github.com/googleapis/mcp-toolbox/issues/2770)) ([9c3a748](https://github.com/googleapis/mcp-toolbox/commit/9c3a748de43eb588586f22590ff74bd433b24d68))
* **quickstart:** Robust tool lookup and modernize dependencies in Python samples ([#2863](https://github.com/googleapis/mcp-toolbox/issues/2863)) ([4c0845d](https://github.com/googleapis/mcp-toolbox/commit/4c0845dc9081d79046dea5f28a032d531faff40f))
* **skills:** Fix skill generation template ([#2914](https://github.com/googleapis/mcp-toolbox/issues/2914)) ([a01a15e](https://github.com/googleapis/mcp-toolbox/commit/a01a15ed1aa9a83eda8362578fed2e3a3c8dde99))
* **skills:** Prevent empty strings overriding optional env vars in node scripts ([#2963](https://github.com/googleapis/mcp-toolbox/issues/2963)) ([c52adeb](https://github.com/googleapis/mcp-toolbox/commit/c52adeba76fc13d0e6e415f6393def0648e478d6))
* **tests/bigquery:** Implement uuid-based isolation and reliable resource cleanup ([#2547](https://github.com/googleapis/mcp-toolbox/issues/2547)) ([479d842](https://github.com/googleapis/mcp-toolbox/commit/479d8424046406d50af02b0602e6bac58aea534f))
* **tests/Bigtable:** Implement uuid-based isolation and reliable resource cleanup ([#2880](https://github.com/googleapis/mcp-toolbox/issues/2880)) ([a769f15](https://github.com/googleapis/mcp-toolbox/commit/a769f15c3ab8d631198546909a6dd1f09446e6b0))
* Update error for ConvertConfig function ([#2993](https://github.com/googleapis/mcp-toolbox/issues/2993)) ([62bdabb](https://github.com/googleapis/mcp-toolbox/commit/62bdabb512d7875d2760c1cd8eb331221b58a09c))


### Code Refactoring

* Update repo name ([#2968](https://github.com/googleapis/mcp-toolbox/issues/2968)) ([3aae809](https://github.com/googleapis/mcp-toolbox/commit/3aae8097f1bda00e41667fb41c02094167c96ace))

## [0.31.0](https://github.com/googleapis/mcp-toolbox/compare/v0.30.0...v0.31.0) (2026-03-26)


### ⚠ BREAKING CHANGES

* release upgraded docsite ([#2831](https://github.com/googleapis/mcp-toolbox/issues/2831))
* **http:** sanitize non-2xx error output ([#2654](https://github.com/googleapis/mcp-toolbox/issues/2654))
* add a new `enable-api` flag ([#2846](https://github.com/googleapis/mcp-toolbox/issues/2846))
* remove deprecations and update tools-file flag ([#2806](https://github.com/googleapis/mcp-toolbox/issues/2806))

### Features

* Add a new `enable-api` flag ([#2846](https://github.com/googleapis/mcp-toolbox/issues/2846)) ([7a070da](https://github.com/googleapis/mcp-toolbox/commit/7a070dae4f1833671649ea605f36659675d402a9))
* **auth:** Add generic `authService` type for MCP ([#2619](https://github.com/googleapis/mcp-toolbox/issues/2619)) ([f6678f8](https://github.com/googleapis/mcp-toolbox/commit/f6678f8e29aa3346f4f73ce33cec37b4753d6947))
* **auth:** Add Protected Resource Metadata endpoint ([#2698](https://github.com/googleapis/mcp-toolbox/issues/2698)) ([b53dcf2](https://github.com/googleapis/mcp-toolbox/commit/b53dcf20694599f8b961c501a532bd122630b6f4))
* **auth:** Support manual PRM override ([#2717](https://github.com/googleapis/mcp-toolbox/issues/2717)) ([283e4e3](https://github.com/googleapis/mcp-toolbox/commit/283e4e33172571e4b20fa6a3ea0cfc632a565e6a))
* **dataplex:** Add support for lookup context tool. ([#2744](https://github.com/googleapis/mcp-toolbox/issues/2744)) ([facb69d](https://github.com/googleapis/mcp-toolbox/commit/facb69d01fe0c7ff9e2e1c40804dd00762e508a6))
* Remove deprecations and update tools-file flag ([#2806](https://github.com/googleapis/mcp-toolbox/issues/2806)) ([ab64c95](https://github.com/googleapis/mcp-toolbox/commit/ab64c9514a467d92a4547eda5a4ecdd08f86b0c9))


### Bug Fixes

* **ci:** Remove search index generation from preview deployment workflow ([#2859](https://github.com/googleapis/mcp-toolbox/issues/2859)) ([f8891b8](https://github.com/googleapis/mcp-toolbox/commit/f8891b82fcaaef240e1031cd9f784749d91d4210))
* **docs:** Skip empty folders in pagination & reduce PR comment noise ([#2853](https://github.com/googleapis/mcp-toolbox/issues/2853)) ([9ebd93a](https://github.com/googleapis/mcp-toolbox/commit/9ebd93a8ecb9bae673aa77a859803629fc7a4e1d))
* **http:** Sanitize non-2xx error output ([#2654](https://github.com/googleapis/mcp-toolbox/issues/2654)) ([5bef954](https://github.com/googleapis/mcp-toolbox/commit/5bef954507c8e23b6c9b0eb2551265e4be32b452))
* **skills:** Fix integer parameter parsing through agent skills ([#2847](https://github.com/googleapis/mcp-toolbox/issues/2847)) ([4564efe](https://github.com/googleapis/mcp-toolbox/commit/4564efe75436b4081d9f3d1f7c912bc64c13f850))


### Documentation

* Release upgraded docsite ([#2831](https://github.com/googleapis/mcp-toolbox/issues/2831)) ([5b25ce0](https://github.com/googleapis/mcp-toolbox/commit/5b25ce081235b21c884e27057cd4a2fa4d0d7c0e))


## [0.30.0](https://github.com/googleapis/mcp-toolbox/compare/v0.29.0...v0.30.0) (2026-03-20)


### Features

* **cli:** Add migrate subcommand ([#2679](https://github.com/googleapis/mcp-toolbox/issues/2679)) ([12171f7](https://github.com/googleapis/mcp-toolbox/commit/12171f7a02bcd34ce647db10abdb79bb2dac7ace))
* **cli:** Add serve subcommand ([#2550](https://github.com/googleapis/mcp-toolbox/issues/2550)) ([1e2c7c7](https://github.com/googleapis/mcp-toolbox/commit/1e2c7c7804c67bebf5e2ee9b67c6feb6f05292fd))
* **skill:** One skill per toolset ([#2733](https://github.com/googleapis/mcp-toolbox/issues/2733)) ([5b85c65](https://github.com/googleapis/mcp-toolbox/commit/5b85c65960dba9bfaf4cadca6d44532a153976e1))
* **source/oracledb:** Add Oracle DB for MCP tools and configurations, updated tools and documentation ([#2625](https://github.com/googleapis/mcp-toolbox/issues/2625)) ([e350fc7](https://github.com/googleapis/mcp-toolbox/commit/e350fc7879182aaf592a70c3509ed061164b3913))
* **tools/looker:** Support git_branch tools for looker. ([#2718](https://github.com/googleapis/mcp-toolbox/issues/2718)) ([70ed8a0](https://github.com/googleapis/mcp-toolbox/commit/70ed8a0dcb8e654b748a6e3e1c5ef283c26006da))
* **tools/dataplex-search-entries:** Add `scope` support to search_entries tool ([#2740](https://github.com/googleapis/mcp-toolbox/issues/2740)) ([10af468](https://github.com/googleapis/mcp-toolbox/commit/10af4682ccd51070463604124293968944d05017))


### Bug Fixes

* **cloudloggingadmin:** Increase log injesting time and add auth test ([#2772](https://github.com/googleapis/mcp-toolbox/issues/2772)) ([50b4457](https://github.com/googleapis/mcp-toolbox/commit/50b4457095ec4ac881b3b12719da24d35141f65d))
* **oracle:** Normalize encoded proxy usernames in go-ora DSN ([#2469](https://github.com/googleapis/mcp-toolbox/issues/2469)) ([b1333cd](https://github.com/googleapis/mcp-toolbox/commit/b1333cd27117655f8ab09f222721e14bea74b487))
* **postgres:** Update execute-sql tool to avoid multi-statements parameter ([#2707](https://github.com/googleapis/mcp-toolbox/issues/2707)) ([58bc772](https://github.com/googleapis/mcp-toolbox/commit/58bc772f882f0d9e00f403e73fbec812dd8a03ac))
* **skills:** Improve flag validation and silence unit test output ([#2759](https://github.com/googleapis/mcp-toolbox/issues/2759)) ([f3da6aa](https://github.com/googleapis/mcp-toolbox/commit/f3da6aa5e23b609a1ac9ecc098bccea02f2388ab))
* **test:** Address flaky healthcare integration test run ([#2742](https://github.com/googleapis/mcp-toolbox/issues/2742)) ([9590821](https://github.com/googleapis/mcp-toolbox/commit/9590821bc7d86c5cbacd29b21d4f85b427a87db4))


### Reverts

* **ci:** Implement conditional sharding logic in integration tests ([#2763](https://github.com/googleapis/mcp-toolbox/issues/2763)) ([1528d7c](https://github.com/googleapis/mcp-toolbox/commit/1528d7c38dfaa30bdecbe59c79ba926fa6d18356))

## [0.29.0](https://github.com/googleapis/mcp-toolbox/compare/v0.28.0...v0.29.0) (2026-03-13)


### ⚠ BREAKING CHANGES

* **source/alloydb:** restructure prebuilt toolsets  ([#2639](https://github.com/googleapis/mcp-toolbox/issues/2639))
* **source/spanner:** restructure prebuilt toolsets ([#2641](https://github.com/googleapis/mcp-toolbox/issues/2641))
* **source/dataplex:** restructure prebuilt toolsets ([#2640](https://github.com/googleapis/mcp-toolbox/issues/2640))
* **source/oss-db:** restructure prebuilt toolsets ([#2638](https://github.com/googleapis/mcp-toolbox/issues/2638))
* **source/cloudsql:** restructure prebuilt toolsets ([#2635](https://github.com/googleapis/mcp-toolbox/issues/2635))
* **source/bigquery:** restructure prebuilt toolsets ([#2637](https://github.com/googleapis/mcp-toolbox/issues/2637))
* **source/firestore:** restructure prebuilt toolsets ([#2636](https://github.com/googleapis/mcp-toolbox/issues/2636))
* telemetry metrics updates as per semantic convention ([#2566](https://github.com/googleapis/mcp-toolbox/issues/2566))

### Features

* Add user agent to embeddings generation ([#2572](https://github.com/googleapis/mcp-toolbox/issues/2572)) ([287251a](https://github.com/googleapis/mcp-toolbox/commit/287251a0cfed4d24617e5d0d957026a94f65d820))
* **skill:** Attach user agent metadata for generated skill ([#2697](https://github.com/googleapis/mcp-toolbox/issues/2697)) ([9598a6a](https://github.com/googleapis/mcp-toolbox/commit/9598a6a32597b9c9abdb0f20c06d86a01b0d011f))
* **skills:** Add additional-notes flag to generate skills command ([#2696](https://github.com/googleapis/mcp-toolbox/issues/2696)) ([73bf962](https://github.com/googleapis/mcp-toolbox/commit/73bf962459b76872f748248bb5e289be232a30b6))
* **skill:** Update skill generation logic ([#2646](https://github.com/googleapis/mcp-toolbox/issues/2646)) ([c233eee](https://github.com/googleapis/mcp-toolbox/commit/c233eee98cd9621526cb286245f3874f5bd6e7da))
* **source/alloydb:** Restructure prebuilt toolsets  ([#2639](https://github.com/googleapis/mcp-toolbox/issues/2639)) ([5f3f063](https://github.com/googleapis/mcp-toolbox/commit/5f3f063fc7335e47e35fa1a4f93616abbd7959d5))
* **source/bigquery:** Restructure prebuilt toolsets ([#2637](https://github.com/googleapis/mcp-toolbox/issues/2637)) ([dc984ba](https://github.com/googleapis/mcp-toolbox/commit/dc984badd79f54ff423713a763648c6a6880a640))
* **sources/bigquery:** Support custom oauth header name ([#2564](https://github.com/googleapis/mcp-toolbox/issues/2564)) ([d3baf77](https://github.com/googleapis/mcp-toolbox/commit/d3baf77d61ab30d97edc93587e6f0365b8523fee))
* **source/cloudsql:** Restructure prebuilt toolsets ([#2635](https://github.com/googleapis/mcp-toolbox/issues/2635)) ([99613dc](https://github.com/googleapis/mcp-toolbox/commit/99613dcc7a06bd3a2324d20e1ef41404cf6fd9d5))
* **source/dataplex:** Restructure prebuilt toolsets ([#2640](https://github.com/googleapis/mcp-toolbox/issues/2640)) ([acb9a80](https://github.com/googleapis/mcp-toolbox/commit/acb9a80cf2438e04c76cf10267b1c9ca9227da0b))
* **source/firestore:** Restructure prebuilt toolsets ([#2636](https://github.com/googleapis/mcp-toolbox/issues/2636)) ([22ab7b9](https://github.com/googleapis/mcp-toolbox/commit/22ab7b9365eab21bfa04da64574fadbd0746f669))
* **source/oss-db:** Restructure prebuilt toolsets ([#2638](https://github.com/googleapis/mcp-toolbox/issues/2638)) ([5196c6a](https://github.com/googleapis/mcp-toolbox/commit/5196c6a78eb256ec83d847385c69bfebece48c87))
* **source/spanner:** Restructure prebuilt toolsets ([#2641](https://github.com/googleapis/mcp-toolbox/issues/2641)) ([ea2b698](https://github.com/googleapis/mcp-toolbox/commit/ea2b698b03517c400bbaef27f56c4d3abead8b2c))
* Telemetry metrics updates as per semantic convention ([#2566](https://github.com/googleapis/mcp-toolbox/issues/2566)) ([131d764](https://github.com/googleapis/mcp-toolbox/commit/131d764f895c14908e29914b3c0c273d91a2654f))
* **tools/mongodb:** Add tool annotations to MongoDB tools for improved LLM understanding ([#2219](https://github.com/googleapis/mcp-toolbox/issues/2219)) ([b7a5f80](https://github.com/googleapis/mcp-toolbox/commit/b7a5f80b42b3c1564870e2868aeab87d82a85d39))
* **tools/serverless-spark:** Add get_session_template tool  ([#2308](https://github.com/googleapis/mcp-toolbox/issues/2308)) ([a136e16](https://github.com/googleapis/mcp-toolbox/commit/a136e169b3551a14b081624d7f50e1c32f0fb857))
* **tools/serverless-spark:** Add list/get sessions tools ([#2576](https://github.com/googleapis/mcp-toolbox/issues/2576)) ([a554298](https://github.com/googleapis/mcp-toolbox/commit/a554298535444671228fc08f6e3139d199a8b6b4))


### Bug Fixes

* Improve list locks integration test for postgres ([#2279](https://github.com/googleapis/mcp-toolbox/issues/2279)) ([d9ebe5d](https://github.com/googleapis/mcp-toolbox/commit/d9ebe5d4bf1b7ca04cae47386b36c38ca5b77b8a))
* **mcp:** Guard nil SSE session lookup and return 400 for missing session ([#2681](https://github.com/googleapis/mcp-toolbox/issues/2681)) ([f66189f](https://github.com/googleapis/mcp-toolbox/commit/f66189fe43cb711da3a041fa31edbacd7bbc7153))
* **oracle:** Update oracle-execute-sql tool interface to match source signature ([#2627](https://github.com/googleapis/mcp-toolbox/issues/2627)) ([81699a3](https://github.com/googleapis/mcp-toolbox/commit/81699a375b7e5af37945f4124aa4c5f2a1a9f7a6))
* Return AllParams for GetParameter() for tools with templateParameter([#2734](https://github.com/googleapis/mcp-toolbox/issues/2734)) ([bfd7ba6](https://github.com/googleapis/mcp-toolbox/commit/bfd7ba601a52294fa71623d88af71bd0288bf887))
* **server/mcp:** Scope defer span.End inside loop iteration ([#2558](https://github.com/googleapis/mcp-toolbox/issues/2558)) ([c88a62d](https://github.com/googleapis/mcp-toolbox/commit/c88a62dcf4c16118ae706cc43d18cad827e7496d)), closes [#2549](https://github.com/googleapis/mcp-toolbox/issues/2549)
* **skill:** Fix env variable propagation ([#2645](https://github.com/googleapis/mcp-toolbox/issues/2645)) ([5271368](https://github.com/googleapis/mcp-toolbox/commit/52713687208994c423da64333cb0a04fb483f794))
* **sources/looker:** Looker and looker-dev prebuilt tools should share one source definition. ([#2620](https://github.com/googleapis/mcp-toolbox/issues/2620)) ([df7f2fd](https://github.com/googleapis/mcp-toolbox/commit/df7f2fd7d5b75211dbbbd471c84f0ec5097ca7ad))
* **telemetry:** Histogram buckets from OTel standard to MCP standards ([#2729](https://github.com/googleapis/mcp-toolbox/issues/2729)) ([87cd4a0](https://github.com/googleapis/mcp-toolbox/commit/87cd4a0bf48605225ef25ca554cc787def976d11))
* **ui:** Remove module from script ([#2703](https://github.com/googleapis/mcp-toolbox/issues/2703)) ([6943ab6](https://github.com/googleapis/mcp-toolbox/commit/6943ab6839d21da7b6a4241700c7891c6f4a9b2c))
* Update toolset attributes naming ([#2554](https://github.com/googleapis/mcp-toolbox/issues/2554)) ([3d6ae4e](https://github.com/googleapis/mcp-toolbox/commit/3d6ae4eeaf5acfbde83374a244573edd8fc9012b))

## [0.28.0](https://github.com/googleapis/mcp-toolbox/compare/v0.27.0...v0.28.0) (2026-03-02)


### Features

* Add polling system to dynamic reloading ([#2466](https://github.com/googleapis/mcp-toolbox/issues/2466)) ([fcaac9b](https://github.com/googleapis/mcp-toolbox/commit/fcaac9bb957226ee3db1baea24330f337ba88ab7))
* Added basic template for sdks doc migrate ([#1961](https://github.com/googleapis/mcp-toolbox/issues/1961)) ([87f2eaf](https://github.com/googleapis/mcp-toolbox/commit/87f2eaf79cdecca7b939151e1543eccf2f812a69))
* **dataproc:** Add dataproc source and list/get clusters/jobs tools ([#2407](https://github.com/googleapis/mcp-toolbox/issues/2407)) ([cc05e57](https://github.com/googleapis/mcp-toolbox/commit/cc05e5745d1c25a6088702b827cd098250164b7e))
* **sources/postgres:** Add configurable pgx query execution mode ([#2477](https://github.com/googleapis/mcp-toolbox/issues/2477)) ([57b77bc](https://github.com/googleapis/mcp-toolbox/commit/57b77bca09ce6ee260bd64af9be5fcef593e9acb))
* **sources/redis:** Add TLS support for Redis connections ([#2432](https://github.com/googleapis/mcp-toolbox/issues/2432)) ([d6af290](https://github.com/googleapis/mcp-toolbox/commit/d6af2907fd2dca5a6751d7d42090dd7ebb8ccd48))
* **tools/looker:** Enable Get All Lookml Tests, Run LookML Tests, and Create View From Table tools for Looker ([#2522](https://github.com/googleapis/mcp-toolbox/issues/2522)) ([e01139a](https://github.com/googleapis/mcp-toolbox/commit/e01139a90268f8587b9823be1157259c1bcbfd66))
* **tools/looker:** Tools to list/create/delete directories ([#2488](https://github.com/googleapis/mcp-toolbox/issues/2488)) ([0036d8c](https://github.com/googleapis/mcp-toolbox/commit/0036d8c35844c3de2079cb5b2479781e8938525b))
* **ui:** Make tool list panel resizable ([#2253](https://github.com/googleapis/mcp-toolbox/issues/2253)) ([276cf60](https://github.com/googleapis/mcp-toolbox/commit/276cf604a2bb41861639ed6881557e38dd97a614))


### Bug Fixes

* **ci:** Add path for forked PR unit test runs ([#2540](https://github.com/googleapis/mcp-toolbox/issues/2540)) ([04dd2a7](https://github.com/googleapis/mcp-toolbox/commit/04dd2a77603c7babf01da724dfb77808e3f25fe5))
* Deflake alloydb omni ([#2431](https://github.com/googleapis/mcp-toolbox/issues/2431)) ([62b8309](https://github.com/googleapis/mcp-toolbox/commit/62b830987d65c3573214d04e50742476097ee9e9))
* **docs/adk:** Resolve dependency duplication ([#2418](https://github.com/googleapis/mcp-toolbox/issues/2418)) ([4d44abb](https://github.com/googleapis/mcp-toolbox/commit/4d44abb4638926ca50b0fa4dcf10a03e7fab657f))
* **docs/langchain:** Fix core at 0.3.0 and align compatible dependencies ([#2426](https://github.com/googleapis/mcp-toolbox/issues/2426)) ([36edfd3](https://github.com/googleapis/mcp-toolbox/commit/36edfd3d506e839c092d04cbca1799b5ebc38160))
* Enforce required validation for explicit null parameter values ([#2519](https://github.com/googleapis/mcp-toolbox/issues/2519)) ([d5e9512](https://github.com/googleapis/mcp-toolbox/commit/d5e9512a237e658f9b9127fdd8c174ec023c3310))
* **oracle:** Enable DML operations and resolve incorrect array type error ([#2323](https://github.com/googleapis/mcp-toolbox/issues/2323)) ([72146a4](https://github.com/googleapis/mcp-toolbox/commit/72146a4b1605bcdd3e1038106bfb1f899e677e39))
* **server/mcp:** Guard nil dereference in sseManager.get ([#2557](https://github.com/googleapis/mcp-toolbox/issues/2557)) ([e534196](https://github.com/googleapis/mcp-toolbox/commit/e534196303c2b8d9b6e599ac25add337e6fc9b8f)), closes [#2548](https://github.com/googleapis/mcp-toolbox/issues/2548)
* **tests/postgres:** Implement uuid-based isolation and reliable resource cleanup ([#2377](https://github.com/googleapis/mcp-toolbox/issues/2377)) ([8a96fb1](https://github.com/googleapis/mcp-toolbox/commit/8a96fb1a8874baa3688e566f3dea8a0912fcf2df))
* **tests/postgres:** Restore list_schemas test and implement dynamic owner ([#2521](https://github.com/googleapis/mcp-toolbox/issues/2521)) ([7041e79](https://github.com/googleapis/mcp-toolbox/commit/7041e797347f337d6f7f44ca051ae31acd58babe))
* **tests:** Resolve LlamaIndex dependency conflict in JS quickstart ([#2597](https://github.com/googleapis/mcp-toolbox/issues/2597)) ([ac11f5a](https://github.com/googleapis/mcp-toolbox/commit/ac11f5af9c7bcf228d667e1b8e08b5dc49ad91a0))

## [0.27.0](https://github.com/googleapis/mcp-toolbox/compare/v0.26.0...v0.27.0) (2026-02-12)


### ⚠ BREAKING CHANGES

* Update configuration file v2 ([#2369](https://github.com/googleapis/mcp-toolbox/issues/2369))([293c1d6](https://github.com/googleapis/mcp-toolbox/commit/293c1d6889c39807855ba5e01d4c13ba2a4c50ce))
* Update/add detailed telemetry for mcp endpoint compliant with OTEL semantic convention ([#1987](https://github.com/googleapis/mcp-toolbox/issues/1987)) ([478a0bd](https://github.com/googleapis/mcp-toolbox/commit/478a0bdb59288c1213f83862f95a698b4c2c0aab))

### Features

* **cli/invoke:** Add support for direct tool invocation from CLI ([#2353](https://github.com/googleapis/mcp-toolbox/issues/2353)) ([6e49ba4](https://github.com/googleapis/mcp-toolbox/commit/6e49ba436ef2390c13feaf902b29f5907acffb57))
* **cli/skills:** Add support for generating agent skills from toolset ([#2392](https://github.com/googleapis/mcp-toolbox/issues/2392)) ([80ef346](https://github.com/googleapis/mcp-toolbox/commit/80ef34621453b77bdf6a6016c354f102a17ada04))
* **cloud-logging-admin:** Add source, tools, integration test and docs ([#2137](https://github.com/googleapis/mcp-toolbox/issues/2137)) ([252fc30](https://github.com/googleapis/mcp-toolbox/commit/252fc3091af10d25d8d7af7e047b5ac87a5dd041))
* **cockroachdb:** Add CockroachDB integration with cockroach-go ([#2006](https://github.com/googleapis/mcp-toolbox/issues/2006)) ([1fdd99a](https://github.com/googleapis/mcp-toolbox/commit/1fdd99a9b609a5e906acce414226ff44d75d5975))
* **prebuiltconfigs/alloydb-omni:** Implement Alloydb omni dataplane tools ([#2340](https://github.com/googleapis/mcp-toolbox/issues/2340)) ([e995349](https://github.com/googleapis/mcp-toolbox/commit/e995349ea0756c700d188b8f04e9459121219f0c))
* **server:** Add Tool call error categories ([#2387](https://github.com/googleapis/mcp-toolbox/issues/2387)) ([32cb4db](https://github.com/googleapis/mcp-toolbox/commit/32cb4db712d27579c1bf29e61cbd0bed02286c28))
* **tools/looker:** support `looker-validate-project` tool ([#2430](https://github.com/googleapis/mcp-toolbox/issues/2430)) ([a15a128](https://github.com/googleapis/mcp-toolbox/commit/a15a12873f936b0102aeb9500cc3bcd71bb38c34))



### Bug Fixes

* **dataplex:** Capture GCP HTTP errors in MCP Toolbox ([#2347](https://github.com/googleapis/mcp-toolbox/issues/2347)) ([1d7c498](https://github.com/googleapis/mcp-toolbox/commit/1d7c4981164c34b4d7bc8edecfd449f57ad11e15))
* **sources/cockroachdb:** Update kind to type ([#2465](https://github.com/googleapis/mcp-toolbox/issues/2465)) ([2d341ac](https://github.com/googleapis/mcp-toolbox/commit/2d341acaa61c3c1fe908fceee8afbd90fb646d3a))
* Surface Dataplex API errors in MCP results ([#2347](https://github.com/googleapis/mcp-toolbox/pull/2347))([1d7c498](https://github.com/googleapis/mcp-toolbox/commit/1d7c4981164c34b4d7bc8edecfd449f57ad11e15))

## [0.26.0](https://github.com/googleapis/mcp-toolbox/compare/v0.25.0...v0.26.0) (2026-01-22)


### ⚠ BREAKING CHANGES

* Validate tool naming ([#2305](https://github.com/googleapis/mcp-toolbox/issues/2305)) ([5054212](https://github.com/googleapis/mcp-toolbox/commit/5054212fa43017207fe83275d27b9fbab96e8ab5))
* **tools/cloudgda:** Update description and parameter name for cloudgda tool ([#2288](https://github.com/googleapis/mcp-toolbox/issues/2288)) ([6b02591](https://github.com/googleapis/mcp-toolbox/commit/6b025917032394a66840488259db8ff2c3063016))

### Features

* Add new `user-agent-metadata` flag ([#2302](https://github.com/googleapis/mcp-toolbox/issues/2302)) ([adc9589](https://github.com/googleapis/mcp-toolbox/commit/adc9589766904d9e3cbe0a6399222f8d4bb9d0cc))
* Add remaining flag to Toolbox server in MCP registry ([#2272](https://github.com/googleapis/mcp-toolbox/issues/2272)) ([5e0999e](https://github.com/googleapis/mcp-toolbox/commit/5e0999ebf5cdd9046e96857738254b2e0561b6d2))
* **embeddingModel:** Add embedding model to MCP handler ([#2310](https://github.com/googleapis/mcp-toolbox/issues/2310)) ([e4f60e5](https://github.com/googleapis/mcp-toolbox/commit/e4f60e56335b755ef55b9553d3f40b31858ec8d9))
* **sources/bigquery:** Make maximum rows returned from queries configurable ([#2262](https://github.com/googleapis/mcp-toolbox/issues/2262)) ([4abf0c3](https://github.com/googleapis/mcp-toolbox/commit/4abf0c39e717d53b22cc61efb65e09928c598236))
* **prebuilt/cloud-sql:** Add create backup tool for Cloud SQL ([#2141](https://github.com/googleapis/mcp-toolbox/issues/2141)) ([8e0fb03](https://github.com/googleapis/mcp-toolbox/commit/8e0fb0348315a80f63cb47b3c7204869482448f4))
* **prebuilt/cloud-sql:** Add restore backup tool for Cloud SQL ([#2171](https://github.com/googleapis/mcp-toolbox/issues/2171)) ([00c3e6d](https://github.com/googleapis/mcp-toolbox/commit/00c3e6d8cba54e2ab6cb271c7e6b378895df53e1))
* Support combining multiple prebuilt configurations ([#2295](https://github.com/googleapis/mcp-toolbox/issues/2295)) ([e535b37](https://github.com/googleapis/mcp-toolbox/commit/e535b372ea81864d644a67135a1b07e4e519b4b4))
* Support MCP specs version 2025-11-25 ([#2303](https://github.com/googleapis/mcp-toolbox/issues/2303)) ([4d23a3b](https://github.com/googleapis/mcp-toolbox/commit/4d23a3bbf2797b1f7fe328aeb5789e778121da23))
* **tools:** Add `valueFromParam` support to Tool config ([#2333](https://github.com/googleapis/mcp-toolbox/issues/2333)) ([15101b1](https://github.com/googleapis/mcp-toolbox/commit/15101b1edbe2b85a4a5f9f819c23cf83138f4ee1))


### Bug Fixes

* **tools/cloudhealthcare:** Add check for client authorization before retrieving token string ([#2327](https://github.com/googleapis/mcp-toolbox/issues/2327)) ([c25a233](https://github.com/googleapis/mcp-toolbox/commit/c25a2330fea2ac382a398842c9e572e4e19bcb08))

## [0.25.0](https://github.com/googleapis/mcp-toolbox/compare/v0.24.0...v0.25.0) (2026-01-08)


### Features

* Add `embeddingModel` support ([#2121](https://github.com/googleapis/mcp-toolbox/issues/2121)) ([9c62f31](https://github.com/googleapis/mcp-toolbox/commit/9c62f313ff5edf0a3b5b8a3e996eba078fba4095))
* Add `allowed-hosts` flag ([#2254](https://github.com/googleapis/mcp-toolbox/issues/2254)) ([17b41f6](https://github.com/googleapis/mcp-toolbox/commit/17b41f64531b8fe417c28ada45d1992ba430dc1b))
* Add parameter default value to manifest ([#2264](https://github.com/googleapis/mcp-toolbox/issues/2264)) ([9d1feca](https://github.com/googleapis/mcp-toolbox/commit/9d1feca10810fa42cb4c94a409252f1bd373ee36))
* **snowflake:** Add Snowflake Source and Tools ([#858](https://github.com/googleapis/mcp-toolbox/issues/858)) ([b706b5b](https://github.com/googleapis/mcp-toolbox/commit/b706b5bc685aeda277f277868bae77d38d5fd7b6))
* **prebuilt/cloud-sql-mysql:** Update CSQL MySQL prebuilt tools to use IAM ([#2202](https://github.com/googleapis/mcp-toolbox/issues/2202)) ([731a32e](https://github.com/googleapis/mcp-toolbox/commit/731a32e5360b4d6862d81fcb27d7127c655679a8))
* **sources/bigquery:** Make credentials scope configurable ([#2210](https://github.com/googleapis/mcp-toolbox/issues/2210)) ([a450600](https://github.com/googleapis/mcp-toolbox/commit/a4506009b93771b77fb05ae97044f914967e67ed))
* **sources/trino:** Add ssl verification options and fix docs example ([#2155](https://github.com/googleapis/mcp-toolbox/issues/2155)) ([4a4cf1e](https://github.com/googleapis/mcp-toolbox/commit/4a4cf1e712b671853678dba99c4dc49dd4fc16a2))
* **tools/looker:** Add ability to set destination folder with `make_look` and `make_dashboard`. ([#2245](https://github.com/googleapis/mcp-toolbox/issues/2245)) ([eb79339](https://github.com/googleapis/mcp-toolbox/commit/eb793398cd1cc4006d9808ccda5dc7aea5e92bd5))
* **tools/postgressql:** Add tool to list store procedure ([#2156](https://github.com/googleapis/mcp-toolbox/issues/2156)) ([cf0fc51](https://github.com/googleapis/mcp-toolbox/commit/cf0fc515b57d9b84770076f3c0c5597c4597ef62))
* **tools/postgressql:** Add Parameter `embeddedBy` config support ([#2151](https://github.com/googleapis/mcp-toolbox/issues/2151)) ([17b70cc](https://github.com/googleapis/mcp-toolbox/commit/17b70ccaa754d15bcc33a1a3ecb7e652520fa600))


### Bug Fixes

* **server:** Add `embeddingModel` config initialization ([#2281](https://github.com/googleapis/mcp-toolbox/issues/2281)) ([a779975](https://github.com/googleapis/mcp-toolbox/commit/a7799757c9345f99b6d2717841fbf792d364e1a2))
* **sources/cloudgda:** Add import for cloudgda source ([#2217](https://github.com/googleapis/mcp-toolbox/issues/2217)) ([7daa411](https://github.com/googleapis/mcp-toolbox/commit/7daa4111f4ebfb0a35319fd67a8f7b9f0f99efcf))
* **tools/alloydb-wait-for-operation:** Fix connection message generation ([#2228](https://github.com/googleapis/mcp-toolbox/issues/2228)) ([7053fbb](https://github.com/googleapis/mcp-toolbox/commit/7053fbb1953653143d39a8510916ea97a91022a6))
* **tools/alloydbainl:** Only add psv when NL Config Param is defined ([#2265](https://github.com/googleapis/mcp-toolbox/issues/2265)) ([ef8f3b0](https://github.com/googleapis/mcp-toolbox/commit/ef8f3b02f2f38ce94a6ba9acf35d08b9469bef4e))
* **tools/looker:** Looker client OAuth nil pointer error ([#2231](https://github.com/googleapis/mcp-toolbox/issues/2231)) ([268700b](https://github.com/googleapis/mcp-toolbox/commit/268700bdbf8281de0318d60ca613ed3672990b20))

## [0.24.0](https://github.com/googleapis/mcp-toolbox/compare/v0.23.0...v0.24.0) (2025-12-19)


### Features

* **sources/cloud-gemini-data-analytics:** Add the Gemini Data Analytics (GDA) integration for DB NL2SQL conversion to Toolbox ([#2181](https://github.com/googleapis/mcp-toolbox/issues/2181)) ([aa270b2](https://github.com/googleapis/mcp-toolbox/commit/aa270b2630da2e3d618db804ca95550445367dbc))
* **source/cloudsqlmysql:** Add support for IAM authentication in Cloud SQL MySQL source ([#2050](https://github.com/googleapis/mcp-toolbox/issues/2050)) ([af3d3c5](https://github.com/googleapis/mcp-toolbox/commit/af3d3c52044bea17781b89ce4ab71ff0f874ac20))
* **sources/oracle:** Add Oracle OCI and Wallet support ([#1945](https://github.com/googleapis/mcp-toolbox/issues/1945)) ([8ea39ec](https://github.com/googleapis/mcp-toolbox/commit/8ea39ec32fbbaa97939c626fec8c5d86040ed464))
* Support combining prebuilt and custom tool configurations ([#2188](https://github.com/googleapis/mcp-toolbox/issues/2188)) ([5788605](https://github.com/googleapis/mcp-toolbox/commit/57886058188aa5d2a51d5846a98bc6d8a650edd1))
* **tools/mysql-get-query-plan:** Add new `mysql-get-query-plan` tool for MySQL source ([#2123](https://github.com/googleapis/mcp-toolbox/issues/2123)) ([0641da0](https://github.com/googleapis/mcp-toolbox/commit/0641da0353857317113b2169e547ca69603ddfde))


### Bug Fixes

* **spanner:** Move list graphs validation to runtime ([#2154](https://github.com/googleapis/mcp-toolbox/issues/2154)) ([914b3ee](https://github.com/googleapis/mcp-toolbox/commit/914b3eefda40a650efe552d245369e007277dab5))


## [0.23.0](https://github.com/googleapis/mcp-toolbox/compare/v0.22.0...v0.23.0) (2025-12-11)


### ⚠ BREAKING CHANGES

* **serverless-spark:** add URLs to create batch tool outputs
* **serverless-spark:** add URLs to list_batches output
* **serverless-spark:** add Cloud Console and Logging URLs to get_batch
* **tools/postgres:** Add additional filter params for existing postgres tools ([#2033](https://github.com/googleapis/mcp-toolbox/issues/2033))

### Features

* **tools/postgres:** Add list-table-stats-tool to list table statistics. ([#2055](https://github.com/googleapis/mcp-toolbox/issues/2055)) ([78b02f0](https://github.com/googleapis/mcp-toolbox/commit/78b02f08c3cc3062943bb2f91cf60d5149c8d28d))
* **looker/tools:** Enhance dashboard creation with dashboard filters ([#2133](https://github.com/googleapis/mcp-toolbox/issues/2133)) ([285aa46](https://github.com/googleapis/mcp-toolbox/commit/285aa46b887d9acb2da8766e107bbf1ab75b8812))
* **serverless-spark:** Add Cloud Console and Logging URLs to get_batch ([e29c061](https://github.com/googleapis/mcp-toolbox/commit/e29c0616d6b9ecda2badcaf7b69614e511ac031b))
* **serverless-spark:** Add URLs to create batch tool outputs ([c6ccf4b](https://github.com/googleapis/mcp-toolbox/commit/c6ccf4bd87026484143a2d0f5527b2edab03b54a))
* **serverless-spark:** Add URLs to list_batches output ([5605eab](https://github.com/googleapis/mcp-toolbox/commit/5605eabd696696ade07f52431a28ef65c0fb1f77))
* **sources/mariadb:** Add MariaDB source and MySQL tools integration ([#1908](https://github.com/googleapis/mcp-toolbox/issues/1908)) ([3b40fea](https://github.com/googleapis/mcp-toolbox/commit/3b40fea25edae607e02c1e8fc2b0c957fa2c8e9a))
* **tools/postgres:** Add additional filter params for existing postgres tools ([#2033](https://github.com/googleapis/mcp-toolbox/issues/2033)) ([489117d](https://github.com/googleapis/mcp-toolbox/commit/489117d74711ac9260e7547163ca463eb45eeaa2))
* **tools/postgres:** Add list_pg_settings, list_database_stats tools for postgres ([#2030](https://github.com/googleapis/mcp-toolbox/issues/2030)) ([32367a4](https://github.com/googleapis/mcp-toolbox/commit/32367a472fae9653fed7f126428eba0252978bd5))
* **tools/postgres:** Add new postgres-list-roles tool ([#2038](https://github.com/googleapis/mcp-toolbox/issues/2038)) ([bea9705](https://github.com/googleapis/mcp-toolbox/commit/bea97054502cfa236aa10e2ebc8ff58eb00ad035))


### Bug Fixes

* List tables tools null fix ([#2107](https://github.com/googleapis/mcp-toolbox/issues/2107)) ([2b45266](https://github.com/googleapis/mcp-toolbox/commit/2b452665983154041d4cd0ed7d82532e4af682eb))
* **tools/mongodb:** Removed sortPayload and sortParams ([#1238](https://github.com/googleapis/mcp-toolbox/issues/1238)) ([c5a6daa](https://github.com/googleapis/mcp-toolbox/commit/c5a6daa7683d2f9be654300d977692c368e55e31))


### Miscellaneous Chores
* **looker:** Upgrade to latest go sdk ([#2159](https://github.com/googleapis/mcp-toolbox/issues/2159)) ([78e015d](https://github.com/googleapis/mcp-toolbox/commit/78e015d7dfd9cce7e2b444ed934da17eb355bc86))

## [0.22.0](https://github.com/googleapis/mcp-toolbox/compare/v0.21.0...v0.22.0) (2025-12-04)


### Features

* **tools/postgres:** Add allowed-origins flag ([#1984](https://github.com/googleapis/mcp-toolbox/issues/1984)) ([862868f](https://github.com/googleapis/mcp-toolbox/commit/862868f28476ea981575ce412faa7d6a03138f31))
* **tools/postgres:** Add list-query-stats and get-column-cardinality functions ([#1976](https://github.com/googleapis/mcp-toolbox/issues/1976)) ([9f76026](https://github.com/googleapis/mcp-toolbox/commit/9f760269253a8cc92a357e995c6993ccc4a0fb7b))
* **tools/spanner:** Add spanner list graphs to prebuiltconfigs ([#2056](https://github.com/googleapis/mcp-toolbox/issues/2056)) ([0e7fbf4](https://github.com/googleapis/mcp-toolbox/commit/0e7fbf465c488397aa9d8cab2e55165fff4eb53c))
* **prebuilt/cloud-sql:** Add clone instance tool for cloud sql ([#1845](https://github.com/googleapis/mcp-toolbox/issues/1845)) ([5e43630](https://github.com/googleapis/mcp-toolbox/commit/5e43630907aa2d7bc6818142483a33272eab060b))
* **serverless-spark:** Add create_pyspark_batch tool ([1bf0b51](https://github.com/googleapis/mcp-toolbox/commit/1bf0b51f033c956790be1577bf5310d0b17e9c12))
* **serverless-spark:** Add create_spark_batch tool ([17a9792](https://github.com/googleapis/mcp-toolbox/commit/17a979207dbc4fe70acd0ebda164d1a8d34c1ed3))
* Support alternate accessToken header name ([#1968](https://github.com/googleapis/mcp-toolbox/issues/1968)) ([18017d6](https://github.com/googleapis/mcp-toolbox/commit/18017d6545335a6fc1c472617101c35254d9a597))
* Support for annotations ([#2007](https://github.com/googleapis/mcp-toolbox/issues/2007)) ([ac21335](https://github.com/googleapis/mcp-toolbox/commit/ac21335f4e88ca52d954d7f8143a551a35661b94))
* **tool/mssql:** Set default host and port for MSSQL source ([#1943](https://github.com/googleapis/mcp-toolbox/issues/1943)) ([7a9cc63](https://github.com/googleapis/mcp-toolbox/commit/7a9cc633768d9ae9a7ff8230002da69d6a36ca86))
* **tools/cloudsqlpg:** Add CloudSQL PostgreSQL pre-check tool ([#1722](https://github.com/googleapis/mcp-toolbox/issues/1722)) ([8752e05](https://github.com/googleapis/mcp-toolbox/commit/8752e05ab6e98812d95673a6f1ff67e9a6ae48d2))
* **tools/postgres-list-publication-tables:** Add new postgres-list-publication-tables tool ([#1919](https://github.com/googleapis/mcp-toolbox/issues/1919)) ([f4b1f0a](https://github.com/googleapis/mcp-toolbox/commit/f4b1f0a68000ca2fc0325f55a1905705417c38a2))
* **tools/postgres-list-tablespaces:** Add new postgres-list-tablespaces tool ([#1934](https://github.com/googleapis/mcp-toolbox/issues/1934)) ([5ad7c61](https://github.com/googleapis/mcp-toolbox/commit/5ad7c6127b3e47504fc4afda0b7f3de1dff78b8b))
* **tools/spanner-list-graph:** Tool impl + docs + tests ([#1923](https://github.com/googleapis/mcp-toolbox/issues/1923)) ([a0f44d3](https://github.com/googleapis/mcp-toolbox/commit/a0f44d34ea3f044dd08501be616f70ddfd63ab45))


### Bug Fixes

* Add import for firebirdsql ([#2045](https://github.com/googleapis/mcp-toolbox/issues/2045)) ([fb7aae9](https://github.com/googleapis/mcp-toolbox/commit/fb7aae9d35b760d3471d8379642f835a0d84ec41))
* Correct FAQ to mention HTTP tools ([#2036](https://github.com/googleapis/mcp-toolbox/issues/2036)) ([7b44237](https://github.com/googleapis/mcp-toolbox/commit/7b44237d4a21bfbf8d3cebe4d32a15affa29584d))
* Format BigQuery numeric output as decimal strings ([#2084](https://github.com/googleapis/mcp-toolbox/issues/2084)) ([155bff8](https://github.com/googleapis/mcp-toolbox/commit/155bff80c1da4fae1e169e425fd82e1dc3373041))
* Set default annotations for tools in code if annotation not provided in yaml ([#2049](https://github.com/googleapis/mcp-toolbox/issues/2049)) ([565460c](https://github.com/googleapis/mcp-toolbox/commit/565460c4ea8953dbe80070a8e469f957c0f7a70c))
* **tools/alloydb-postgres-list-tables:** Exclude google_ml schema from list_tables ([#2046](https://github.com/googleapis/mcp-toolbox/issues/2046)) ([a03984c](https://github.com/googleapis/mcp-toolbox/commit/a03984cc15254c928f30085f8fa509ded6a79a0c))
* **tools/alloydbcreateuser:** Remove duplication of project praram ([#2028](https://github.com/googleapis/mcp-toolbox/issues/2028)) ([730ac6d](https://github.com/googleapis/mcp-toolbox/commit/730ac6d22805fd50b4a675b74c1865f4e7689e7c))
* **tools/mongodb:** Remove `required` tag from the `canonical` field ([#2099](https://github.com/googleapis/mcp-toolbox/issues/2099)) ([744214e](https://github.com/googleapis/mcp-toolbox/commit/744214e04cd12b11d166e6eb7da8ce4714904abc))

## [0.21.0](https://github.com/googleapis/mcp-toolbox/compare/v0.20.0...v0.21.0) (2025-11-19)


### ⚠ BREAKING CHANGES

* **tools/spanner-list-tables:** Unmarshal `object_details` json string into map to make response have nested json ([#1894](https://github.com/googleapis/mcp-toolbox/issues/1894)) ([446d62a](https://github.com/googleapis/mcp-toolbox/commit/446d62acd995d5128f52e9db254dd1c7138227c6))


### Features

* **tools/postgres:** Add `long_running_transactions`, `list_locks` and `replication_stats` tools ([#1751](https://github.com/googleapis/mcp-toolbox/issues/1751)) ([5abad5d](https://github.com/googleapis/mcp-toolbox/commit/5abad5d56c6cc5ba86adc5253b948bf8230fa830))


### Bug Fixes

* **tools/alloydbgetinstance:** Remove parameter duplication ([#1993](https://github.com/googleapis/mcp-toolbox/issues/1993)) ([0e269a1](https://github.com/googleapis/mcp-toolbox/commit/0e269a1d125eed16a51ead27db4398e6e48cb948))
* **tools:** Check for query execution error for pgxpool.Pool ([#1969](https://github.com/googleapis/mcp-toolbox/issues/1969)) ([2bff138](https://github.com/googleapis/mcp-toolbox/commit/2bff1384a3570ef46bc03ebebc507923af261987))


## [0.20.0](https://github.com/googleapis/mcp-toolbox/compare/v0.19.1...v0.20.0) (2025-11-14)


### Features

* Added prompt support for toolbox ([#1798](https://github.com/googleapis/mcp-toolbox/issues/1798)) ([cd56ea4](https://github.com/googleapis/mcp-toolbox/commit/cd56ea44fbdd149fcb92324e70ee36ac747635db))
* **source/alloydb, source/cloud-sql-postgres,source/cloud-sql-mysql,source/cloud-sql-mssql:** Use project from env for alloydb and cloud sql control plane tools ([#1588](https://github.com/googleapis/mcp-toolbox/issues/1588)) ([12bdd95](https://github.com/googleapis/mcp-toolbox/commit/12bdd954597e49d3ec6b247cc104584c5a4d1943))
* **source/mysql:** Set default host and port for MySQL source ([#1922](https://github.com/googleapis/mcp-toolbox/issues/1922)) ([2c228ef](https://github.com/googleapis/mcp-toolbox/commit/2c228ef4f2d4cb8dfc41e845466bfe3566d141a1))
* **source/Postgresql:** Set default host and port for Postgresql  source ([#1927](https://github.com/googleapis/mcp-toolbox/issues/1927)) ([7e6e88a](https://github.com/googleapis/mcp-toolbox/commit/7e6e88a21f2b9b60e0d645cdde33a95892d31a04))
* **tool/looker-generate-embed-url:** Adding generate embed url tool ([#1877](https://github.com/googleapis/mcp-toolbox/issues/1877)) ([ef63860](https://github.com/googleapis/mcp-toolbox/commit/ef63860559798fbad54c1051d9f53bce42d66464))
* **tools/postgres:** Add `list_triggers`, `database_overview` tools for postgres ([#1912](https://github.com/googleapis/mcp-toolbox/issues/1912)) ([a4c9287](https://github.com/googleapis/mcp-toolbox/commit/a4c9287aecf848faa98d973a9ce5b13fa309a58e))
* **tools/postgres:** Add list_indexes, list_sequences tools for postgres ([#1765](https://github.com/googleapis/mcp-toolbox/issues/1765)) ([897c63d](https://github.com/googleapis/mcp-toolbox/commit/897c63dcea43226262d2062088c59f2d1068fca7))

## [0.19.1](https://github.com/googleapis/mcp-toolbox/compare/v0.18.0...v0.19.1) (2025-11-07)


### ⚠ BREAKING CHANGES

* **tools/alloydbainl:** update AlloyDB AI NL statement order ([#1753](https://github.com/googleapis/mcp-toolbox/issues/1753))
* **tools/bigquery-analyze-contribution:** Add allowed dataset support ([#1675](https://github.com/googleapis/mcp-toolbox/issues/1675)) ([ef28e39](https://github.com/googleapis/mcp-toolbox/commit/ef28e39e90b21287ca8e69b99f4e792c78e9d31f))
* **tools/bigquery-get-dataset-info:** add allowed dataset support ([#1654](https://github.com/googleapis/mcp-toolbox/issues/1654))

### Features

* Support `excludeValues` for parameters ([#1818](https://github.com/googleapis/mcp-toolbox/issues/1818)) ([a8e98dc](https://github.com/googleapis/mcp-toolbox/commit/a8e98dc99d208e8b37a3bc4d1ab4749b5154ed36))
* **elasticsearch:** Add Elasticsearch source and tools ([#1109](https://github.com/googleapis/mcp-toolbox/issues/1109)) ([5367285](https://github.com/googleapis/mcp-toolbox/commit/5367285e91ddda99ae7261d8ed4b025f975d1453))
* **mindsdb:** Add MindsDB Source and Tools  ([#878](https://github.com/googleapis/mcp-toolbox/issues/878)) ([1b2cca9](https://github.com/googleapis/mcp-toolbox/commit/1b2cca9faa6f7bacbeb5d25634ce3bf61067de16))
* **cloud-healthcare:** Add support for healthcare source, tool and prebuilt config ([#1853](https://github.com/googleapis/mcp-toolbox/issues/1853)) ([1f833fb](https://github.com/googleapis/mcp-toolbox/commit/1f833fb1a124e23819ddfec476f2e63ef31dd22f))
* **singlestore:** Add SingleStore Source and Tools ([#1333](https://github.com/googleapis/mcp-toolbox/issues/1333)) ([40b9dba](https://github.com/googleapis/mcp-toolbox/commit/40b9dbab088add05a66995abb1c71a9345b8f7e5))
* **source/bigquery:** Add client cache for user-passed credentials ([#1119](https://github.com/googleapis/mcp-toolbox/issues/1119)) ([cf7012a](https://github.com/googleapis/mcp-toolbox/commit/cf7012a82bb5c77309da3a26e563a5015786aa69))
* **source/bigquery:** Add service account impersonation support for bigquery ([#1641](https://github.com/googleapis/mcp-toolbox/issues/1641)) ([e09d182](https://github.com/googleapis/mcp-toolbox/commit/e09d182f88bf697a169428f477aebc9f1741e35f))
* **tools/bigquery-analyze-contribution:** Add allowed dataset support ([#1675](https://github.com/googleapis/mcp-toolbox/issues/1675)) ([ef28e39](https://github.com/googleapis/mcp-toolbox/commit/ef28e39e90b21287ca8e69b99f4e792c78e9d31f))
* **tools/bigquery-get-dataset-info:** Add allowed dataset support ([#1654](https://github.com/googleapis/mcp-toolbox/issues/1654)) ([a2006ad](https://github.com/googleapis/mcp-toolbox/commit/a2006ad57718ebad3de5c6850e9c6a5a763808ec))
* **tools/looker-run-dashboard:** New `run_dashboard` tool ([#1858](https://github.com/googleapis/mcp-toolbox/issues/1858)) ([30857c2](https://github.com/googleapis/mcp-toolbox/commit/30857c2294bb14961d3be49e2c368c69ecf844ec))
* **tools/looker-run-look:** Modify run_look to show query origin ([#1860](https://github.com/googleapis/mcp-toolbox/issues/1860)) ([991e539](https://github.com/googleapis/mcp-toolbox/commit/991e539f9c7978fa618ada3179a0b656c33ff501))
* **tools/looker:** Tools to retrieve the connections, schemas, databases, and column metadata from a looker system. ([#1804](https://github.com/googleapis/mcp-toolbox/issues/1804)) ([d7d1b03](https://github.com/googleapis/mcp-toolbox/commit/d7d1b03f3b746ed748d67f67e833457d55c846ab))
* **tools/mongodb:** Make MongoDB tools' `filterParams` field optional ([#1614](https://github.com/googleapis/mcp-toolbox/issues/1614)) ([208ab92](https://github.com/googleapis/mcp-toolbox/commit/208ab92eb377b538a99330a415ecc18790b077b7))
* **tools/neo4j-execute-cypher:** Add dry_run parameter to validate Cypher queries ([#1769](https://github.com/googleapis/mcp-toolbox/issues/1769)) ([f475da6](https://github.com/googleapis/mcp-toolbox/commit/f475da63ce1b65387b503ac497eca47635452723))
* **tools/postgres-list-schemas:** Add new postgres-list-schemas tool ([#1741](https://github.com/googleapis/mcp-toolbox/issues/1741)) ([1a19cac](https://github.com/googleapis/mcp-toolbox/commit/1a19cac7cd89ed70291eb55e190370fe7b2c1aba))
* **tools/postgres-list-views:** Add new postgres-list-views tool ([#1709](https://github.com/googleapis/mcp-toolbox/issues/1709)) ([e8c7fe0](https://github.com/googleapis/mcp-toolbox/commit/e8c7fe0994fedcb9be78d461fab3c98cc6bd86b2))
* **tools/serverless-spark:** Add cancel-batch tool ([#1827](https://github.com/googleapis/mcp-toolbox/pull/1827))([2881683](https://github.com/googleapis/mcp-toolbox/commit/28816832265250de97d84e6ba38bf6c35e040796))
* **tools/serverless-spark:** Add get_batch tool ([#1783](https://github.com/googleapis/mcp-toolbox/pull/1783))([7ad1072](https://github.com/googleapis/mcp-toolbox/commit/7ad10720b4638324cd77d8aa410cbd1ccf0cc93f))
* **tools/serverless-spark:** Add serverless-spark source with list_batches tool ([#1690](https://github.com/googleapis/mcp-toolbox/pull/1690))([816dbce](https://github.com/googleapis/mcp-toolbox/commit/816dbce268392046e54767732bd31488c6e89bdb))


### Bug Fixes

* Bigquery execute_sql to assign values to array ([#1884](https://github.com/googleapis/mcp-toolbox/issues/1884)) ([559e2a2](https://github.com/googleapis/mcp-toolbox/commit/559e2a22e0db20bb947702e13140ce869b5865a7))
* **cloudmonitoring:** Populate `authRequired` in tool manifest ([#1800](https://github.com/googleapis/mcp-toolbox/issues/1800)) ([954152c](https://github.com/googleapis/mcp-toolbox/commit/954152c7928bf0da9be221e011e32f74bc4cebbc))
* Update debug logs statements ([#1828](https://github.com/googleapis/mcp-toolbox/issues/1828)) ([3cff915](https://github.com/googleapis/mcp-toolbox/commit/3cff915e22c3a5e4e296607f83ae6409b896c9a9))
* Instructions to quote filters that include commas ([#1794](https://github.com/googleapis/mcp-toolbox/issues/1794)) ([4b01720](https://github.com/googleapis/mcp-toolbox/commit/4b0172083c0dd4c71098d4e0ab5fa0b16ea0d830))
* **source/cloud-sql-mssql:** Remove `ipAddress` field ([#1822](https://github.com/googleapis/mcp-toolbox/issues/1822)) ([38d535d](https://github.com/googleapis/mcp-toolbox/commit/38d535de34cfedd6828a01d9dcd25daf1bad7306))
* **tools/alloydbainl:** AlloyDB AI NL execute_sql statement order ([#1753](https://github.com/googleapis/mcp-toolbox/issues/1753)) ([9723cad](https://github.com/googleapis/mcp-toolbox/commit/9723cadaa181a76d8fdda65a6254f6c887c3cf57))
* **tools/postgres-execute-sql:** Do not ignore SQL failure ([#1829](https://github.com/googleapis/mcp-toolbox/issues/1829)) ([8984287](https://github.com/googleapis/mcp-toolbox/commit/898428759c2a1a384bea8939605cf0914d129bec))


## [0.18.0](https://github.com/googleapis/mcp-toolbox/compare/v0.17.0...v0.18.0) (2025-10-23)


### Features

* Support `allowedValues`, `escape`, `minValue` and `maxValue` for parameters ([#1770](https://github.com/googleapis/mcp-toolbox/issues/1770)) ([eaf7740](https://github.com/googleapis/mcp-toolbox/commit/eaf77406fd386c12315d67eb685dc69e0415c516))
* **tools/looker:** Tools to allow the agent to retrieve, create, modify, and delete LookML project files. ([#1673](https://github.com/googleapis/mcp-toolbox/issues/1673)) ([089081f](https://github.com/googleapis/mcp-toolbox/commit/089081feb0e32f9eb65d00df5987392d413a4081))


### Bug Fixes

* **sources/mysql:** Escape mysql user agent ([#1707](https://github.com/googleapis/mcp-toolbox/issues/1707)) ([eeb694c](https://github.com/googleapis/mcp-toolbox/commit/eeb694c20facc40a38bfa67073c4cb1f3dd657ff))
* **sources/mysql:** Escape program_name for MySQL ([#1717](https://github.com/googleapis/mcp-toolbox/issues/1717)) ([02f7f8a](https://github.com/googleapis/mcp-toolbox/commit/02f7f8af979057efe99fd138cb1b958130355b68))
* **tools/http:** Allow 2xx status code on tool invocation ([#1761](https://github.com/googleapis/mcp-toolbox/issues/1761)) ([a06d0d8](https://github.com/googleapis/mcp-toolbox/commit/a06d0d8735fbec29bea97457248845a8c6b4aa3c))
* **tools/http:** Omit optional nil query parameters ([#1762](https://github.com/googleapis/mcp-toolbox/issues/1762)) ([bd16ba3](https://github.com/googleapis/mcp-toolbox/commit/bd16ba3921e6177065780e5f29870859b8e18e4f))
* **tools/looker:** Looker file content calls should not use url.QueryEscape ([#1758](https://github.com/googleapis/mcp-toolbox/issues/1758)) ([336de1b](https://github.com/googleapis/mcp-toolbox/commit/336de1bd04b869d322c0fd1f4667eb652159d791))


## [0.17.0](https://github.com/googleapis/mcp-toolbox/compare/v0.16.0...v0.17.0) (2025-10-10)


### ⚠ BREAKING CHANGES

* **tools/bigquery-get-table-info:** add allowed dataset support ([#1093](https://github.com/googleapis/mcp-toolbox/issues/1093))
* **tools/bigquery-list-dataset-ids:** add allowed datasets support ([#1573](https://github.com/googleapis/mcp-toolbox/issues/1573))

### Features

* Add configs and workflows for docs versioning ([#1611](https://github.com/googleapis/mcp-toolbox/issues/1611)) ([21ac98b](https://github.com/googleapis/mcp-toolbox/commit/21ac98bc065e95bde911d66185c67d8380891bf8))
* Add metadata in MCP Manifest for Toolbox auth ([#1395](https://github.com/googleapis/mcp-toolbox/issues/1395)) ([0b3dac4](https://github.com/googleapis/mcp-toolbox/commit/0b3dac41322f7aaa5a19df571686fa8fd4338ca5))
* Add program name to MySQL connections ([#1617](https://github.com/googleapis/mcp-toolbox/issues/1617)) ([c4a22b8](https://github.com/googleapis/mcp-toolbox/commit/c4a22b8d3bd0307325215ebd2f30ba37927cd37e))
* **source/bigquery:** Add optional write mode config ([#1157](https://github.com/googleapis/mcp-toolbox/issues/1157)) ([63adc78](https://github.com/googleapis/mcp-toolbox/commit/63adc78beae949dfe5e300c50e5ceef073e9652c))
* **sources/alloydb,cloudsqlpg,cloudsqlmysql,cloudsqlmssql:** Support PSC connection ([#1686](https://github.com/googleapis/mcp-toolbox/issues/1686)) ([9d2bf79](https://github.com/googleapis/mcp-toolbox/commit/9d2bf79becfda104ef77f34b8d4b22cbedbc4bf3))
* **sources/mssql:** Add app name to MSSQL ([#1620](https://github.com/googleapis/mcp-toolbox/issues/1620)) ([1536d1f](https://github.com/googleapis/mcp-toolbox/commit/1536d1fdabb9d7f73dbdeebeb05a83d9a3b78e1c))
* **sources/oracle:** Add Oracle Source and Tool ([#1456](https://github.com/googleapis/mcp-toolbox/issues/1456)) ([3a19a50](https://github.com/googleapis/mcp-toolbox/commit/3a19a50ff211e33429de1d05338d353359a52987))
* **sources/oracle:** Switch Oracle driver from godror to go-ora ([#1685](https://github.com/googleapis/mcp-toolbox/issues/1685)) ([8faf376](https://github.com/googleapis/mcp-toolbox/commit/8faf37667e371b4ed88ebb892e8784b67611ba64))
* **tool/bigquery-list-dataset-ids:** Add allowed datasets support ([#1573](https://github.com/googleapis/mcp-toolbox/issues/1573)) ([1a44c67](https://github.com/googleapis/mcp-toolbox/commit/1a44c671ec593e764a2d2f67f70a98ceec20a168))
* **tools/bigquery-get-table-info:** Add allowed dataset support ([#1093](https://github.com/googleapis/mcp-toolbox/issues/1093)) ([acb205c](https://github.com/googleapis/mcp-toolbox/commit/acb205ca4761d59ce97b804827230978c8c98ede))
* **tools/dataform:** Add dataform compile tool ([#1470](https://github.com/googleapis/mcp-toolbox/issues/1470)) ([3be9b7b](https://github.com/googleapis/mcp-toolbox/commit/3be9b7b3bdf112fe7303706e56e9f39935cde661))
* **tools/looker:** Add support for pulse, vacuum and analyze audit and performance functions on a Looker instance ([#1581](https://github.com/googleapis/mcp-toolbox/issues/1581)) ([5aed4e1](https://github.com/googleapis/mcp-toolbox/commit/5aed4e136d0091731d2ded10ec076ee789e1987c))
* **tools/looker:** Enable access to the Conversational Analytics API for Looker ([#1596](https://github.com/googleapis/mcp-toolbox/issues/1596)) ([2d5a93e](https://github.com/googleapis/mcp-toolbox/commit/2d5a93e312990c8a9f3170c7e9c655f97cf11712))


### Bug Fixes

* Added google_ml_integration extension to use alloydb ai-nl support api ([#1445](https://github.com/googleapis/mcp-toolbox/issues/1445)) ([dbc477a](https://github.com/googleapis/mcp-toolbox/commit/dbc477ab0f832495cf51f73ea16ae363472d6eed))
* Fix broken links ([#1625](https://github.com/googleapis/mcp-toolbox/issues/1625)) ([36c6584](https://github.com/googleapis/mcp-toolbox/commit/36c658472ccdeb6cddd8a4452a8b3438aeb0a744))
* Remove duplicated build type in Dockerfile ([#1598](https://github.com/googleapis/mcp-toolbox/issues/1598)) ([b43c945](https://github.com/googleapis/mcp-toolbox/commit/b43c94575d86aa65b0528d59f9b41d30b439fee5))
* **source/bigquery:** Allowed datasets project id issue with client oauth ([#1663](https://github.com/googleapis/mcp-toolbox/issues/1663)) ([f4cf486](https://github.com/googleapis/mcp-toolbox/commit/f4cf486fa929299fef076cf71689776e5dec19c1))
* **sources/looker:** Allow Looker to be configured without setting a Client Id or Secret ([#1496](https://github.com/googleapis/mcp-toolbox/issues/1496)) ([67d8221](https://github.com/googleapis/mcp-toolbox/commit/67d8221a2e780df54a81f0f7e8f7e41e4bf1a82e))
* **tools/looker:** Refactor run-inline-query logic to helper function ([#1497](https://github.com/googleapis/mcp-toolbox/issues/1497)) ([62af39d](https://github.com/googleapis/mcp-toolbox/commit/62af39d751443eb758586663969b162c868a233f))
* **tools/mysql-list-tables:** Update sql query to resolve subquery scope error ([#1629](https://github.com/googleapis/mcp-toolbox/issues/1629)) ([94e19d8](https://github.com/googleapis/mcp-toolbox/commit/94e19d87e54e831b80eb766572e48bc7370305d8))

## [0.16.0](https://github.com/googleapis/mcp-toolbox/compare/v0.15.0...v0.16.0) (2025-09-25)


### ⚠ BREAKING CHANGES

* **tool/bigquery-execute-sql:** add allowed datasets support ([#1443](https://github.com/googleapis/mcp-toolbox/issues/1443))
* **tool/bigquery-forecast:** add allowed datasets support ([#1412](https://github.com/googleapis/mcp-toolbox/issues/1412))

### Features

* **cassandra:** Add Cassandra Source and Tool ([#1012](https://github.com/googleapis/mcp-toolbox/issues/1012)) ([6e42053](https://github.com/googleapis/mcp-toolbox/commit/6e420534ee894da4a8d226acb6cdb63d0d5d9ce5))
* **sources/postgres:** Add application_name ([#1504](https://github.com/googleapis/mcp-toolbox/issues/1504)) ([72a2366](https://github.com/googleapis/mcp-toolbox/commit/72a2366b28870aa6f81c4f890f4770ec5ecffdba))
* **tool/bigquery-execute-sql:** Add allowed datasets support ([#1443](https://github.com/googleapis/mcp-toolbox/issues/1443)) ([9501ebb](https://github.com/googleapis/mcp-toolbox/commit/9501ebbdbcba871b98663185c690308dda1729b5))
* **tool/bigquery-forecast:** Add allowed datasets support ([#1412](https://github.com/googleapis/mcp-toolbox/issues/1412)) ([88bac7e](https://github.com/googleapis/mcp-toolbox/commit/88bac7e36f5ebb6ad18773bff30b85ef678431e7))
* **tools/clickhouse-list-tables:** Add list-tables tool ([#1446](https://github.com/googleapis/mcp-toolbox/issues/1446)) ([69a3caf](https://github.com/googleapis/mcp-toolbox/commit/69a3cafabec5a40e2776d71de3587c0d16c722a2))


### Bug Fixes

* **tool/mongodb-find:** Fix find tool `limit` field ([#1570](https://github.com/googleapis/mcp-toolbox/issues/1570)) ([4166bf7](https://github.com/googleapis/mcp-toolbox/commit/4166bf7ab85732f64b877d5f20235057df919049))
* **tools/mongodb:** Concat filter params only once in mongodb update tools ([#1545](https://github.com/googleapis/mcp-toolbox/issues/1545)) ([295f9dc](https://github.com/googleapis/mcp-toolbox/commit/295f9dc41a43f0a4bdbd99e465bf2be01249084e))

## [0.15.0](https://github.com/googleapis/mcp-toolbox/compare/v0.14.0...v0.15.0) (2025-09-18)


### ⚠ BREAKING CHANGES

* **prebuilt:** update prebuilt tool names to use consistent guidance ([#1421](https://github.com/googleapis/mcp-toolbox/issues/1421))
* **tools/alloydb-wait-for-operation:** Add `alloydb-admin` source to `alloydb-wait-for-operation` tool ([#1449](https://github.com/googleapis/mcp-toolbox/issues/1449))

### Features

* Add AlloyDB admin source ([#1369](https://github.com/googleapis/mcp-toolbox/issues/1369)) ([33beb71](https://github.com/googleapis/mcp-toolbox/commit/33beb7187d2e0f968fc949a00c780073d1bc7cdd))
* Add Cloud monitoring source and tool ([#1311](https://github.com/googleapis/mcp-toolbox/issues/1311)) ([d661f53](https://github.com/googleapis/mcp-toolbox/commit/d661f5343f2ad28fbf0481db16440aec823eece6))
* Add YugabyteDB Source and Tool ([#732](https://github.com/googleapis/mcp-toolbox/issues/732)) ([664711f](https://github.com/googleapis/mcp-toolbox/commit/664711f4b35409bd1c57af92f625b70a0dc9a4e6))
* **prebuilt:** Update default values for prebuilt tools ([#1355](https://github.com/googleapis/mcp-toolbox/issues/1355)) ([70e832b](https://github.com/googleapis/mcp-toolbox/commit/70e832bd08a98a95b925e590f31c8d3f2d8b6aa0))
* **prebuilt/cloud-sql:** Add  list instances tool for cloudsql ([#1310](https://github.com/googleapis/mcp-toolbox/issues/1310)) ([0171228](https://github.com/googleapis/mcp-toolbox/commit/01712284b480774ffa68930affae290ee2e3fcfd))
* **prebuilt/cloud-sql:** Add cloud sql create database tool. ([#1453](https://github.com/googleapis/mcp-toolbox/issues/1453)) ([a1bc044](https://github.com/googleapis/mcp-toolbox/commit/a1bc04477b0f822ffaab039098682f1776b8a472))
* **prebuilt/cloud-sql:** Add `cloud-sql-get-instances` tool ([#1383](https://github.com/googleapis/mcp-toolbox/issues/1383)) ([77919c7](https://github.com/googleapis/mcp-toolbox/commit/77919c7d8e4aac16eeb703c0cc61ca774dc4f94e))
* **prebuilt/cloud-sql:** Add create user tool for cloud sql ([#1406](https://github.com/googleapis/mcp-toolbox/issues/1406)) ([3a6b517](https://github.com/googleapis/mcp-toolbox/commit/3a6b51752f077b225b8c2e2e7308a69a68eec3c0))
* **prebuilt/cloud-sql:** Add list databases tool for cloud sql ([#1454](https://github.com/googleapis/mcp-toolbox/issues/1454)) ([e6a6c61](https://github.com/googleapis/mcp-toolbox/commit/e6a6c615d5480e8930ad173d44d243f5bd99eebc))
* **prebuilt/cloud-sql:** Package cloud sql tools ([#1455](https://github.com/googleapis/mcp-toolbox/issues/1455)) ([bf6266b](https://github.com/googleapis/mcp-toolbox/commit/bf6266ba1131bd1c5829ac112a8c45c8a5919fea))
* **prebuilt/cloud-sql-mssql:** Add create instance tool for mssql ([#1440](https://github.com/googleapis/mcp-toolbox/issues/1440)) ([b176523](https://github.com/googleapis/mcp-toolbox/commit/b17652309d8a02b1f20c6c576b1617b23c8e481f))
* **prebuilt/cloud-sql-mysql:** Add create instance tool for Cloud SQL MySQL ([#1434](https://github.com/googleapis/mcp-toolbox/issues/1434)) ([15b628d](https://github.com/googleapis/mcp-toolbox/commit/15b628d2d2feb2ecdd418394b9265a6c77c77f6d))
* **prebuilt/cloud-sql-mysql:** Add env var support for IP Type ([#1232](https://github.com/googleapis/mcp-toolbox/issues/1232)) ([#1347](https://github.com/googleapis/mcp-toolbox/issues/1347)) ([0cd3f16](https://github.com/googleapis/mcp-toolbox/commit/0cd3f16f877f426b45e35625ba0af03789459591))
* **prebuilt/cloudsqlpg:** Add cloud sql pg create instance tool ([#1403](https://github.com/googleapis/mcp-toolbox/issues/1403)) ([d302499](https://github.com/googleapis/mcp-toolbox/commit/d30249961b5a2ddc2c3809b481085d1ca034ead0))
* **prebuilt/mysql:** Add a new tool to show query plan of a given query in MySQL ([#1474](https://github.com/googleapis/mcp-toolbox/issues/1474)) ([1a42e05](https://github.com/googleapis/mcp-toolbox/commit/1a42e05675645ac4f1b89edef7a71ac61b637a76))
* **prebuilt/mysql:** Add  `queryParams` field in MySQL prebuilt config ([#1318](https://github.com/googleapis/mcp-toolbox/issues/1318)) ([4b32c2a](https://github.com/googleapis/mcp-toolbox/commit/4b32c2a7701ce5ccc56d019055283e73e7046372))
* **prebuilt/neo4j:** Add prebuiltconfig support for neo4j ([#1352](https://github.com/googleapis/mcp-toolbox/issues/1352)) ([f819e26](https://github.com/googleapis/mcp-toolbox/commit/f819e2644315a589ec283494f244c1b8407cae59))
* **prebuilt/observability:** Add cloud sql observability tools ([#1425](https://github.com/googleapis/mcp-toolbox/issues/1425)) ([236be89](https://github.com/googleapis/mcp-toolbox/commit/236be89961fe423c1ec992d3d1f699f77a6e5b29))
* **prebuilt/postgres:** Add postgres prebuilt tools ([#1473](https://github.com/googleapis/mcp-toolbox/issues/1473)) ([edca9dc](https://github.com/googleapis/mcp-toolbox/commit/edca9dc7d772baf1a234485020fa69d76f71bfcc))
* **prebuilt/sqlite:** Prebuilt tools for the sqlite. ([#1227](https://github.com/googleapis/mcp-toolbox/issues/1227)) ([681c2b4](https://github.com/googleapis/mcp-toolbox/commit/681c2b4f3a65837d972c138c623c08fb6b1f1785))
* **source/alloydb-admin:** Add user agent and attach alloydb api in `alloydb-admin` source ([#1448](https://github.com/googleapis/mcp-toolbox/issues/1448)) ([9710014](https://github.com/googleapis/mcp-toolbox/commit/971001400f25796784f8aeb3ec5cb1a2df2e4c69))
* **source/bigquery:** Add support for datasets selection ([#1313](https://github.com/googleapis/mcp-toolbox/issues/1313)) ([aa39724](https://github.com/googleapis/mcp-toolbox/commit/aa3972470fd0f6f5901c5d85dd05f1e2ae973e7b))
* **source/cloud-monitoring:** Add support for user agent in cloud monitoring source ([#1472](https://github.com/googleapis/mcp-toolbox/issues/1472)) ([92680b1](https://github.com/googleapis/mcp-toolbox/commit/92680b18d6159300ae66f80ddb4c6bf0547d45a1))
* **source/cloud-sql-admin:** Add User agent and attach sqldmin in `cloud-sql-admin` source. ([#1441](https://github.com/googleapis/mcp-toolbox/issues/1441)) ([56b6574](https://github.com/googleapis/mcp-toolbox/commit/56b6574fc2c506c7c7df7f2a25686e3e4aae0e8a))
* **source/cloudsqladmin:** Add cloud sql admin source ([#1408](https://github.com/googleapis/mcp-toolbox/issues/1408)) ([4f46782](https://github.com/googleapis/mcp-toolbox/commit/4f4678292762507494515ce61188cd0310805c40))
* **tool/cloudsql:** Add cloud sql wait for operation tool with exponential backoff ([#1306](https://github.com/googleapis/mcp-toolbox/issues/1306)) ([3aef2bb](https://github.com/googleapis/mcp-toolbox/commit/3aef2bb7be8274bb4718739faeaa5f97b50dbf19))
* **tools/alloydb-create-cluster:** Add custom tool kind for AlloyDB create cluster ([#1331](https://github.com/googleapis/mcp-toolbox/issues/1331)) ([76bb876](https://github.com/googleapis/mcp-toolbox/commit/76bb876d546780908c1a69ef3b1a92781af28a3b))
* **tools/alloydb-create-instance:** Add new custom tool kind for AlloyDB ([#1379](https://github.com/googleapis/mcp-toolbox/issues/1379)) ([091cd9a](https://github.com/googleapis/mcp-toolbox/commit/091cd9aa1aabe1cb3de2ce2be5c707a8f77ad647))
* **tools/alloydb-create-user:** Add new custom tool kind for AlloyDB create user ([#1380](https://github.com/googleapis/mcp-toolbox/issues/1380)) ([ab3fd26](https://github.com/googleapis/mcp-toolbox/commit/ab3fd261af373dcdaf4555292c63d7095d7a02df))
* **tools/alloydb-get-cluster:** Add new tool for AlloyDB ([#1420](https://github.com/googleapis/mcp-toolbox/issues/1420)) ([c181dab](https://github.com/googleapis/mcp-toolbox/commit/c181dabc91bdc1c24c89a3c7bba0049d9af4cf2b))
* **tools/alloydb-get-instance:** Add new for AlloyDB ([#1435](https://github.com/googleapis/mcp-toolbox/issues/1435)) ([f2d9e3b](https://github.com/googleapis/mcp-toolbox/commit/f2d9e3b57963082f0db70880d5c02b1cbe3eb75d))
* **tools/alloydb-get-user:** Add new tool for AlloyDB ([#1436](https://github.com/googleapis/mcp-toolbox/issues/1436)) ([677254e](https://github.com/googleapis/mcp-toolbox/commit/677254e6d9c532fa9f0fb0b0e4062446640ab75f))
* **tools/alloydb-list-cluster:** Add custom tool kind for AlloyDB ([#1319](https://github.com/googleapis/mcp-toolbox/issues/1319)) ([d4a9eb0](https://github.com/googleapis/mcp-toolbox/commit/d4a9eb0ce217c7969aff61868e53c7dc7757d28d))
* **tools/alloydb-list-instances:** Add custom tool kind for AlloyDB ([#1357](https://github.com/googleapis/mcp-toolbox/issues/1357)) ([93c1b30](https://github.com/googleapis/mcp-toolbox/commit/93c1b30fced113d6721ade9fdcfb92b5ed6c0ad6))
* **tools/alloydb-list-users:** Add new custom tool kind for AlloyDB ([#1377](https://github.com/googleapis/mcp-toolbox/issues/1377)) ([3a8a65c](https://github.com/googleapis/mcp-toolbox/commit/3a8a65ceaa92368563e237087c4a38ed7c0d3fd5))
* **tools/bigquery-analyze-contribution:** Add analyze contribution tool ([#1223](https://github.com/googleapis/mcp-toolbox/issues/1223)) ([81d239b](https://github.com/googleapis/mcp-toolbox/commit/81d239b053a6978250878a6809905dcc9424909e))
* **tools/bigquery-conversational-analytics:** Add allowed datasets support ([#1411](https://github.com/googleapis/mcp-toolbox/issues/1411)) ([345bd6a](https://github.com/googleapis/mcp-toolbox/commit/345bd6af520bb9ce8a43834951e68fea7bbe6a02))
* **tools/bigquery-search-catalog:** Add new tool to BigQuery ([#1382](https://github.com/googleapis/mcp-toolbox/issues/1382)) ([bffb39d](https://github.com/googleapis/mcp-toolbox/commit/bffb39dea3cc946a1e611e3523241443b1e4f047))
* **tools/bigquery:** Add `useClientOAuth` to BigQuery prebuilt source config ([#1431](https://github.com/googleapis/mcp-toolbox/issues/1431)) ([fe2999a](https://github.com/googleapis/mcp-toolbox/commit/fe2999a691ac92b2bf35cb7cfd504df2f3ce84b3))
* **tools/clickhouse-list-databases:** Add `list-databases` tool to clickhouse source ([#1274](https://github.com/googleapis/mcp-toolbox/issues/1274)) ([e515d92](https://github.com/googleapis/mcp-toolbox/commit/e515d9254f3b8e89f89322d490eb3cedce85d2bb))
* **tools/firestore-get-rules:** Add `databaseId` to the Firestore source and `firestore-get-rules` tool ([#1505](https://github.com/googleapis/mcp-toolbox/issues/1505)) ([7450482](https://github.com/googleapis/mcp-toolbox/commit/7450482bb2479eab7d1c8f0d40755a8d11aa3b26))
* **tools/firestore:** Add `firestore-query` tool ([#1305](https://github.com/googleapis/mcp-toolbox/issues/1305)) ([cce602f](https://github.com/googleapis/mcp-toolbox/commit/cce602f28097353f6a3017cec1fa5f75283f111d))
* **tools/looker:** Query tracking for MCP Toolbox in Looker System Activity views ([#1410](https://github.com/googleapis/mcp-toolbox/issues/1410)) ([2036c8e](https://github.com/googleapis/mcp-toolbox/commit/2036c8efd2fb9edc26df599629d3131c6c367f4b))
* **tools/mssql-list-tables:** Add new tool for sql server ([#1433](https://github.com/googleapis/mcp-toolbox/issues/1433)) ([b036047](https://github.com/googleapis/mcp-toolbox/commit/b036047a21f63265c9d9637ac1a671792c9c2e80))
* **tools/mysql-list-active-queries:** Add a new tool to list ongoing queries in a MySQL instance ([#1471](https://github.com/googleapis/mcp-toolbox/issues/1471)) ([ed54cd6](https://github.com/googleapis/mcp-toolbox/commit/ed54cd6cfd17a3bdd84025d4eb8264763da36a98))
* **tools/mysql-list-table-fragmentation:** Add a new tool to list table fragmentation in a MySQL instance ([#1479](https://github.com/googleapis/mcp-toolbox/issues/1479)) ([fe651d8](https://github.com/googleapis/mcp-toolbox/commit/fe651d822f88832833e869ec049c6c084eae7e51))
* **tools/mysql-list-tables-missing-index:** Add a new tool to list tables that do not have primary or unique keys in a MySQL instance ([#1493](https://github.com/googleapis/mcp-toolbox/issues/1493)) ([9eb821a](https://github.com/googleapis/mcp-toolbox/commit/9eb821a6dca408ba993f904aa42b5b4f70674ba7))
* **tools/mysql-list-tables:** Add new tool for MySQL ([#1287](https://github.com/googleapis/mcp-toolbox/issues/1287)) ([6c8460b](https://github.com/googleapis/mcp-toolbox/commit/6c8460b0e507315d407c91ba1c821f4820cc1620))
* **tools/postgres-list-active-queries:** Add new `postgres-list-active-queries` tool ([#1400](https://github.com/googleapis/mcp-toolbox/issues/1400)) ([b2b06c7](https://github.com/googleapis/mcp-toolbox/commit/b2b06c72c29fd99a0c7118b85e6f7bcf6853d173))
* **tools/postgres-list-tables:** Add new tool to postgres source ([#1284](https://github.com/googleapis/mcp-toolbox/issues/1284)) ([71f360d](https://github.com/googleapis/mcp-toolbox/commit/71f360d31522f429a646b705ce7d1d11dac4cf68))
* **tools/spanner-list-tables:** Add new tool `spanner-list-tables` ([#1404](https://github.com/googleapis/mcp-toolbox/issues/1404)) ([7d384dc](https://github.com/googleapis/mcp-toolbox/commit/7d384dc28f8c37dddc2f6cefc0bbeb4c201e3167))


### Bug Fixes

* **bigquery:** Add `Bearer` parsing to auth token ([#1386](https://github.com/googleapis/mcp-toolbox/issues/1386)) ([b5f9780](https://github.com/googleapis/mcp-toolbox/commit/b5f9780a59e15eca2591dee32f5da42435e03039))
* **source/alloydb-admin, source/cloudsql-admin:** Post append new user agent ([#1494](https://github.com/googleapis/mcp-toolbox/issues/1494)) ([30f1d3a](https://github.com/googleapis/mcp-toolbox/commit/30f1d3a983aa317f1e1a98f9fe753005b56c52bd))
* **tools/alloydb:** Update parameter names and set default description for AlloyDB control plane tools ([#1468](https://github.com/googleapis/mcp-toolbox/issues/1468)) ([6c140d7](https://github.com/googleapis/mcp-toolbox/commit/6c140d718a66b45c7ec2d5a267331adb7680f689))
* **tools/bigquery-conversational-analytics:** Fix authentication scope error in Cloud Run ([#1381](https://github.com/googleapis/mcp-toolbox/issues/1381)) ([80b7488](https://github.com/googleapis/mcp-toolbox/commit/80b7488ad248ab1d98ee6713e1f6737f67f6754b))
* **tools/mysql-list-tables:** Update `mysql-list-tables` table_names parameter with default value ([#1439](https://github.com/googleapis/mcp-toolbox/issues/1439)) ([da24661](https://github.com/googleapis/mcp-toolbox/commit/da246610e105df10a9dc1bce19fa35d408c039f3))
* **tools/neo4j:** Implement value conversion for Neo4j types to JSON-compatible ([#1428](https://github.com/googleapis/mcp-toolbox/issues/1428)) ([4babc4e](https://github.com/googleapis/mcp-toolbox/commit/4babc4e11b3b64db8d8c9d6b65e47744f5174f7f))

## [0.14.0](https://github.com/googleapis/mcp-toolbox/compare/v0.13.0...v0.14.0) (2025-09-05)


### ⚠ BREAKING CHANGES

* **bigquery:** Move `useClientOAuth` config from tool to source ([#1279](https://github.com/googleapis/mcp-toolbox/issues/1279)) ([8d20a48](https://github.com/googleapis/mcp-toolbox/commit/8d20a48f13bcda853d41bdf80a162de12b076d1b))
* **tools/bigquerysql:** remove `useClientOAuth` from tools config ([#1312](https://github.com/googleapis/mcp-toolbox/issues/1312))

### Features

* **clickhouse:** Add ClickHouse Source and Tools ([#1088](https://github.com/googleapis/mcp-toolbox/issues/1088)) ([75a04a5](https://github.com/googleapis/mcp-toolbox/commit/75a04a55dd2259bed72fe95119a7a51a906c0b21))
* **prebuilt/alloydb-postgres:** Support ipType and IAM users ([#1324](https://github.com/googleapis/mcp-toolbox/issues/1324)) ([0b2121e](https://github.com/googleapis/mcp-toolbox/commit/0b2121ea72eb81348dcd9c740a62ccd32e71fe37))
* **server/mcp:** Support toolbox auth in mcp ([#1140](https://github.com/googleapis/mcp-toolbox/issues/1140)) ([ca353e0](https://github.com/googleapis/mcp-toolbox/commit/ca353e0b66fedc00e9110df57db18632aef49018))
* **source/mysql:** Support `queryParams` in MySQL source ([#1299](https://github.com/googleapis/mcp-toolbox/issues/1299)) ([3ae2526](https://github.com/googleapis/mcp-toolbox/commit/3ae2526e0fe36b57b05a9b54f1d99f3fc68d9657))
* **tools/bigquery:** Support end-user credential passthrough on multiple BQ tools ([#1314](https://github.com/googleapis/mcp-toolbox/issues/1314)) ([88f4b30](https://github.com/googleapis/mcp-toolbox/commit/88f4b3028df3b6a400936cdf8a035bf55021924c))
* **tools/looker:** Add description for looker-get-models tool ([#1266](https://github.com/googleapis/mcp-toolbox/issues/1266)) ([89af3a4](https://github.com/googleapis/mcp-toolbox/commit/89af3a4ca332f029615b2a739d1f6cd50519638d))
* **tools/looker:** Authenticate via end user credentials ([#1257](https://github.com/googleapis/mcp-toolbox/issues/1257)) ([8755e3d](https://github.com/googleapis/mcp-toolbox/commit/8755e3db3476abb35629b3cca9c78db7366757a4))
* **tools/looker:** Report field suggestions to agent ([#1267](https://github.com/googleapis/mcp-toolbox/issues/1267)) ([2cad82e](https://github.com/googleapis/mcp-toolbox/commit/2cad82e5107566dd6c9b75e34e9976af63af0bb5))


### Bug Fixes

* Do not print usage on runtime error ([#1315](https://github.com/googleapis/mcp-toolbox/issues/1315)) ([afba7a5](https://github.com/googleapis/mcp-toolbox/commit/afba7a57cdd4fe7c1b0741dbf8f8c78b14a68089))
* Update env var to allow empty string ([#1260](https://github.com/googleapis/mcp-toolbox/issues/1260)) ([03aa9fa](https://github.com/googleapis/mcp-toolbox/commit/03aa9fabacda06f860c9f178485126bddb7d5782))
* **tools/firestore:** Add document/collection path validation ([#1229](https://github.com/googleapis/mcp-toolbox/issues/1229)) ([14c2249](https://github.com/googleapis/mcp-toolbox/commit/14c224939a2f9bb349fa00a7d5227877198530c2))
* **tools/looker-get-dashboards:** Fix Looker client OAuth check ([#1338](https://github.com/googleapis/mcp-toolbox/issues/1338)) ([36225aa](https://github.com/googleapis/mcp-toolbox/commit/36225aa6db7f8426ad87930866530fde4e9bf0cd))
* **tools/oceanbase:** Fix encoded text with mysql driver ([#1283](https://github.com/googleapis/mcp-toolbox/issues/1283)) ([d16f89f](https://github.com/googleapis/mcp-toolbox/commit/d16f89fbb6e49c03998f114ef7dc2b584b5e4967)), closes [#1161](https://github.com/googleapis/mcp-toolbox/issues/1161)

## [0.13.0](https://github.com/googleapis/mcp-toolbox/compare/v0.12.0...v0.13.0) (2025-08-27)

### ⚠ BREAKING CHANGES

* **prebuilt/alloydb:** Add bearer token support for alloydb-wait-for-operation ([#1183](https://github.com/googleapis/mcp-toolbox/issues/1183))


### Features

* Add capability to set default for environment variable in config ([#1248](https://github.com/googleapis/mcp-toolbox/issues/1248)) ([5bcd52e](https://github.com/googleapis/mcp-toolbox/commit/5bcd52e7dcd0773ded723585f4abe29d044e1540))
* **firebird:** Add Firebird SQL 2.5+ source and tool ([#1011](https://github.com/googleapis/mcp-toolbox/issues/1011)) ([4f6b806](https://github.com/googleapis/mcp-toolbox/commit/4f6b806de947efc4e12bdb50dff7781aedb7b966))
* **oceanbase:** Add Oceanbase source and tool  ([#895](https://github.com/googleapis/mcp-toolbox/issues/895)) ([6fc4982](https://github.com/googleapis/mcp-toolbox/commit/6fc49826d43f46c84028e752ebebddf3d94b3d13))
* **server/mcp:** Support `ping` mechanism ([#1178](https://github.com/googleapis/mcp-toolbox/issues/1178)) ([5dcc66c](https://github.com/googleapis/mcp-toolbox/commit/5dcc66c84fa72c75ec50a9ac5198018212ec2979))
* **server:** Fail-fast on environment variable substitution ([#1177](https://github.com/googleapis/mcp-toolbox/issues/1177)) ([212aaba](https://github.com/googleapis/mcp-toolbox/commit/212aaba74c8b431de8a5f7b9822a0af4afcaaa0e))
* **server:** Implement Tool call auth error propagation ([#1235](https://github.com/googleapis/mcp-toolbox/issues/1235)) ([b94a021](https://github.com/googleapis/mcp-toolbox/commit/b94a021ca11c6637cf8038449483b5e75f2012b3))
* **sources/bigquery:** Add support for user-credential passthrough  ([#1067](https://github.com/googleapis/mcp-toolbox/issues/1067)) ([650e2e2](https://github.com/googleapis/mcp-toolbox/commit/650e2e26f51bff75ce66343f64944d0a89a58b69))
* **tool/looker:** Add support for `description` field in looker tool ([#1199](https://github.com/googleapis/mcp-toolbox/issues/1199)) ([97f0dd2](https://github.com/googleapis/mcp-toolbox/commit/97f0dd2acf26caf28ecad65abea8779c196a27f1))
* **tools/bigquery-ask-data-insights:** Add bigquery `ask-data-insights` tool ([#932](https://github.com/googleapis/mcp-toolbox/issues/932)) ([7651357](https://github.com/googleapis/mcp-toolbox/commit/7651357d424a2b6656d8b6818cebc5c8a86ed053))
* **tools/bigquery-forecast:** Add bigqueryforecast tool ([#1148](https://github.com/googleapis/mcp-toolbox/issues/1148)) ([2ad0ccf](https://github.com/googleapis/mcp-toolbox/commit/2ad0ccf83df542340087742468d6762f81eedee6))
* **tools/firestore-add-documents:** Add firestore-add-documents tool ([#1107](https://github.com/googleapis/mcp-toolbox/issues/1107)) ([ee4a70a](https://github.com/googleapis/mcp-toolbox/commit/ee4a70a0e82b346b07b5b4c60dfa060da2273f50))
* **tools/firestore-update-document:** Add firestore-update-document tool ([#1191](https://github.com/googleapis/mcp-toolbox/issues/1191)) ([0010123](https://github.com/googleapis/mcp-toolbox/commit/00101232a39c70288aac5715649c184858d351e3))
* **tools/looker:** Control over whether hidden objects are surfaced ([#1222](https://github.com/googleapis/mcp-toolbox/issues/1222)) ([bc91559](https://github.com/googleapis/mcp-toolbox/commit/bc91559cc4e5b20385b84cc562b624fabf7e47a8))
* **trino:** Add Trino source and tools ([#948](https://github.com/googleapis/mcp-toolbox/issues/948)) ([7dd123b](https://github.com/googleapis/mcp-toolbox/commit/7dd123b3d76b8eb2b74b5d960959c1f90684b37e))


### Bug Fixes

* **tools/looker:** Lookergetdashboards uses proper Authorized helper func ([#1255](https://github.com/googleapis/mcp-toolbox/issues/1255)) ([00866bc](https://github.com/googleapis/mcp-toolbox/commit/00866bc7fc33115c547213e60316ae889735fdbb))
* **tools/mongodb-find-one:** ProjectPayload unmarshaling ([#1167](https://github.com/googleapis/mcp-toolbox/issues/1167)) ([8ea6a98](https://github.com/googleapis/mcp-toolbox/commit/8ea6a98bd9096ba97722e5f807366887e864004f))
* **tools/mysql:** Fix encoded text for mysql ([#1161](https://github.com/googleapis/mcp-toolbox/issues/1161)) ([a37cfa8](https://github.com/googleapis/mcp-toolbox/commit/a37cfa841d151b9995d4fab73cfc5e4d30d2cc57)), closes [#840](https://github.com/googleapis/mcp-toolbox/issues/840)

## [0.12.0](https://github.com/googleapis/mcp-toolbox/compare/v0.11.0...v0.12.0) (2025-08-14)


### Features

* **prebuiltconfig:** Introduce additional parameter to limit context in list_tables ([#1151](https://github.com/googleapis/mcp-toolbox/issues/1151)) ([497d3b1](https://github.com/googleapis/mcp-toolbox/commit/497d3b126da252a4b59806ca2ca3c56e78efc13d))
* **prebuiltconfig/alloydb-admin:** Add list cluster, instance and users ([#1126](https://github.com/googleapis/mcp-toolbox/issues/1126)) ([b42c139](https://github.com/googleapis/mcp-toolbox/commit/b42c139158650fb1f3b696965e840c52e2016bf0))
* **prebuiltconfig/alloydb-admin:** Add tool to create user via Built in user type or IAM ([#1130](https://github.com/googleapis/mcp-toolbox/issues/1130)) ([f5bcb9c](https://github.com/googleapis/mcp-toolbox/commit/f5bcb9c755a2c1747d0beeda568b6217d7420e7a))
* **source/http:** Add User Agent to `http` invocations ([#1102](https://github.com/googleapis/mcp-toolbox/issues/1102)) ([6f55b78](https://github.com/googleapis/mcp-toolbox/commit/6f55b78e96b8c7aa9aca601cfae4d62f3e1eb42b))
* **sources/postgres:** Add support for `queryParams` ([#1047](https://github.com/googleapis/mcp-toolbox/issues/1047)) ([7b57251](https://github.com/googleapis/mcp-toolbox/commit/7b5725140279de21fece45e860945b7a7d23e7d0)), closes [#963](https://github.com/googleapis/mcp-toolbox/issues/963)
* **tools/bigquery-execute-sql:** Add dry run support ([#1057](https://github.com/googleapis/mcp-toolbox/issues/1057)) ([1cac9b5](https://github.com/googleapis/mcp-toolbox/commit/1cac9b5b378153c7dc65ff3dfb4ebd852b715a10))
* **tools/dataplex-search-aspect-types:** Add support for `dataplex-search-aspect-types` tool ([#1061](https://github.com/googleapis/mcp-toolbox/issues/1061)) ([d940187](https://github.com/googleapis/mcp-toolbox/commit/d940187c851666cc201f519665fb4f2e1478465c))
* **tools/looker:** Add `looker-make-look` tool to create Looks ([#1099](https://github.com/googleapis/mcp-toolbox/issues/1099)) ([61d9489](https://github.com/googleapis/mcp-toolbox/commit/61d94893448f633a5f2b9d7f0744ab40704af824))
* **tools/looker:** Add visualizations to `query-url` tool ([#1090](https://github.com/googleapis/mcp-toolbox/issues/1090)) ([5bf2758](https://github.com/googleapis/mcp-toolbox/commit/5bf275846a268a8d305d6392fa4e8e79e365f00d))
* **tools/looker:** New Looker tools for dashboards ([#1118](https://github.com/googleapis/mcp-toolbox/issues/1118)) ([42be3f5](https://github.com/googleapis/mcp-toolbox/commit/42be3f550ceab34baf43fe2a246ded7a09cff8e3))
* **ui:** Add login with google button for automatic id token retrieval ([#1044](https://github.com/googleapis/mcp-toolbox/issues/1044)) ([d91bdfc](https://github.com/googleapis/mcp-toolbox/commit/d91bdfcbdcbf5fcae6e17770c88c5ffba4115d67))


### Bug Fixes

* Correct the capitalization of `map` manifests ([#1139](https://github.com/googleapis/mcp-toolbox/issues/1139)) ([0b0457c](https://github.com/googleapis/mcp-toolbox/commit/0b0457c8e6b78f53a2f1929c05d46fb31421fbca))
* Remove unnecessary fields from `map` parameter manifests ([#1138](https://github.com/googleapis/mcp-toolbox/issues/1138)) ([fbe8c1a](https://github.com/googleapis/mcp-toolbox/commit/fbe8c1a9c0f28797443bf9cb32d63bfbc1072881))
* **tools/looker:** Add authorized invocation feature to all Looker tools ([#1091](https://github.com/googleapis/mcp-toolbox/issues/1091)) ([3b1cce7](https://github.com/googleapis/mcp-toolbox/commit/3b1cce72e7ff4f6b3a0a31db0564dc45b8302caa))
* Update ui info log to reflect port ([#1125](https://github.com/googleapis/mcp-toolbox/issues/1125)) ([6d691d5](https://github.com/googleapis/mcp-toolbox/commit/6d691d582f18137de504d39f372c5104b7392bff))

## [0.11.0](https://github.com/googleapis/mcp-toolbox/compare/v0.11.0...v0.11.0) (2025-08-05)


### ⚠ BREAKING CHANGES

* **tools/bigquery-sql:** Ensure invoke always returns a non-null value ([#1020](https://github.com/googleapis/mcp-toolbox/issues/1020)) ([9af55b6](https://github.com/googleapis/mcp-toolbox/commit/9af55b651d836f268eda342ea27380e7c9967c94))
* **tools/bigquery-execute-sql:** Update the return messages ([#1034](https://github.com/googleapis/mcp-toolbox/issues/1034)) ([051e686](https://github.com/googleapis/mcp-toolbox/commit/051e686476a781ca49f7617764d507916a1188b8))

### Features

* Add TiDB source and tool ([#829](https://github.com/googleapis/mcp-toolbox/issues/829)) ([6eaf36a](https://github.com/googleapis/mcp-toolbox/commit/6eaf36ac8505d523fa4f5a4ac3c97209fd688cef))
* Interactive web UI for Toolbox  ([#1065](https://github.com/googleapis/mcp-toolbox/issues/1065)) ([8749b03](https://github.com/googleapis/mcp-toolbox/commit/8749b030035e65361047c4ead13dfacb8e9a9b59))
* **prebuiltconfigs/cloud-sql-postgres:** Introduce additional parameter to limit context in list tables ([#1062](https://github.com/googleapis/mcp-toolbox/issues/1062)) ([c3a58e1](https://github.com/googleapis/mcp-toolbox/commit/c3a58e1d1678dc14d8de5006511df597fd75faa3))
* **tools/looker-query-url:** Add support for `looker-query-url` tool ([#1015](https://github.com/googleapis/mcp-toolbox/issues/1015)) ([327ddf0](https://github.com/googleapis/mcp-toolbox/commit/327ddf0439058aa5ecd2c7ae8251fcde6aeff18c))
* **tools/dataplex-lookup-entry:** Add support for `dataplex-lookup-entry` tool ([#1009](https://github.com/googleapis/mcp-toolbox/issues/1009)) ([5fa1660](https://github.com/googleapis/mcp-toolbox/commit/5fa1660fc8631989b4d13abea205b6426bb506a5))

### Bug Fixes

* **tools/bigquery,mssql,mysql,postgres,spanner,tidb:** Add query logging to execute-sql tools ([#1069](https://github.com/googleapis/mcp-toolbox/issues/1069)) ([0527532]([https://github.com/googleapis/mcp-toolbox/commit/0527532bd7085ef9eb8f9c30f430a2f2f35cef32))

## [0.10.0](https://github.com/googleapis/mcp-toolbox/compare/v0.9.0...v0.10.0) (2025-07-25)


### Features

* Add `Map` parameters support ([#928](https://github.com/googleapis/mcp-toolbox/issues/928)) ([4468bc9](https://github.com/googleapis/mcp-toolbox/commit/4468bc920bbf27dce4ab160197587b7c12fcd20f))
* Add Dataplex source and tool ([#847](https://github.com/googleapis/mcp-toolbox/issues/847)) ([30c16a5](https://github.com/googleapis/mcp-toolbox/commit/30c16a559e8d49a9a717935269e69b97ec25519a))
* Add Looker source and tool ([#923](https://github.com/googleapis/mcp-toolbox/issues/923)) ([c67e01b](https://github.com/googleapis/mcp-toolbox/commit/c67e01bcf998e7b884be30ebb1fd277c89ed6ffc))
* Add support for null optional parameter ([#802](https://github.com/googleapis/mcp-toolbox/issues/802)) ([a817b12](https://github.com/googleapis/mcp-toolbox/commit/a817b120ca5e09ce80eb8d7544ebbe81fc28b082)), closes [#736](https://github.com/googleapis/mcp-toolbox/issues/736)
* **prebuilt/alloydb-admin-config:** Add alloydb control plane as a prebuilt config ([#937](https://github.com/googleapis/mcp-toolbox/issues/937)) ([0b28b72](https://github.com/googleapis/mcp-toolbox/commit/0b28b72aa0ca2cdc87afbddbeb7f4dbb9688593d))
* **prebuilt/mysql,prebuilt/mssql:** Add generic mysql and mssql prebuilt tools ([#983](https://github.com/googleapis/mcp-toolbox/issues/983)) ([c600c30](https://github.com/googleapis/mcp-toolbox/commit/c600c30374443b6106c1f10b60cd334fd202789b))
* **server/mcp:** Support MCP version 2025-06-18 ([#898](https://github.com/googleapis/mcp-toolbox/issues/898)) ([313d3ca](https://github.com/googleapis/mcp-toolbox/commit/313d3ca0d084a3a6e7ac9a21a862aa31bf3edadd))
* **sources/mssql:** Add support for encrypt connection parameter ([#874](https://github.com/googleapis/mcp-toolbox/issues/874)) ([14a868f](https://github.com/googleapis/mcp-toolbox/commit/14a868f2a0780b94c2ca104419b2ff098778303b))
* **sources/firestore:** Add Firestore as Source ([#786](https://github.com/googleapis/mcp-toolbox/issues/786)) ([2bb790e](https://github.com/googleapis/mcp-toolbox/commit/2bb790e4f8194b677fe0ba40122d409d0e3e687e))
* **sources/mongodb:** Add MongoDB Source ([#969](https://github.com/googleapis/mcp-toolbox/issues/969)) ([74dbd61](https://github.com/googleapis/mcp-toolbox/commit/74dbd6124daab6192dd880dbd1d15f36861abf74))
* **tools/alloydb-wait-for-operation:** Add wait for operation tool with exponential backoff ([#920](https://github.com/googleapis/mcp-toolbox/issues/920)) ([3f6ec29](https://github.com/googleapis/mcp-toolbox/commit/3f6ec2944ede18ee02b10157cc048145bdaec87a))
* **tools/mongodb-aggregate:** Add MongoDB `aggregate` Tools ([#977](https://github.com/googleapis/mcp-toolbox/issues/977)) ([bd399bb](https://github.com/googleapis/mcp-toolbox/commit/bd399bb0fb7134469345ed9a1111ea4209440867))
* **tools/mongodb-delete:** Add MongoDB `delete` Tools ([#974](https://github.com/googleapis/mcp-toolbox/issues/974)) ([78e9752](https://github.com/googleapis/mcp-toolbox/commit/78e9752f620e065246f3e7b9d37062e492247c8a))
* **tools/mongodb-find:** Add MongoDB `find` Tools ([#970](https://github.com/googleapis/mcp-toolbox/issues/970)) ([a747475](https://github.com/googleapis/mcp-toolbox/commit/a7474752d8d7ea7af1e80a3c4533d2fd4154d897))
* **tools/mongodb-insert:** Add MongoDB `insert` Tools ([#975](https://github.com/googleapis/mcp-toolbox/issues/975)) ([4c63f0c](https://github.com/googleapis/mcp-toolbox/commit/4c63f0c1e402817a0c8fec611635e99290308d0e))
* **tools/mongodb-update:** Add MongoDB `update` Tools ([#972](https://github.com/googleapis/mcp-toolbox/issues/972)) ([dfde52c](https://github.com/googleapis/mcp-toolbox/commit/dfde52ca9a8e25e2f3944f52b4c2e307072b6c37))
* **tools/neo4j-execute-cypher:** Add neo4j-execute-cypher for Neo4j sources ([#946](https://github.com/googleapis/mcp-toolbox/issues/946)) ([81d0505](https://github.com/googleapis/mcp-toolbox/commit/81d05053b2e08338fd6eabe4849c309064f76b6b))
* **tools/neo4j-schema:** Add neo4j-schema tool ([#978](https://github.com/googleapis/mcp-toolbox/issues/978)) ([be7db3d](https://github.com/googleapis/mcp-toolbox/commit/be7db3dff263625ce64fdb726e81164996b7a708))
* **tools/wait:** Create wait for tool ([#885](https://github.com/googleapis/mcp-toolbox/issues/885)) ([ed5ef4c](https://github.com/googleapis/mcp-toolbox/commit/ed5ef4caea10ba1dbc49c0fc0a0d2b91cf341d3b))


### Bug Fixes

* Fix document preview pipeline for forked PRs ([#950](https://github.com/googleapis/mcp-toolbox/issues/950)) ([481cc60](https://github.com/googleapis/mcp-toolbox/commit/481cc608bae807d9e92497bc8863066916f7ef21))
* **prebuilt/firestore:** Mark database field as required in the firestore prebuilt tools ([#959](https://github.com/googleapis/mcp-toolbox/issues/959)) ([15417d4](https://github.com/googleapis/mcp-toolbox/commit/15417d4e0c7b173e81edbbeb672e53884d186104))
* **prebuilt/cloud-sql-mssql:** Correct source reference for execute_sql tool in cloud-sql-mssql.yaml prebuilt config ([#938](https://github.com/googleapis/mcp-toolbox/issues/938)) ([d16728e](https://github.com/googleapis/mcp-toolbox/commit/d16728e5c603eab37700876a6ddacbf709fd5823))
* **prebuilt/cloud-sql-mysql:** Update list_table tool ([#924](https://github.com/googleapis/mcp-toolbox/issues/924)) ([2083ba5](https://github.com/googleapis/mcp-toolbox/commit/2083ba50483951e9ee6101bb832aa68823cd96a5))
* Replace 'float' with 'number' in McpManifest ([#985](https://github.com/googleapis/mcp-toolbox/issues/985)) ([59e23e1](https://github.com/googleapis/mcp-toolbox/commit/59e23e17250a516e3931996114f32ac6526a4f8e))
* **server/api:** Add logger to context in tool invoke handler ([#891](https://github.com/googleapis/mcp-toolbox/issues/891)) ([8ce311f](https://github.com/googleapis/mcp-toolbox/commit/8ce311f256481e8f11ecb4aa505b95a562f394ef))
* **sources/looker:** Add agent tag to Looker API calls. ([#966](https://github.com/googleapis/mcp-toolbox/issues/966)) ([f55dd6f](https://github.com/googleapis/mcp-toolbox/commit/f55dd6fcd099f23bd89df62b268c4a53d16f3bac))
* **tools/bigquery-execute-sql:** Ensure invoke always returns a non-null value ([#925](https://github.com/googleapis/mcp-toolbox/issues/925)) ([9a55b80](https://github.com/googleapis/mcp-toolbox/commit/9a55b804821a6ccfcd157bcfaee7e599c4a5cb63))
* **tools/mysqlsql:** Unmarshal json data from database during invoke ([#979](https://github.com/googleapis/mcp-toolbox/issues/979)) ([ccc3498](https://github.com/googleapis/mcp-toolbox/commit/ccc3498cf0a4c43eb909e3850b9e6f582cd48f2a)), closes [#840](https://github.com/googleapis/mcp-toolbox/issues/840)

## [0.9.0](https://github.com/googleapis/mcp-toolbox/compare/v0.8.0...v0.9.0) (2025-07-11)


### Features

* Dynamic reloading for toolbox config ([#800](https://github.com/googleapis/mcp-toolbox/issues/800)) ([4c240ac](https://github.com/googleapis/mcp-toolbox/commit/4c240ac3c961cd14738c998ba2d10d5235ef523e))
* **sources/mysql:** Add queryTimeout support to MySQL source ([#830](https://github.com/googleapis/mcp-toolbox/issues/830)) ([391cb5b](https://github.com/googleapis/mcp-toolbox/commit/391cb5bfe845e554411240a1d9838df5331b25fa))
* **tools/bigquery:** Add optional projectID parameter to bigquery tools ([#799](https://github.com/googleapis/mcp-toolbox/issues/799)) ([c6ab74c](https://github.com/googleapis/mcp-toolbox/commit/c6ab74c5dad53a0e7885a18438ab3be36b9b7cb3))


### Bug Fixes

* Cleanup unassigned err log ([#857](https://github.com/googleapis/mcp-toolbox/issues/857)) ([c081ace](https://github.com/googleapis/mcp-toolbox/commit/c081ace46bb24cb3fd2adb21d519489be0d3f3c3))
* Fix docs preview deployment pipeline ([#787](https://github.com/googleapis/mcp-toolbox/issues/787)) ([0a93b04](https://github.com/googleapis/mcp-toolbox/commit/0a93b0482c8d3c64b324e67408d408f5576ecaf3))
* **tools:** Nil parameter error when arrays are used ([#801](https://github.com/googleapis/mcp-toolbox/issues/801)) ([2bdcc08](https://github.com/googleapis/mcp-toolbox/commit/2bdcc0841ab37d18e2f0d6fe63fb6f10da3e302b))
* Trigger reload on additional fsnotify operations ([#854](https://github.com/googleapis/mcp-toolbox/issues/854)) ([aa8dbec](https://github.com/googleapis/mcp-toolbox/commit/aa8dbec97095cf0d7ac771c8084a84e2d3d8ce4e))

## [0.8.0](https://github.com/googleapis/mcp-toolbox/compare/v0.7.0...v0.8.0) (2025-07-02)


### ⚠ BREAKING CHANGES

* **postgres,mssql,cloudsqlmssql:** encode source connection url for  sources ([#727](https://github.com/googleapis/mcp-toolbox/issues/727))

### Features

* Add support for multiple YAML configuration files ([#760](https://github.com/googleapis/mcp-toolbox/issues/760)) ([40679d7](https://github.com/googleapis/mcp-toolbox/commit/40679d700eded50d19569923e2a71c51e907a8bf))
* Add support for optional parameters ([#617](https://github.com/googleapis/mcp-toolbox/issues/617)) ([4827771](https://github.com/googleapis/mcp-toolbox/commit/4827771b78dee9a1284a898b749509b472061527)), closes [#475](https://github.com/googleapis/mcp-toolbox/issues/475)
* **mcp:** Support MCP version 2025-03-26 ([#755](https://github.com/googleapis/mcp-toolbox/issues/755)) ([474df57](https://github.com/googleapis/mcp-toolbox/commit/474df57d62de683079f8d12c31db53396a545fd1))
* **sources/http:** Support disable SSL verification for HTTP Source ([#674](https://github.com/googleapis/mcp-toolbox/issues/674)) ([4055b0c](https://github.com/googleapis/mcp-toolbox/commit/4055b0c3569c527560d7ad34262963b3dd4e282d))
* **tools/bigquery:** Add templateParameters field for bigquery ([#699](https://github.com/googleapis/mcp-toolbox/issues/699)) ([f5f771b](https://github.com/googleapis/mcp-toolbox/commit/f5f771b0f3d159630ff602ff55c6c66b61981446))
* **tools/bigtable:** Add templateParameters field for bigtable ([#692](https://github.com/googleapis/mcp-toolbox/issues/692)) ([1c06771](https://github.com/googleapis/mcp-toolbox/commit/1c067715fac06479eb0060d7067b73dba099ed92))
* **tools/couchbase:** Add templateParameters field for couchbase ([#723](https://github.com/googleapis/mcp-toolbox/issues/723)) ([9197186](https://github.com/googleapis/mcp-toolbox/commit/9197186b8bea1ac4ec1b39c9c5c110807c8b2ba9))
* **tools/http:** Add support for HTTP Tool pathParams ([#726](https://github.com/googleapis/mcp-toolbox/issues/726)) ([fd300dc](https://github.com/googleapis/mcp-toolbox/commit/fd300dc606d88bf9f7bba689e2cee4e3565537dd))
* **tools/redis:** Add Redis Source and Tool ([#519](https://github.com/googleapis/mcp-toolbox/issues/519)) ([f0aef29](https://github.com/googleapis/mcp-toolbox/commit/f0aef29b0c2563e2a00277fbe2784f39f16d2835))
* **tools/spanner:** Add templateParameters field for spanner ([#691](https://github.com/googleapis/mcp-toolbox/issues/691)) ([075dfa4](https://github.com/googleapis/mcp-toolbox/commit/075dfa47e1fd92be4847bd0aec63296146b66455))
* **tools/sqlitesql:** Add templateParameters field for sqlitesql ([#687](https://github.com/googleapis/mcp-toolbox/issues/687)) ([75e254c](https://github.com/googleapis/mcp-toolbox/commit/75e254c0a4ce690ca5fa4d1741550ce54734b226))
* **tools/valkey:** Add Valkey Source and Tool ([#532](https://github.com/googleapis/mcp-toolbox/issues/532)) ([054ec19](https://github.com/googleapis/mcp-toolbox/commit/054ec198b97ba9f36f67dd12b2eff0cc6bc4d080))


### Bug Fixes

* **bigquery,mssql:** Fix panic on tools with array param ([#722](https://github.com/googleapis/mcp-toolbox/issues/722)) ([7a6644c](https://github.com/googleapis/mcp-toolbox/commit/7a6644cf0c5413e5c803955c88a2cfd0a2233ed3))
* **postgres,mssql,cloudsqlmssql:** Encode source connection url for  sources ([#727](https://github.com/googleapis/mcp-toolbox/issues/727)) ([67964d9](https://github.com/googleapis/mcp-toolbox/commit/67964d939f27320b63b5759f4b3f3fdaa0c76fbf)), closes [#717](https://github.com/googleapis/mcp-toolbox/issues/717)
* Set default value to field's type during unmarshalling ([#774](https://github.com/googleapis/mcp-toolbox/issues/774)) ([fafed24](https://github.com/googleapis/mcp-toolbox/commit/fafed2485839cf1acc1350e8a24103d2e6356ee0)), closes [#771](https://github.com/googleapis/mcp-toolbox/issues/771)
* **server/mcp:** Do not listen from port for stdio ([#719](https://github.com/googleapis/mcp-toolbox/issues/719)) ([d51dbc7](https://github.com/googleapis/mcp-toolbox/commit/d51dbc759ba493021d3ec6f5417fc04c21f7044f)), closes [#711](https://github.com/googleapis/mcp-toolbox/issues/711)
* **tools/mysqlexecutesql:** Handle nil panic and connection leak in Invoke ([#757](https://github.com/googleapis/mcp-toolbox/issues/757)) ([7badba4](https://github.com/googleapis/mcp-toolbox/commit/7badba42eefb34252be77b852a57d6bd78dd267d))
* **tools/mysqlsql:** Handle nil panic and connection leak in invoke ([#758](https://github.com/googleapis/mcp-toolbox/issues/758)) ([cbb4a33](https://github.com/googleapis/mcp-toolbox/commit/cbb4a333517313744800d148840312e56340f3fd))

## [0.7.0](https://github.com/googleapis/mcp-toolbox/compare/v0.6.0...v0.7.0) (2025-06-10)


### Features

* Add templateParameters field for mssqlsql ([#671](https://github.com/googleapis/mcp-toolbox/issues/671)) ([b81fc6a](https://github.com/googleapis/mcp-toolbox/commit/b81fc6aa6ccdfbc15676fee4d87041d9ad9682fa))
* Add templateParameters field for mysqlsql ([#663](https://github.com/googleapis/mcp-toolbox/issues/663)) ([0a08d2c](https://github.com/googleapis/mcp-toolbox/commit/0a08d2c15dcbec18bb556f4dc49792ba0c69db46))
* **metrics:** Add user agent for prebuilt tools ([#669](https://github.com/googleapis/mcp-toolbox/issues/669)) ([29aa0a7](https://github.com/googleapis/mcp-toolbox/commit/29aa0a70da3c2eb409a38993b3782da8bec7cb85))
* **tools/postgressql:** Add templateParameters field ([#615](https://github.com/googleapis/mcp-toolbox/issues/615)) ([b763469](https://github.com/googleapis/mcp-toolbox/commit/b76346993f298b4f7493a51405d0a287bacce05f))


### Bug Fixes

* Improve versionString ([#658](https://github.com/googleapis/mcp-toolbox/issues/658)) ([cf96f4c](https://github.com/googleapis/mcp-toolbox/commit/cf96f4c249f0692e3eb19fc56c794ca6a3079307))
* **server/stdio:** Notifications should not return a response ([#638](https://github.com/googleapis/mcp-toolbox/issues/638)) ([69d047a](https://github.com/googleapis/mcp-toolbox/commit/69d047af46f1ec00f236db8a978a7a7627217fd2))
* **tools/mysqlsql:** Handled the null value for string case in mysqlsql tools ([#641](https://github.com/googleapis/mcp-toolbox/issues/641)) ([ef94648](https://github.com/googleapis/mcp-toolbox/commit/ef94648455c3b20adda4f8cff47e70ddccac8c06))
* Update path library ([#678](https://github.com/googleapis/mcp-toolbox/issues/678)) ([4998f82](https://github.com/googleapis/mcp-toolbox/commit/4998f8285287b5daddd0043540f2cf871e256db5)), closes [#662](https://github.com/googleapis/mcp-toolbox/issues/662)

## [0.6.0](https://github.com/googleapis/mcp-toolbox/compare/v0.5.0...v0.6.0) (2025-05-28)


### Features

* Add Execute sql tool for SQL Server(MSSQL) ([#585](https://github.com/googleapis/mcp-toolbox/issues/585)) ([6083a22](https://github.com/googleapis/mcp-toolbox/commit/6083a224aa650caf4e132b4a704323c5f18c4986))
* Add mysql-execute-sql tool ([#577](https://github.com/googleapis/mcp-toolbox/issues/577)) ([8590061](https://github.com/googleapis/mcp-toolbox/commit/8590061ae4908da0e4b1bd6f7cf7ee8d972fa5ba))
* Add new BigQuery tools: execute_sql, list_datatset_ids, list_table_ids, get_dataset_info, get_table_info ([0fd88b5](https://github.com/googleapis/mcp-toolbox/commit/0fd88b574b4ab0d3bee4585999b814675d3b74ed))
* Add spanner-execute-sql tool ([#576](https://github.com/googleapis/mcp-toolbox/issues/576)) ([d65747a](https://github.com/googleapis/mcp-toolbox/commit/d65747a2dcf3022f22c86a1524ee28c8229f7c20))
* Add support for read-only in Spanner tool ([#563](https://github.com/googleapis/mcp-toolbox/issues/563)) ([6512704](https://github.com/googleapis/mcp-toolbox/commit/6512704e77088d92fea53a85c6e6cbf4b99c988d))
* Adding support for the --prebuilt flag ([#604](https://github.com/googleapis/mcp-toolbox/issues/604)) ([a29c800](https://github.com/googleapis/mcp-toolbox/commit/a29c80012eec4729187c12968b53051d20b263a7))
* Support MCP stdio transport protocol ([#607](https://github.com/googleapis/mcp-toolbox/issues/607)) ([1702ce1](https://github.com/googleapis/mcp-toolbox/commit/1702ce1e00a52170a4271ac999caf534ba00196f))


### Bug Fixes

* Explicitly set query location for BigQuery queries ([#586](https://github.com/googleapis/mcp-toolbox/issues/586)) ([eb52b66](https://github.com/googleapis/mcp-toolbox/commit/eb52b66d82aaa11be6b1489335f49cba8168099b))
* Fix spellings in comments ([#561](https://github.com/googleapis/mcp-toolbox/issues/561)) ([b58bf76](https://github.com/googleapis/mcp-toolbox/commit/b58bf76ddaba407e3fd995dfe86d00a09484e14a))
* Prevent tool calls through MCP when auth is required ([#544](https://github.com/googleapis/mcp-toolbox/issues/544)) ([e747b6e](https://github.com/googleapis/mcp-toolbox/commit/e747b6e289730c17f68be8dec0c6fa6021bb23bd))
* Reinitialize required slice if nil ([#571](https://github.com/googleapis/mcp-toolbox/issues/571)) ([04dcf47](https://github.com/googleapis/mcp-toolbox/commit/04dcf4791272e1dd034b9a03664dd8dbe77fdddd)), closes [#564](https://github.com/googleapis/mcp-toolbox/issues/564)

## [0.5.0](https://github.com/googleapis/mcp-toolbox/compare/v0.4.0...v0.5.0) (2025-05-06)


### Features

* Add Couchbase as Source and Tool ([#307](https://github.com/googleapis/mcp-toolbox/issues/307)) ([d7390b0](https://github.com/googleapis/mcp-toolbox/commit/d7390b06b7bcb15411388e9a4dbcfe75afcca1ee))
* Add postgres-execute-sql tool ([#490](https://github.com/googleapis/mcp-toolbox/issues/490)) ([11ea7bc](https://github.com/googleapis/mcp-toolbox/commit/11ea7bc584aa4ca8e8b0e7a355f6666ccbea2883))


## [0.4.0](https://github.com/googleapis/mcp-toolbox/compare/v0.3.0...v0.4.0) (2025-04-23)


### Features

* Add `AuthRequired` to Neo4j & Dgraph Tools ([#434](https://github.com/googleapis/mcp-toolbox/issues/434)) ([afbf4b2](https://github.com/googleapis/mcp-toolbox/commit/afbf4b2daeb01119a22ce18469bffb9e9f57d2f8))
* Add `AuthRequired` to tool manifest ([#433](https://github.com/googleapis/mcp-toolbox/issues/433)) ([d9388ad](https://github.com/googleapis/mcp-toolbox/commit/d9388ad57e832570aab56b9b357c1fb0ba994852))
* Add BigQuery source and tool ([#463](https://github.com/googleapis/mcp-toolbox/issues/463)) ([8055aa5](https://github.com/googleapis/mcp-toolbox/commit/8055aa519fe6e7993ba524f8f7e684fbfdecc1b9))
* Add Bigtable source and tool ([#418](https://github.com/googleapis/mcp-toolbox/issues/418)) ([ae53b8e](https://github.com/googleapis/mcp-toolbox/commit/ae53b8eeff9d0e9ec14d9c6d4286c856cc8f1811))
* Add IAM AuthN to AlloyDB Source ([#399](https://github.com/googleapis/mcp-toolbox/issues/399)) ([e8ed447](https://github.com/googleapis/mcp-toolbox/commit/e8ed447d9153c60a1d6321285587e6e4ca930f87))
* Add IAM AuthN to Cloud SQL Sources ([#414](https://github.com/googleapis/mcp-toolbox/issues/414)) ([be85b82](https://github.com/googleapis/mcp-toolbox/commit/be85b820785dbce79133b0cf8788bde75ff25fee))
* Add toolset feature to mcp ([#425](https://github.com/googleapis/mcp-toolbox/issues/425)) ([e307857](https://github.com/googleapis/mcp-toolbox/commit/e307857085ac4c8c2ee8292c914daa5534ba74bf)), closes [#403](https://github.com/googleapis/mcp-toolbox/issues/403)
* Add SQLite source and tool ([#438](https://github.com/googleapis/mcp-toolbox/issues/438)) ([fc14cbf](https://github.com/googleapis/mcp-toolbox/commit/fc14cbfd078d465591e4fefb80542759e82a2731))
* Support env replacement for tools.yaml ([#462](https://github.com/googleapis/mcp-toolbox/issues/462)) ([eadb678](https://github.com/googleapis/mcp-toolbox/commit/eadb678a7bd4ce74a3b1160f5ed8966ffbb13c61))


### Bug Fixes

* [#419](https://github.com/googleapis/mcp-toolbox/issues/419) TLS https URL for SSE endpoint ([#420](https://github.com/googleapis/mcp-toolbox/issues/420)) ([0a7d3ff](https://github.com/googleapis/mcp-toolbox/commit/0a7d3ff06b88051c752b6d53bc964ed6e6be400e))
* **docs:** Fix link 'Edit this page' ([#454](https://github.com/googleapis/mcp-toolbox/issues/454)) ([969065e](https://github.com/googleapis/mcp-toolbox/commit/969065e561f28ddb9755d99bbe0b288040198296)), closes [#427](https://github.com/googleapis/mcp-toolbox/issues/427)
* Update http error code from invocation ([#468](https://github.com/googleapis/mcp-toolbox/issues/468)) ([ff7c0ff](https://github.com/googleapis/mcp-toolbox/commit/ff7c0ffc65172a335e8d3321e5a6b92d38dc7e6d)), closes [#465](https://github.com/googleapis/mcp-toolbox/issues/465)

## [0.3.0](https://github.com/googleapis/mcp-toolbox/compare/v0.2.1...v0.3.0) (2025-04-04)


### Features

* Add 'alloydb-ai-nl' tool ([#358](https://github.com/googleapis/mcp-toolbox/issues/358)) ([f02885f](https://github.com/googleapis/mcp-toolbox/commit/f02885fd4a919103fdabaa4ca38d975dc8497542))
* Add HTTP Source and Tool ([#332](https://github.com/googleapis/mcp-toolbox/issues/332)) ([64da5b4](https://github.com/googleapis/mcp-toolbox/commit/64da5b4efe7d948ceb366c37fdaabd42405bc932))
* Adding support for Model Context Protocol (MCP). ([#396](https://github.com/googleapis/mcp-toolbox/issues/396)) ([a7d1d4e](https://github.com/googleapis/mcp-toolbox/commit/a7d1d4eb2ae337b463d1b25ccb25c3c0eb30df6f))
* Added [toolbox-core](https://pypi.org/project/toolbox-core/) SDK – easily integrate Toolbox into any Python function calling framework


### Bug Fixes

* Add `tools-file` flag and deprecate `tools_file`  ([#384](https://github.com/googleapis/mcp-toolbox/issues/384)) ([34a7263](https://github.com/googleapis/mcp-toolbox/commit/34a7263fdce40715de20ef5677f94be29f9f5c98)), closes [#383](https://github.com/googleapis/mcp-toolbox/issues/383)

## [0.2.1](https://github.com/googleapis/mcp-toolbox/compare/v0.2.0...v0.2.1) (2025-03-20)


### Bug Fixes

* Fix variable name in quickstart ([#336](https://github.com/googleapis/mcp-toolbox/issues/336)) ([5400127](https://github.com/googleapis/mcp-toolbox/commit/54001278878042aff75ed421b9fbe70008e9dd4d))
* **source/alloydb:** Correct user agents not being sent ([#323](https://github.com/googleapis/mcp-toolbox/issues/323)) ([ce12a34](https://github.com/googleapis/mcp-toolbox/commit/ce12a344ed6290c7c6e36ee117318c20d6fdccc2))

## [0.2.0](https://github.com/googleapis/mcp-toolbox/compare/v0.1.0...v0.2.0) (2025-03-03)


### ⚠ BREAKING CHANGES

* Rename "AuthSource" in favor of "AuthService" ([#297](https://github.com/googleapis/mcp-toolbox/issues/297))

### Features

* Rename "AuthSource" in favor of "AuthService" ([#297](https://github.com/googleapis/mcp-toolbox/issues/297)) ([04cb5fb](https://github.com/googleapis/mcp-toolbox/commit/04cb5fbc3e1876d1cf83d3f3de2c176ee2862d63))


### Bug Fixes

* Add items to parameter manifest ([#293](https://github.com/googleapis/mcp-toolbox/issues/293)) ([541612d](https://github.com/googleapis/mcp-toolbox/commit/541612d72d0123b285bb9f58c9cf1bfd61ebd902))
* **source/cloud-sql:** Correct user agents not being sent ([#306](https://github.com/googleapis/mcp-toolbox/issues/306)) ([584c8ae](https://github.com/googleapis/mcp-toolbox/commit/584c8aea438eeb991935b4347c2c3b2cb7144cbf))
* Throw error when items field is missing from array parameter ([#296](https://github.com/googleapis/mcp-toolbox/issues/296)) ([9193836](https://github.com/googleapis/mcp-toolbox/commit/9193836effaae79204f73a8c5d26668a95d2cb91))
* Validate required common fields for parameters ([#298](https://github.com/googleapis/mcp-toolbox/issues/298)) ([e494d11](https://github.com/googleapis/mcp-toolbox/commit/e494d11e6e1651138dcd527171f63d4fa8604211))


### Miscellaneous Chores

* Release 0.2.0 ([#314](https://github.com/googleapis/mcp-toolbox/issues/314)) ([d7ccf73](https://github.com/googleapis/mcp-toolbox/commit/d7ccf730e7c0c752615f8a7ea162836c5f9950da))

## [0.1.0](https://github.com/googleapis/mcp-toolbox/compare/v0.0.5...v0.1.0) (2025-02-06)


### ⚠ BREAKING CHANGES

* **langchain-sdk:** The SDK for `toolbox-langchain` is now located [here](https://github.com/googleapis/-langchain-python).

### Features

* Add Cloud SQL for SQL Server Source and Tool ([#223](https://github.com/googleapis/mcp-toolbox/issues/223)) ([9bad952](https://github.com/googleapis/mcp-toolbox/commit/9bad9520604aa363a6d73f5ce14686895c2f4333))
* Add Cloud SQL for MySQL Source and Tool ([#221](https://github.com/googleapis/mcp-toolbox/issues/221)) ([f1f61d7](https://github.com/googleapis/mcp-toolbox/commit/f1f61d70877a1c7cc9080f6d70112bd0c0533473))
* Add Dgraph Source and Tool ([#233](https://github.com/googleapis/mcp-toolbox/issues/233)) ([617cc87](https://github.com/googleapis/mcp-toolbox/commit/617cc872d1d692138a712d39fb7c1a405e9c1876))
* Add local quickstart ([#232](https://github.com/googleapis/mcp-toolbox/issues/232)) ([497fb06](https://github.com/googleapis/mcp-toolbox/commit/497fb06fae6d04adaad11fa78eb04282d0225dbd))
* Add user agents for cloud sources ([#244](https://github.com/googleapis/mcp-toolbox/issues/244)) ([8452f8e](https://github.com/googleapis/mcp-toolbox/commit/8452f8eb4457dcb0e360a9d9ae5b6e14e78806b1))
* Add MySQL Source ([#250](https://github.com/googleapis/mcp-toolbox/issues/250)) ([378692a](https://github.com/googleapis/mcp-toolbox/commit/378692ab50a90dcc1c3353052d0741cfd318c79d))
* Add MSSQL source ([#255](https://github.com/googleapis/mcp-toolbox/issues/255)) ([8fca0a9](https://github.com/googleapis/mcp-toolbox/commit/8fca0a95ee5e79e30919b05592af643ba57f3183))


### Bug Fixes

* Auth token verification failure should not throw error immediately ([#234](https://github.com/googleapis/mcp-toolbox/issues/234)) ([4639cc6](https://github.com/googleapis/mcp-toolbox/commit/4639cc6560f09b6b8203650ccce424ce59aa0c14))
* Fix typo in postgres test ([#216](https://github.com/googleapis/mcp-toolbox/issues/216)) ([0c3d12a](https://github.com/googleapis/mcp-toolbox/commit/0c3d12ae04a752fddcff06e92967910cdd643bbf))
* **mssql:** Fix mssql tool kind to mssql-sql ([#249](https://github.com/googleapis/mcp-toolbox/issues/249)) ([1357be2](https://github.com/googleapis/mcp-toolbox/commit/1357be2569b5f8d31b2b72fa83749fa8519fc8bd))
* **mysql:** Fix mysql tool kind to mysql-sql ([#248](https://github.com/googleapis/mcp-toolbox/issues/248)) ([669d6b7](https://github.com/googleapis/mcp-toolbox/commit/669d6b7239c36f612f02948716cf167c5a2eaa10))
* Schema float type ([#264](https://github.com/googleapis/mcp-toolbox/issues/264)) ([1702f74](https://github.com/googleapis/mcp-toolbox/commit/1702f74e9937eb4539c38c7152fe474870e61591))
* Typos at test cases ([#265](https://github.com/googleapis/mcp-toolbox/issues/265)) ([b7c5661](https://github.com/googleapis/mcp-toolbox/commit/b7c5661215c431c8590a60e029f3c340132574b7))
* Update README and quickstart with the correct async APIs. ([#269](https://github.com/googleapis/mcp-toolbox/issues/269)) ([21eef2e](https://github.com/googleapis/mcp-toolbox/commit/21eef2e198683d2f7fd0e606a4410b4f3a51686e))
* Update tool invoke to return json ([#266](https://github.com/googleapis/mcp-toolbox/issues/266)) ([ad58cd5](https://github.com/googleapis/mcp-toolbox/commit/ad58cd5855be9e1b73926e16527fb89ce778b8d9))

## [0.0.5](https://github.com/googleapis/mcp-toolbox/compare/v0.0.4...v0.0.5) (2025-01-14)


### ⚠ BREAKING CHANGES

* replace Source field `ip_type` with `ipType` for consistency ([#197](https://github.com/googleapis/mcp-toolbox/issues/197))
* **toolbox-sdk:** deprecate 'add_auth_headers' in favor of 'add_auth_tokens'  ([#170](https://github.com/googleapis/mcp-toolbox/issues/170))

### Features

* Add support for OpenTelemetry ([#205](https://github.com/googleapis/mcp-toolbox/issues/205)) ([1fcc20a](https://github.com/googleapis/mcp-toolbox/commit/1fcc20a8469794ed8e6846cded44196d26c306be))
* Added Neo4j Source and Tool ([#189](https://github.com/googleapis/mcp-toolbox/issues/189)) ([8a1224b](https://github.com/googleapis/mcp-toolbox/commit/8a1224b9e0145c4e214d42f14f5308b508ea27ce))
* **llamaindex-sdk:** Implement OAuth support for LlamaIndex. ([#159](https://github.com/googleapis/mcp-toolbox/issues/159)) ([003ce51](https://github.com/googleapis/mcp-toolbox/commit/003ce510a1fb37a23e4c64fdf21376e0e32ec8ab))
* Replace Source field `ip_type` with `ipType` for consistency ([#197](https://github.com/googleapis/mcp-toolbox/issues/197)) ([e069520](https://github.com/googleapis/mcp-toolbox/commit/e069520bb79d086dbdd37ebc3ad9bb39b31c8fac))
* Update log with given context ([#147](https://github.com/googleapis/mcp-toolbox/issues/147)) ([809e547](https://github.com/googleapis/mcp-toolbox/commit/809e547a481bd4af351bbaa2dcfd203b086bb51d))


### Bug Fixes

* Correct parsing of floats/ints from json ([#180](https://github.com/googleapis/mcp-toolbox/issues/180)) ([387a5b5](https://github.com/googleapis/mcp-toolbox/commit/387a5b56b53ccfe0637a0f44c0ddbec8e991cc39))
* **doc:** Update example `clientId` field ([#198](https://github.com/googleapis/mcp-toolbox/issues/198)) ([0c86e89](https://github.com/googleapis/mcp-toolbox/commit/0c86e895066ee3dee9ab9bc20fe00934066b67ac))
* Fix config name in auth doc samples ([#186](https://github.com/googleapis/mcp-toolbox/issues/186)) ([bb03457](https://github.com/googleapis/mcp-toolbox/commit/bb0345767e0550fcda975958f450086e44f6a913))
* Handle shutdown gracefully ([#178](https://github.com/googleapis/mcp-toolbox/issues/178)) ([66ab70f](https://github.com/googleapis/mcp-toolbox/commit/66ab70f702d7178c61c8d90399483b6125ba01c8))
* Improve return error for parameters  ([#206](https://github.com/googleapis/mcp-toolbox/issues/206)) ([346c57d](https://github.com/googleapis/mcp-toolbox/commit/346c57da2394e398ee8cc527b84973aa2bcde642))
* **toolbox-sdk:** Deprecate 'add_auth_headers' in favor of 'add_auth_tokens'  ([#170](https://github.com/googleapis/mcp-toolbox/issues/170)) ([b56fa68](https://github.com/googleapis/mcp-toolbox/commit/b56fa685e379c3515025ed76d9abe61f93365a65))


### Miscellaneous Chores

* Release 0.0.5 ([#210](https://github.com/googleapis/mcp-toolbox/issues/210)) ([bd407c0](https://github.com/googleapis/mcp-toolbox/commit/bd407c0ab749c9a72523122a2212652f9d97ab03))

## [0.0.4](https://github.com/googleapis/mcp-toolbox/compare/v0.0.3...v0.0.4) (2024-12-18)


### Features

* Add `auth_required` to tools ([#123](https://github.com/googleapis/mcp-toolbox/issues/123)) ([3118104](https://github.com/googleapis/mcp-toolbox/commit/3118104ae17335db073911a88f2ea8ce8d0bfb45))
* Add Auth Source configuration ([#71](https://github.com/googleapis/mcp-toolbox/issues/71)) ([77b0d43](https://github.com/googleapis/mcp-toolbox/commit/77b0d4317580214c1c9bd542b24371f09fd17fe0))
* Add Tool authenticated parameters ([#80](https://github.com/googleapis/mcp-toolbox/issues/80)) ([380a6fb](https://github.com/googleapis/mcp-toolbox/commit/380a6fbbd5a5abc3159c96421b0923c117807267))
* **langchain-sdk:** Correctly parse Manifest API response as JSON ([#143](https://github.com/googleapis/mcp-toolbox/issues/143)) ([2c8633c](https://github.com/googleapis/mcp-toolbox/commit/2c8633c3eb2d936b62fe24c87a6385d5898f4370))
* **langchain-sdk:** Support authentication in LangChain Toolbox SDK. ([#133](https://github.com/googleapis/mcp-toolbox/issues/133)) ([23fa912](https://github.com/googleapis/mcp-toolbox/commit/23fa912a80e7e02f53a5ad27781e32a5cfa05458))


### Bug Fixes

* Fix release image version tag ([#136](https://github.com/googleapis/mcp-toolbox/issues/136)) ([6d19ff9](https://github.com/googleapis/mcp-toolbox/commit/6d19ff96e4004c97739ad6a064ef72e57f8da2f2))
* **langchain-sdk:** Correct test name to ensure execution and full coverage. ([#145](https://github.com/googleapis/mcp-toolbox/issues/145)) ([d820ac3](https://github.com/googleapis/mcp-toolbox/commit/d820ac3767127058dc726b44e469a7adec26783b))
* Set server version ([#150](https://github.com/googleapis/mcp-toolbox/issues/150)) ([abd1eb7](https://github.com/googleapis/mcp-toolbox/commit/abd1eb702c1ab75d76be624d2f0decd34548f93f))


### Miscellaneous Chores

* Release 0.0.4 ([#152](https://github.com/googleapis/mcp-toolbox/issues/152)) ([86ec12f](https://github.com/googleapis/mcp-toolbox/commit/86ec12f8c5d67ced5bcd52c9d8e80b17aa11b514))

## [0.0.3](https://github.com/googleapis/mcp-toolbox/compare/v0.0.2...v0.0.3) (2024-12-10)


### Features

* Add --log-level and --logging-format flags ([#97](https://github.com/googleapis/mcp-toolbox/issues/97)) ([9a0f618](https://github.com/googleapis/mcp-toolbox/commit/9a0f618efca13e0accb2656ea74a393e8cda5d40))
* Add options for command ([#110](https://github.com/googleapis/mcp-toolbox/issues/110)) ([5c690c5](https://github.com/googleapis/mcp-toolbox/commit/5c690c5c30515ae790b045677ef518106c52a491))
* Add Spanner source and tool ([#90](https://github.com/googleapis/mcp-toolbox/issues/90)) ([890914a](https://github.com/googleapis/mcp-toolbox/commit/890914aae0989d181b26efa940326a5c2f559959))
* Add std logger ([#95](https://github.com/googleapis/mcp-toolbox/issues/95)) ([6a8feb5](https://github.com/googleapis/mcp-toolbox/commit/6a8feb51f0d148607f52c4a5c755faa9e3b7e6a4))
* Add structured logger ([#96](https://github.com/googleapis/mcp-toolbox/issues/96)) ([5e20417](https://github.com/googleapis/mcp-toolbox/commit/5e2041755163932c6c3135fad2404cffd22cb463))
* **source/alloydb-pg:** Add configuration for public and private IP ([#103](https://github.com/googleapis/mcp-toolbox/issues/103)) ([e88ec40](https://github.com/googleapis/mcp-toolbox/commit/e88ec409d14c85d6b0896c45d9957cce9097912a))
* **source/cloudsql-pg:** Add configuration for public and private IP ([#114](https://github.com/googleapis/mcp-toolbox/issues/114)) ([6479c1d](https://github.com/googleapis/mcp-toolbox/commit/6479c1dbe26f05438df9c2289118da558eee0a0d))


### Bug Fixes

* Fix go test workflow ([#84](https://github.com/googleapis/mcp-toolbox/issues/84)) ([8c2c373](https://github.com/googleapis/mcp-toolbox/commit/8c2c373d359b718b2182f566bc245a2a8fa03333))
* Fix issue causing client session to not close properly while closing SDK. ([#81](https://github.com/googleapis/mcp-toolbox/issues/81)) ([9d360e1](https://github.com/googleapis/mcp-toolbox/commit/9d360e16eab664992bca9d6b01dbec12c9d5d2e1))
* Fix test cases for ip_type ([#115](https://github.com/googleapis/mcp-toolbox/issues/115)) ([5528bec](https://github.com/googleapis/mcp-toolbox/commit/5528bec8ed8c7efa03979abedc98102bff4abed8))
* Fix the errors showing up after setting up mypy type checker. ([#74](https://github.com/googleapis/mcp-toolbox/issues/74)) ([522bbef](https://github.com/googleapis/mcp-toolbox/commit/522bbefa7b305a1695bb21ce4a9c92429cde4ee9))
* **llamaindex-sdk:** Fix issue causing client session to not close properly while closing SDK. ([#82](https://github.com/googleapis/mcp-toolbox/issues/82)) ([fa03376](https://github.com/googleapis/mcp-toolbox/commit/fa03376bbc4b9dba93a471b13225c8f1a37187c2))


### Miscellaneous Chores

* Release 0.0.3 ([#122](https://github.com/googleapis/mcp-toolbox/issues/122)) ([626e12f](https://github.com/googleapis/mcp-toolbox/commit/626e12fdb3e27996e9e4a8c9661563ec3c3bcc5c))

## [0.0.2](https://github.com/googleapis/mcp-toolbox/compare/v0.0.1...v0.0.2) (2024-11-12)


### ⚠ BREAKING CHANGES

* consolidate "x-postgres-generic" tools to "postgres-sql" tool ([#43](https://github.com/googleapis/mcp-toolbox/issues/43))

### Features

* Consolidate "x-postgres-generic" tools to "postgres-sql" tool ([#43](https://github.com/googleapis/mcp-toolbox/issues/43)) ([f630965](https://github.com/googleapis/mcp-toolbox/commit/f6309659374bc9cb500cc54dd4220baa0a451a3b))
* **container:** Add entrypoint in Dockerfile ([#38](https://github.com/googleapis/mcp-toolbox/issues/38)) ([b08072a](https://github.com/googleapis/mcp-toolbox/commit/b08072a80034a34a394dea82838422bd6cb0d23a))
* **sdk:** Added LlamaIndex SDK ([#48](https://github.com/googleapis/mcp-toolbox/issues/48)) ([b824abe](https://github.com/googleapis/mcp-toolbox/commit/b824abe72fbf518ec91fb12e5270c0a19e776d2f))
* **sdk:** Make ClientSession optional when initializing ToolboxClient ([#55](https://github.com/googleapis/mcp-toolbox/issues/55)) ([26347b5](https://github.com/googleapis/mcp-toolbox/commit/26347b5a5e71434d7bd2b7a9e6458247e75e3969))
* Support requesting a single tool ([#56](https://github.com/googleapis/mcp-toolbox/issues/56)) ([efafba9](https://github.com/googleapis/mcp-toolbox/commit/efafba9033e046905552f149f59893a4fad41afb))


### Bug Fixes

* Correct source type validation for postgres-sql tool ([#47](https://github.com/googleapis/mcp-toolbox/issues/47)) ([52ebb43](https://github.com/googleapis/mcp-toolbox/commit/52ebb431b784d160508273492d904d3b101afeb9))
* **docs:** Correct outdated references to tool kinds ([#49](https://github.com/googleapis/mcp-toolbox/issues/49)) ([972888b](https://github.com/googleapis/mcp-toolbox/commit/972888b9d64e1fea1d9a56b13268235ea55b9d66))
* Handle content-type correctly ([#33](https://github.com/googleapis/mcp-toolbox/issues/33)) ([cf8112f](https://github.com/googleapis/mcp-toolbox/commit/cf8112f85610833f2f4f2817a65fc4f7cf2322d8))


### Miscellaneous Chores

* Release 0.0.2 ([#65](https://github.com/googleapis/mcp-toolbox/issues/65)) ([beea3c3](https://github.com/googleapis/mcp-toolbox/commit/beea3c32d94d605973ba06b71a37b7c1bd4787bf))

## 0.0.1 (2024-10-28)


### Features

* Add address and port flags ([#7](https://github.com/googleapis/mcp-toolbox/issues/7)) ([df9ad9e](https://github.com/googleapis/mcp-toolbox/commit/df9ad9e33f99e6e5b692d9a99c2a90fbe3667265))
* Add AlloyDB source and tool ([#23](https://github.com/googleapis/mcp-toolbox/issues/23)) ([fe92d02](https://github.com/googleapis/mcp-toolbox/commit/fe92d02ae2ac2e70769dd2ee177cab91233a01cd))
* Add basic CLI ([#5](https://github.com/googleapis/mcp-toolbox/issues/5)) ([1539ee5](https://github.com/googleapis/mcp-toolbox/commit/1539ee56dddbee3a19069ef887375e76503fbdbd))
* Add basic http server ([#6](https://github.com/googleapis/mcp-toolbox/issues/6)) ([e09ae30](https://github.com/googleapis/mcp-toolbox/commit/e09ae30a90083a3777f91dd661e5a85bacdd48ba))
* Add basic parsing from tools file ([#8](https://github.com/googleapis/mcp-toolbox/issues/8)) ([b9ba364](https://github.com/googleapis/mcp-toolbox/commit/b9ba364fb66a884178d207e57310e07cf8d6cff1))
* Add initial cloud sql pg invocation ([#14](https://github.com/googleapis/mcp-toolbox/issues/14)) ([3703176](https://github.com/googleapis/mcp-toolbox/commit/3703176fce110ebb999deeb73d6b3aba29dee276))
* Add Postgres source and tool ([#25](https://github.com/googleapis/mcp-toolbox/issues/25)) ([2742ed4](https://github.com/googleapis/mcp-toolbox/commit/2742ed48b8d52f748a9edbc520068e1b88d82758))
* Add preliminary parsing of parameters ([#13](https://github.com/googleapis/mcp-toolbox/issues/13)) ([27edd3b](https://github.com/googleapis/mcp-toolbox/commit/27edd3b5f671b2ce7677729fae4e56381271c990))
* Add support for array type parameters ([#26](https://github.com/googleapis/mcp-toolbox/issues/26)) ([3903e86](https://github.com/googleapis/mcp-toolbox/commit/3903e860bc67a7b385e316220ba4ea37e00c20f2))
* Add toolset configuration ([#12](https://github.com/googleapis/mcp-toolbox/issues/12)) ([59b4bc0](https://github.com/googleapis/mcp-toolbox/commit/59b4bc07f4b8521c188d10ed047eee817d19e424))
* Add Toolset manifest endpoint ([#11](https://github.com/googleapis/mcp-toolbox/issues/11)) ([61e7b78](https://github.com/googleapis/mcp-toolbox/commit/61e7b78ad8af2e51f824ced32d14234fa32da30a))
* **langchain-sdk:** Add Toolbox SDK for LangChain ([#22](https://github.com/googleapis/mcp-toolbox/issues/22)) ([0bcd4b6](https://github.com/googleapis/mcp-toolbox/commit/0bcd4b6e418a8e43f2b7b74a0969da171e2081bf))
* Stub basic control plane functionality  ([#9](https://github.com/googleapis/mcp-toolbox/issues/9)) ([336bdc4](https://github.com/googleapis/mcp-toolbox/commit/336bdc4d56580637afff2313bef64b50b148faca))


### Miscellaneous Chores

* Release 0.0.1 ([#31](https://github.com/googleapis/mcp-toolbox/issues/31)) ([1f24ddd](https://github.com/googleapis/mcp-toolbox/commit/1f24dddb4b24ff4336998bf43acaf4607a48ff66))


### Continuous Integration

* Add realease-please ([#15](https://github.com/googleapis/mcp-toolbox/issues/15)) ([17fbbb4](https://github.com/googleapis/mcp-toolbox/commit/17fbbb49b05996c2c43df4b72cf08488224c522a))
