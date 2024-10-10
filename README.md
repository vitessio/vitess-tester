# VT Utilities

The `vt` binary encapsulates several utility tools for Vitess, providing a comprehensive suite for testing, summarizing, and query analysis.

## Tools Included
- **`vt tester`**: A testing utility using the same test files as the [MySQL Test Framework](https://github.com/mysql/mysql-server/tree/8.0/mysql-test). It compares the results of identical queries executed on both MySQL and Vitess (vtgate), helping to ensure compatibility.
- **`vt benchstat`**: A tool used to summarize or compare trace logs or key logs for deeper analysis.
- **`vt keys`**: A utility that analyzes query logs and provides information about queries, tables, and column usage. It integrates with `vt benchstat` for summarizing and comparing query logs.

## Installation
You can install `vt` using the following command:

```bash
go install github.com/vitessio/vt/go/vt@latest
```

## Testing Methodology

To verify compatibility and correctness, the testing strategy involves running identical queries on both MySQL and vtgate, followed by a comparison of results. The process includes:

1. **Query Execution**: Each test query is executed on both MySQL and vtgate.
2. **Result Comparison**: The returned data, result set structure (column types, order), and errors are compared.
3. **Error Handling**: Any errors are checked to ensure vtgate produces the same error types as MySQL.

This dual-testing strategy ensures high confidence in vtgate's compatibility with MySQL.

### Sharded Testing Strategy
Vitess operates in a sharded environment, presenting unique challenges, especially during schema changes (DDL). The `vt tester` tool handles these by converting DDL statements into VSchema commands.

Here’s an example of running `vt tester`:

```bash
vt tester --sharded t/basic.test  # Runs a test on a sharded database
```

Custom schemas and configurations can be applied using directives. 
Run `vt tester --help`, and check out `directives.test` for more examples.

## Tracing and Key Analysis

`vt tester` can also operate in tracing mode to generate a trace of the query execution plan using the `vexplain trace` tool for detailed execution analysis.

To run `vt tester` with tracing:

```bash
vt tester --sharded --trace=trace-log.json t/tpch.test
```

The generated trace logs can be summarized or compared using `vt benchstat`:

- **Summarize a trace log**:

  ```bash
  vt benchstat trace-log.json
  ```

- **Compare two trace logs**:

  ```bash
  vt benchstat trace-log1.json trace-log2.json
  ```

## Key Analysis Workflow

`vt keys` analyzes a query log and outputs detailed information about table and column usage in queries. This data can be summarized using `vt benchstat`. Here's a typical workflow:

1. **Run `vt keys` to analyze the query log**:

   ```bash
   vt keys t/tpch.test > keys-log.json
   ```

   This command generates a `keys-log.json` file that contains a detailed analysis of table and column usage from the query log.

2. **Summarize the `keys-log` using `vt benchstat`**:

   ```bash
   vt benchstat keys-log.json
   ```

   This command summarizes the key analysis, providing insight into which tables and columns are used across queries, and how frequently they are involved in filters, groupings, and joins.

3. **Example of output from the summarized key analysis**:

   ```
   Summary from trace file testdata/keys-log.json
   Table: customer used in 8 queries
   +--------------+----------+------------+--------+
   |    Column    | Filter % | Grouping % | Join % |
   +--------------+----------+------------+--------+
   | c_acctbal    | 0.00%    | 12.50%     | 0.00%  |
   | c_address    | 0.00%    | 12.50%     | 0.00%  |
   | c_comment    | 0.00%    | 12.50%     | 0.00%  |
   | c_custkey    | 0.00%    | 37.50%     | 87.50% |
   | c_mktsegment | 12.50%   | 0.00%      | 0.00%  |
   | c_name       | 0.00%    | 25.00%     | 0.00%  |
   | c_nationkey  | 0.00%    | 0.00%      | 50.00% |
   | c_phone      | 0.00%    | 12.50%     | 0.00%  |
   +--------------+----------+------------+--------+
   ```

   This summary shows the columns of the `customer` table, along with their usage percentages in filters, groupings, and joins across the queries in the log.

## Contributing

We welcome contributions in the following areas:

- Writing documentation on how to use the framework
- Triaging issues
- Submitting new test cases
- Fixing bugs in the test framework
- Adding features from the MySQL test framework that are missing in this implementation

For more details, see our [CONTRIBUTING.md](./CONTRIBUTING.md).

## License

Vitess Tester is licensed under the Apache 2.0 license. See the [LICENSE](./LICENSE) file for more information.

## Acknowledgments

Vitess Tester started as a fork from [pingcap/mysql-tester](https://github.com/pingcap/mysql-tester). We thank the original authors for their foundational work.