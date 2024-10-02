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
	"encoding/json"
	"fmt"
	"github.com/vitessio/vitess-tester/go/tools"
	"github.com/vitessio/vitess-tester/go/typ"

	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pingcap/errors"
	log "github.com/sirupsen/logrus"
	"vitess.io/vitess/go/mysql"
	"vitess.io/vitess/go/test/endtoend/cluster"
	"vitess.io/vitess/go/test/endtoend/utils"
	"vitess.io/vitess/go/vt/sqlparser"
	"vitess.io/vitess/go/vt/vtgate/vindexes"
)

type (
	Tester struct {
		name string

		clusterInstance       *cluster.LocalProcessCluster
		vtParams, mysqlParams mysql.ConnParams
		curr                  utils.MySQLCompare

		skipBinary  string
		skipVersion int
		skipNext    bool
		olap        bool
		ksNames     []string
		vschema     vindexes.VSchema
		vschemaFile string
		vexplain    string

		// check expected error, use --error before the statement
		// we only care if an error is returned, not the exact error message.
		expectedErrs bool

		reporter             Reporter
		alreadyWrittenTraces bool // we need to keep track of it is the first trace or not, to add commas in between traces

		qr QueryRunner
	}

	QueryRunner interface {
		runQuery(q tools.Query, expectedErrs bool, ast sqlparser.Statement) error
	}

	QueryRunnerFactory interface {
		NewQueryRunner(reporter Reporter, handleCreateTable CreateTableHandler, comparer utils.MySQLCompare) QueryRunner
		Close()
	}
)

func NewTester(
	name string,
	reporter Reporter,
	clusterInstance *cluster.LocalProcessCluster,
	vtParams, mysqlParams mysql.ConnParams,
	olap bool,
	ksNames []string,
	vschema vindexes.VSchema,
	vschemaFile string,
	factory QueryRunnerFactory,
) *Tester {
	t := &Tester{
		name:            name,
		reporter:        reporter,
		vtParams:        vtParams,
		mysqlParams:     mysqlParams,
		clusterInstance: clusterInstance,
		ksNames:         ksNames,
		vschema:         vschema,
		vschemaFile:     vschemaFile,
		olap:            olap,
	}

	mcmp, err := utils.NewMySQLCompare(t.reporter, t.vtParams, t.mysqlParams)
	if err != nil {
		panic(err.Error())
	}
	t.curr = mcmp
	t.qr = factory.NewQueryRunner(reporter, t.handleCreateTable, mcmp)

	return t
}

func (t *Tester) preProcess() {
	if t.olap {
		_, err := t.curr.VtConn.ExecuteFetch("set workload = 'olap'", 0, false)
		if err != nil {
			panic(err)
		}
	}
}

func (t *Tester) postProcess() {
	r, err := t.curr.MySQLConn.ExecuteFetch("show tables", 1000, true)
	if err != nil {
		panic(err)
	}
	for _, row := range r.Rows {
		t.curr.Exec(fmt.Sprintf("drop table %s", row[0].ToString()))
	}
	t.curr.Close()
}

var PERM os.FileMode = 0755

func (t *Tester) getVschema() func() []byte {
	return func() []byte {
		httpClient := &http.Client{Timeout: 5 * time.Second}
		resp, err := httpClient.Get(t.clusterInstance.VtgateProcess.VSchemaURL)
		if err != nil {
			log.Errorf(err.Error())
			return nil
		}
		defer resp.Body.Close()
		res, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Errorf(err.Error())
			return nil
		}

		return res
	}
}

