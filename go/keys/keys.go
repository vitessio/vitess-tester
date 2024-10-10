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

package keys

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"sort"

	"vitess.io/vitess/go/vt/sqlparser"
	"vitess.io/vitess/go/vt/vtgate/planbuilder/operators"
	"vitess.io/vitess/go/vt/vtgate/planbuilder/plancontext"
	"vitess.io/vitess/go/vt/vtgate/semantics"

	"github.com/vitessio/vt/go/data"
	"github.com/vitessio/vt/go/typ"

	querypb "vitess.io/vitess/go/vt/proto/query"
)

func Run(fileName string) error {
	return run(os.Stdout, fileName)
}

func run(out io.Writer, fileName string) error {
	si := &schemaInfo{
		tables: make(map[string]columns),
	}
	ql := &queryList{
		queries: make(map[string]*QueryAnalysisResult),
	}
	queries, err := data.LoadQueries(fileName)
	if err != nil {
		return err
	}

	skip := false
	for _, query := range queries {
		switch query.Type {
		case typ.Skip, typ.Error, typ.VExplain:
			skip = true
		case typ.RemoveFile, typ.Unknown:
			return fmt.Errorf("unknown command type: %s", query.Type)
		case typ.Comment, typ.CommentWithCommand, typ.EmptyLine, typ.WaitForAuthoritative, typ.SkipIfBelowVersion:
			// no-op for keys
		case typ.Query:
			if skip {
				skip = false
				continue
			}
			process(query, si, ql)
		}
	}

	return ql.writeJSONTo(out)
}

func process(q data.Query, si *schemaInfo, ql *queryList) {
	ast, bv, err := sqlparser.NewTestParser().Parse2(q.Query)
	if err != nil {
		panic(err) // TODO: write this to the json output
	}

	switch ast := ast.(type) {
	case *sqlparser.CreateTable:
		si.handleCreateTable(ast)
	case sqlparser.Statement:
		st, err := semantics.Analyze(ast, "ks", si)
		if err != nil {
			panic(err) // TODO: write this to the json output
		}
		ctx := &plancontext.PlanningContext{
			ReservedVars: sqlparser.NewReservedVars("", bv),
			SemTable:     st,
		}
		ql.processQuery(ctx, ast, q)
	}
}

type queryList struct {
	queries map[string]*QueryAnalysisResult
}

func (ql *queryList) processQuery(ctx *plancontext.PlanningContext, ast sqlparser.Statement, q data.Query) {
	bv := make(map[string]*querypb.BindVariable)
	err := sqlparser.Normalize(ast, ctx.ReservedVars, bv)
	if err != nil {
		panic("oh no")
	}
	structure := sqlparser.CanonicalString(ast)
	r, found := ql.queries[structure]
	if found {
		r.UsageCount++
		r.LineNumbers = append(r.LineNumbers, q.Line)
		return
	}

	var tableNames []string
	for _, t := range ctx.SemTable.Tables {
		rtbl, ok := t.(*semantics.RealTable)
		if !ok {
			continue
		}
		tableNames = append(tableNames, rtbl.Table.Name.String())
	}

	result := operators.GetVExplainKeys(ctx, ast)
	ql.queries[structure] = &QueryAnalysisResult{
		QueryStructure:  structure,
		StatementType:   result.StatementType,
		UsageCount:      1,
		LineNumbers:     []int{q.Line},
		TableName:       tableNames,
		GroupingColumns: result.GroupingColumns,
		JoinColumns:     result.JoinColumns,
		FilterColumns:   result.FilterColumns,
	}
}

// writeJsonTo writes the query list, sorted by the first line number of the query, to the given writer.
func (ql *queryList) writeJSONTo(w io.Writer) error {
	values := slices.Collect(maps.Values(ql.queries))
	sort.Slice(values, func(i, j int) bool {
		return values[i].LineNumbers[0] < values[j].LineNumbers[0]
	})

	_, err := fmt.Fprint(w, "[\n  ")
	if err != nil {
		return err
	}

	for i, q := range values {
		if i > 0 {
			_, err = fmt.Fprint(w, ",\n  ")
			if err != nil {
				return err
			}
		}
		jsonData, err := json.MarshalIndent(q, "  ", "  ")
		if err != nil {
			return err
		}
		_, err = w.Write(jsonData)
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprint(w, "\n]")
	return err
}

// QueryAnalysisResult represents the result of analyzing a query in a query log. It contains the query structure, the number of
// times the query was used, the line numbers where the query was used, the table name, grouping columns, join columns,
// filter columns, and the statement type.
type QueryAnalysisResult struct {
	QueryStructure  string   `json:"queryStructure"`
	UsageCount      int      `json:"usageCount"`
	LineNumbers     []int    `json:"lineNumbers"`
	TableName       []string `json:"tableName,omitempty"`
	GroupingColumns []string `json:"groupingColumns,omitempty"`
	JoinColumns     []string `json:"joinColumns,omitempty"`
	FilterColumns   []string `json:"filterColumns,omitempty"`
	StatementType   string   `json:"statementType"`
}
