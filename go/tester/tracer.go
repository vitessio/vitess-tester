/*
Copyright 2024 The Vitess Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tester

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"vitess.io/vitess/go/mysql"
	"vitess.io/vitess/go/test/endtoend/cluster"
	"vitess.io/vitess/go/test/endtoend/utils"
	"vitess.io/vitess/go/vt/sqlparser"
	"vitess.io/vitess/go/vt/vterrors"

	"github.com/vitessio/vt/go/data"
	"github.com/vitessio/vt/go/tester/state"
)

var (
	_ QueryRunner        = (*Tracer)(nil)
	_ QueryRunnerFactory = (*TracerFactory)(nil)
)

type (
	Tracer struct {
		traceFile            *os.File
		MySQLConn, VtConn    *mysql.Conn
		reporter             Reporter
		inner                QueryRunner
		alreadyWrittenTraces bool
	}
	TracerFactory struct {
		traceFile *os.File
		inner     QueryRunnerFactory
	}
)

func NewTracerFactory(traceFile *os.File, inner QueryRunnerFactory) *TracerFactory {
	return &TracerFactory{
		traceFile: traceFile,
		inner:     inner,
	}
}

func (t *TracerFactory) NewQueryRunner(
	reporter Reporter,
	handleCreateTable CreateTableHandler,
	comparer utils.MySQLCompare,
	cluster *cluster.LocalProcessCluster,
) QueryRunner {
	inner := t.inner.NewQueryRunner(reporter, handleCreateTable, comparer, cluster)
	return newTracer(t.traceFile, comparer.MySQLConn, comparer.VtConn, reporter, inner)
}

func (t *TracerFactory) Close() {
	_, err := t.traceFile.Write([]byte("]"))
	exitIf(err, "failed to write closing bracket")
	err = t.traceFile.Close()
	exitIf(err, "failed to close trace file")
}

func newTracer(traceFile *os.File,
	mySQLConn, vtConn *mysql.Conn,
	reporter Reporter,
	inner QueryRunner,
) QueryRunner {
	return &Tracer{
		traceFile: traceFile,
		MySQLConn: mySQLConn,
		VtConn:    vtConn,
		reporter:  reporter,
		inner:     inner,
	}
}

func (t *Tracer) runQuery(q data.Query, ast sqlparser.Statement, state *state.State) error {
	if sqlparser.IsDMLStatement(ast) && t.traceFile != nil && !state.IsErrorExpectedSet() && state.RunOnVitess() {
		// we don't want to run DMLs twice, so we just run them once while tracing
		var errs []error
		err := t.trace(q)
		if err != nil {
			errs = append(errs, err)
		}

		if state.RunOnMySQL() {
			// we need to run the DMLs on mysql as well
			_, err = t.MySQLConn.ExecuteFetch(q.Query, 10000, false)
			if err != nil {
				errs = append(errs, err)
			}
		}

		return vterrors.Aggregate(errs)
	}

	reference := state.IsReferenceSet()

	err := t.inner.runQuery(q, ast, state)
	if err != nil {
		return err
	}

	_, isSelect := ast.(sqlparser.SelectStatement)
	if reference || !state.RunOnVitess() || !(isSelect || sqlparser.IsDMLStatement(ast)) {
		return nil
	}

	return t.trace(q)
}

// trace writes the query and its trace (fetched from VtConn) as a JSON object into traceFile
func (t *Tracer) trace(query data.Query) error {
	// Marshal the query into JSON format for safe embedding
	queryJSON, err := json.Marshal(query.Query)
	if err != nil {
		return err
	}

	// Fetch the trace for the query using "vexplain trace"
	rs, err := t.VtConn.ExecuteFetch(fmt.Sprintf("vexplain trace %s", query.Query), 10000, false)
	if err != nil {
		return err
	}

	// Extract the trace result and format it with indentation for pretty printing
	var prettyTrace bytes.Buffer
	if err = json.Indent(&prettyTrace, []byte(rs.Rows[0][0].ToString()), "", "  "); err != nil {
		return err
	}

	// Construct the entire JSON entry in memory
	var traceEntry bytes.Buffer
	if t.alreadyWrittenTraces {
		traceEntry.WriteString(",") // Prepend a comma if there are already written traces
	}
	traceEntry.WriteString(fmt.Sprintf(`{"Query": %s, "LineNumber": "%d", "Trace": `, queryJSON, query.Line))
	traceEntry.Write(prettyTrace.Bytes()) // Add the formatted trace
	traceEntry.WriteString("}")           // Close the JSON object

	// Mark that at least one trace has been written
	t.alreadyWrittenTraces = true

	// Write the fully constructed JSON entry to the file
	if _, err = t.traceFile.Write(traceEntry.Bytes()); err != nil {
		return err
	}

	return nil
}