func (t *Tester) Run() error {
	t.preProcess()
	if t.autoVSchema() {
		defer t.postProcess()
	}
	data, err := tools.ReadData(t.name)
	if err != nil {
		return err
	}

	queries, err := tools.LoadQueries(data)
	if err != nil {
		t.reporter.AddFailure(err)
		return err
	}

	for _, q := range queries {
		switch q.Type {
		// no-ops
		case typ.Q_ENABLE_QUERY_LOG,
			typ.Q_DISABLE_QUERY_LOG,
			typ.Q_ECHO,
			typ.Q_DISABLE_WARNINGS,
			typ.Q_ENABLE_WARNINGS,
			typ.Q_ENABLE_INFO,
			typ.Q_DISABLE_INFO,
			typ.Q_ENABLE_RESULT_LOG,
			typ.Q_DISABLE_RESULT_LOG,
			typ.Q_SORTED_RESULT,
			typ.Q_REPLACE_REGEX:
			// do nothing
		case typ.Q_SKIP:
			t.skipNext = true
		case typ.Q_BEGIN_CONCURRENT, typ.Q_END_CONCURRENT, typ.Q_CONNECT, typ.Q_CONNECTION, typ.Q_DISCONNECT, typ.Q_LET, typ.Q_REPLACE_COLUMN:
			t.reporter.AddFailure(fmt.Errorf("%s not supported", q.Type.String()))
		case typ.Q_SKIP_IF_BELOW_VERSION:
			strs := strings.Split(q.Query, " ")
			if len(strs) != 3 {
				t.reporter.AddFailure(fmt.Errorf("incorrect syntax for typ.Q_SKIP_IF_BELOW_VERSION in: %v", q.Query))
				continue
			}
			t.skipBinary = strs[1]
			var err error
			t.skipVersion, err = strconv.Atoi(strs[2])
			if err != nil {
				t.reporter.AddFailure(err)
				continue
			}
		case typ.Q_ERROR:
			t.expectedErrs = true
		case typ.Q_VEXPLAIN:
			strs := strings.Split(q.Query, " ")
			if len(strs) != 2 {
				t.reporter.AddFailure(fmt.Errorf("incorrect syntax for typ.Q_VEXPLAIN in: %v", q.Query))
				continue
			}

			t.vexplain = strs[1]
		case typ.Q_WAIT_FOR_AUTHORITATIVE:
			t.waitAuthoritative(q.Query)
		case typ.Q_QUERY:
			if t.vexplain != "" {
				result, err := t.curr.VtConn.ExecuteFetch(fmt.Sprintf("vexplain %s %s", t.vexplain, q.Query), -1, false)
				t.vexplain = ""
				if err != nil {
					t.reporter.AddFailure(err)
				}

				t.reporter.AddInfo(fmt.Sprintf("VExplain Output:\n %s\n", result.Rows[0][0].ToString()))
			}

			t.runQuery(q)
		case typ.Q_REMOVE_FILE:
			err = os.Remove(strings.TrimSpace(q.Query))
			if err != nil {
				return errors.Annotate(err, "failed to remove file")
			}
		default:
			t.reporter.AddFailure(fmt.Errorf("%s not supported", q.Type.String()))
		}
	}
	fmt.Printf("%s\n", t.reporter.Report())

	return nil
}

func (t *Tester) runQuery(q tools.Query) {
	if t.skipNext {
		t.skipNext = false
		return
	}
	if t.skipBinary != "" {
		okayToRun := utils.BinaryIsAtLeastAtVersion(t.skipVersion, t.skipBinary)
		t.skipBinary = ""
		if !okayToRun {
			return
		}
	}
	t.reporter.AddTestCase(q.Query, q.Line)
	parser := sqlparser.NewTestParser()
	ast, err := parser.Parse(q.Query)
	if err != nil {
		t.reporter.AddFailure(err)
		return
	}

	err = t.qr.runQuery(q, t.expectedErrs, ast)
	if err != nil {
		t.reporter.AddFailure(err)
	}
	t.reporter.EndTestCase()
	// clear expected errors and current query after we execute any query
	t.expectedErrs = false
}

