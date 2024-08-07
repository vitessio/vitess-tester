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

package vitess_tester

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/pingcap/errors"
	log "github.com/sirupsen/logrus"
	"vitess.io/vitess/go/mysql"
	"vitess.io/vitess/go/test/endtoend/cluster"
	"vitess.io/vitess/go/test/endtoend/utils"
	"vitess.io/vitess/go/vt/sqlparser"
	"vitess.io/vitess/go/vt/vtgate/vindexes"
)

type Tester struct {
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

	reporter Reporter
}

func NewTester(
	name string,
	reporter Reporter,
	clusterInstance *cluster.LocalProcessCluster,
	vtParams, mysqlParams mysql.ConnParams,
	olap bool,
	ksNames []string,
	vschema vindexes.VSchema,
	vschemaFile string,
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
	return t
}

func (t *Tester) preProcess() {
	mcmp, err := utils.NewMySQLCompare(t, t.vtParams, t.mysqlParams)
	if err != nil {
		panic(err.Error())
	}
	t.curr = mcmp
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

func (t *Tester) addSuccess() {

}

func (t *Tester) Run() error {
	t.preProcess()
	if t.autoVSchema() {
		defer t.postProcess()
	}
	queries, err := t.loadQueries()
	if err != nil {
		t.reporter.AddFailure(t.vschema, err)
		return err
	}

	for _, q := range queries {
		switch q.tp {
		// no-ops
		case Q_ENABLE_QUERY_LOG,
			Q_DISABLE_QUERY_LOG,
			Q_ECHO,
			Q_DISABLE_WARNINGS,
			Q_ENABLE_WARNINGS,
			Q_ENABLE_INFO,
			Q_DISABLE_INFO,
			Q_ENABLE_RESULT_LOG,
			Q_DISABLE_RESULT_LOG,
			Q_SORTED_RESULT,
			Q_REPLACE_REGEX:
			// do nothing
		case Q_SKIP:
			t.skipNext = true
		case Q_BEGIN_CONCURRENT, Q_END_CONCURRENT, Q_CONNECT, Q_CONNECTION, Q_DISCONNECT, Q_LET, Q_REPLACE_COLUMN:
			t.reporter.AddFailure(t.vschema, fmt.Errorf("%s not supported", String(q.tp)))
		case Q_SKIP_IF_BELOW_VERSION:
			strs := strings.Split(q.Query, " ")
			if len(strs) != 3 {
				t.reporter.AddFailure(t.vschema, fmt.Errorf("incorrect syntax for Q_SKIP_IF_BELOW_VERSION in: %v", q.Query))
				continue
			}
			t.skipBinary = strs[1]
			var err error
			t.skipVersion, err = strconv.Atoi(strs[2])
			if err != nil {
				t.reporter.AddFailure(t.vschema, err)
				continue
			}
		case Q_ERROR:
			t.expectedErrs = true
		case Q_VEXPLAIN:
			strs := strings.Split(q.Query, " ")
			if len(strs) != 2 {
				t.reporter.AddFailure(t.vschema, fmt.Errorf("incorrect syntax for Q_VEXPLAIN in: %v", q.Query))
				continue
			}

			t.vexplain = strs[1]
		case Q_WAIT_FOR_AUTHORITATIVE:
			t.waitAuthoritative(q.Query)
		case Q_QUERY:
			if t.skipNext {
				t.skipNext = false
				continue
			}
			if t.skipBinary != "" {
				okayToRun := utils.BinaryIsAtLeastAtVersion(t.skipVersion, t.skipBinary)
				t.skipBinary = ""
				if !okayToRun {
					continue
				}
			}
			t.reporter.AddTestCase(q.Query, q.Line)
			if t.vexplain != "" {
				result, err := t.curr.VtConn.ExecuteFetch("vexplain "+t.vexplain+" "+q.Query, -1, false)
				t.vexplain = ""
				if err != nil {
					t.reporter.AddFailure(t.vschema, err)
					continue
				}

				t.reporter.AddInfo(fmt.Sprintf("VExplain Output:\n %s\n", result.Rows[0][0].ToString()))
			}
			if err = t.execute(q); err != nil && !t.expectedErrs {
				t.reporter.AddFailure(t.vschema, err)
			}
			t.reporter.EndTestCase()
			// clear expected errors and current query after we execute any query
			t.expectedErrs = false
		case Q_REMOVE_FILE:
			err = os.Remove(strings.TrimSpace(q.Query))
			if err != nil {
				return errors.Annotate(err, "failed to remove file")
			}
		default:
			t.reporter.AddFailure(t.vschema, fmt.Errorf("%s not supported", String(q.tp)))
		}
	}
	fmt.Printf("%s\n", t.reporter.Report())

	return nil
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
			t.reporter.AddFailure(t.vschema, err)
			return
		}
	case 3:
		tblName = strs[1]
		ksName = strs[2]

	default:
		t.reporter.AddFailure(t.vschema, fmt.Errorf("expected table name and keyspace for wait_authoritative in: %v", query))
	}

	log.Infof("Waiting for authoritative schema for table %s", tblName)
	err := utils.WaitForAuthoritative(t, ksName, tblName, t.clusterInstance.VtgateProcess.ReadVSchema)
	if err != nil {
		t.reporter.AddFailure(t.vschema, fmt.Errorf("failed to wait for authoritative schema for table %s: %v", tblName, err))
	}
}

