# YDB-specific configuration for stroppy tests

To support the authentication methods specific to YDB Managed Service, stroppy uses the additional environment variables when running in *client* mode only:
* `YDB_SERVICE_ACCOUNT_KEY_FILE_CREDENTIALS` - the path to the service account key file. When configured, the key file is used to authenticate the connection.
* `YDB_METADATA_CREDENTIALS` - when set to `1`, the service account key associated with the Cloud compute instance is used to authenticate the connection.
* `YDB_ACCESS_TOKEN_CREDENTIALS` - YDB access token. When configured, the access token is passed as is to authenticate the connection.
* `YDB_TLS_CERTIFICATES_FILE` - PEM-encoded file with custom TLS certificate(s) to be used for GRPCS connections.

In addition, there are the following YDB-specific environment variables:
* `YDB_STROPPY_PARTITIONS_COUNT` - [`AUTO_PARTITIONING_MIN_PARTITIONS_COUNT`](https://ydb.tech/en/docs/concepts/datamodel/table#auto_partitioning_partition_size_mb) setting value for `account` and `transfer` tables. This setting only affects the `pop` operation mode.
* `YDB_STROPPY_PARTITIONS_SIZE` - [`AUTO_PARTITIONING_PARTITION_SIZE_MB`](https://ydb.tech/en/docs/concepts/datamodel/table#auto_partitioning_min_partitions_count) setting value for `account` and `transfer` tables. This setting only affects the `pop` operation mode.
* `YDB_STROPPY_HASH_TRANSFER_ID` - when set to `1`, the actual value of `transfer_id` field in the `transfer` table is replaced with its SHA-1 hash code (Base-64 encoded). This setting only affects the `pay` operation mode.

Typical "client" operation modes command examples:

```bash
export YDB_DB='grpc://stroppy:passw0rd@ycydb-d1:2136?database=/Root/testdb'
./stroppy pop --dbtype ydb --url "$YDB_DB" -n 1000000 -w 8000 --run-type client
./stroppy pay --dbtype ydb --url "$YDB_DB" -n 10000000 -w 8000 --run-type client
```