func (t *Tester) findTable(name string) (ks string, err error) {
	for ksName, ksSchema := range t.vschema.Keyspaces {
		for _, table := range ksSchema.Tables {
			if table.Name.String() == name {
				if ks != "" {
					return "", fmt.Errorf("table %s found in multiple keyspaces", name)
				}
				ks = ksName
			}
		}
	}
	if ks == "" {
		return "", fmt.Errorf("table %s not found in any keyspace", name)
	}
	return ks, nil
}

func (t *Tester) waitAuthoritative(query string) {
	var tblName, ksName string
	strs := strings.Split(query, " ")
	switch len(strs) {
	case 2:
		tblName = strs[1]
		var err error
		ksName, err = t.findTable(tblName)
		if err != nil {
			t.reporter.AddFailure(err)
			return
		}
	case 3:
		tblName = strs[1]
		ksName = strs[2]

	default:
		t.reporter.AddFailure(fmt.Errorf("expected table name and keyspace for wait_authoritative in: %v", query))
	}

	log.Infof("Waiting for authoritative schema for table %s", tblName)
	err := utils.WaitForAuthoritative(t.reporter, ksName, tblName, t.clusterInstance.VtgateProcess.ReadVSchema)
	if err != nil {
		t.reporter.AddFailure(fmt.Errorf("failed to wait for authoritative schema for table %s: %v", tblName, err))
	}
}

func newPrimaryKeyIndexDefinitionSingleColumn(name sqlparser.IdentifierCI) *sqlparser.IndexDefinition {
	index := &sqlparser.IndexDefinition{
		Info: &sqlparser.IndexInfo{
			Name: sqlparser.NewIdentifierCI("PRIMARY"),
			Type: sqlparser.IndexTypePrimary,
		},
		Columns: []*sqlparser.IndexColumn{{Column: name}},
	}
	return index
}

func (t *Tester) autoVSchema() bool {
	return t.vschemaFile == ""
}

func getShardingKeysForTable(create *sqlparser.CreateTable) (sks []sqlparser.IdentifierCI) {
	var allIdCI []sqlparser.IdentifierCI
	// first we normalize the primary keys
	for _, col := range create.TableSpec.Columns {
		if col.Type.Options.KeyOpt == sqlparser.ColKeyPrimary {
			create.TableSpec.Indexes = append(create.TableSpec.Indexes, newPrimaryKeyIndexDefinitionSingleColumn(col.Name))
			col.Type.Options.KeyOpt = sqlparser.ColKeyNone
		}
		allIdCI = append(allIdCI, col.Name)
	}

	// and now we can fetch the primary keys
	for _, index := range create.TableSpec.Indexes {
		if index.Info.Type == sqlparser.IndexTypePrimary {
			for _, column := range index.Columns {
				sks = append(sks, column.Column)
			}
		}
	}

	// if we have no primary keys, we'll use all columns as the sharding keys
	if len(sks) == 0 {
		sks = allIdCI
	}
	return
}

func (t *Tester) handleCreateTable(create *sqlparser.CreateTable) func() {
	sks := getShardingKeysForTable(create)

	shardingKeys := &vindexes.ColumnVindex{
		Columns: sks,
		Name:    "xxhash",
		Type:    "xxhash",
	}

	ks := t.vschema.Keyspaces[t.ksNames[0]]
	tableName := create.Table.Name
	ks.Tables[tableName.String()] = &vindexes.Table{
		Name:           tableName,
		Keyspace:       ks.Keyspace,
		ColumnVindexes: []*vindexes.ColumnVindex{shardingKeys},
	}

	ksJson, err := json.Marshal(ks)
	if err != nil {
		panic(err)
	}

	err = t.clusterInstance.VtctldClientProcess.ApplyVSchema(t.ksNames[0], string(ksJson))
	if err != nil {
		panic(err)
	}

	return func() {
		err := utils.WaitForAuthoritative(t.reporter, t.ksNames[0], create.Table.Name.String(), t.clusterInstance.VtgateProcess.ReadVSchema)
		if err != nil {
			panic(err)
		}
	}
}