func (t *Tester) loadQueries() ([]query, error) {
	data, err := t.readData()
	if err != nil {
		return nil, err
	}

	seps := bytes.Split(data, []byte("\n"))
	queries := make([]query, 0, len(seps))
	newStmt := true
	for i, v := range seps {
		v := bytes.TrimSpace(v)
		s := string(v)
		// we will skip # comment here
		if strings.HasPrefix(s, "#") {
			newStmt = true
			continue
		} else if strings.HasPrefix(s, "--") {
			queries = append(queries, query{Query: s, Line: i + 1})
			newStmt = true
			continue
		} else if len(s) == 0 {
			continue
		}

		if newStmt {
			queries = append(queries, query{Query: s, Line: i + 1})
		} else {
			lastQuery := queries[len(queries)-1]
			lastQuery = query{Query: fmt.Sprintf("%s\n%s", lastQuery.Query, s), Line: lastQuery.Line}
			queries[len(queries)-1] = lastQuery
		}

		// if the line has a ; in the end, we will treat new line as the new statement.
		newStmt = strings.HasSuffix(s, ";")
	}

	return ParseQueries(queries...)
}

func (t *Tester) readData() ([]byte, error) {
	if strings.HasPrefix(t.name, "http") {
		client := http.Client{}
		res, err := client.Get(t.name)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusOK {
			return nil, errors.Errorf("failed to get data from %s, status code %d", t.name, res.StatusCode)
		}
		defer res.Body.Close()
		return io.ReadAll(res.Body)
	}
	return os.ReadFile(t.name)
}

func (t *Tester) execute(query query) error {
	if len(query.Query) == 0 {
		return nil
	}

	err := t.executeStmt(query.Query)

	if err != nil {
		return errors.Trace(errors.Errorf("run \"%v\" at line %d err %v", query.Query, query.Line, err))
	}
	// clear expected errors after we execute the first query
	t.expectedErrs = false

	if err != nil {
		return errors.Trace(errors.Errorf("run \"%v\" at line %d err %v", query.Query, query.Line, err))
	}

	return errors.Trace(err)
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

func (t *Tester) executeStmt(query string) error {
	parser := sqlparser.NewTestParser()
	ast, err := parser.Parse(query)
	if err != nil {
		return err
	}
	_, commentOnly := ast.(*sqlparser.CommentOnly)
	if commentOnly {
		return nil
	}

	log.Debugf("executeStmt: %s", query)
	create, isCreateStatement := ast.(*sqlparser.CreateTable)
	handleVSchema := isCreateStatement && !t.expectedErrs && t.autoVSchema()
	if handleVSchema {
		t.handleCreateTable(create)
	}

	switch {
	case t.expectedErrs:
		_, err := t.curr.ExecAllowAndCompareError(query, utils.CompareOptions{CompareColumnNames: true})
		if err == nil {
			// If we expected an error, but didn't get one, return an error
			return fmt.Errorf("expected error, but got none")
		}
	default:
		_ = t.curr.Exec(query)
	}

	if handleVSchema {
		err = utils.WaitForAuthoritative(t, t.ksNames[0], create.Table.Name.String(), t.clusterInstance.VtgateProcess.ReadVSchema)
		if err != nil {
			panic(err)
		}
	}
	return nil
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

func (t *Tester) handleCreateTable(create *sqlparser.CreateTable) {
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
}

func (t *Tester) Errorf(format string, args ...interface{}) {
	t.reporter.AddFailure(t.vschema, errors.Errorf(format, args...))
}

func (t *Tester) FailNow() {
	// we don't need to do anything here
}

func (t *Tester) Helper() {}
