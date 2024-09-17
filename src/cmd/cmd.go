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

package cmd

import (
	vitess_tester "github.com/vitessio/vitess-tester/src/vitess-tester"
	"vitess.io/vitess/go/mysql"
	"vitess.io/vitess/go/test/endtoend/cluster"
	"vitess.io/vitess/go/vt/vtgate/vindexes"
)

func ExecuteTests(
	clusterInstance *cluster.LocalProcessCluster,
	vtParams, mysqlParams mysql.ConnParams,
	fileNames []string,
	s vitess_tester.Suite,
	ksNames []string,
	vschemaFile, vtexplainVschemaFile string,
	vschema vindexes.VSchema,
	olap bool,
) (failed bool) {
	vschemaF := vschemaFile
	if vschemaF == "" {
		vschemaF = vtexplainVschemaFile
	}
	for _, name := range fileNames {
		errReporter := s.NewReporterForFile(name)
		vTester := vitess_tester.NewTester(name, errReporter, clusterInstance, vtParams, mysqlParams, olap, ksNames, vschema, vschemaF)
		err := vTester.Run()
		if err != nil {
			failed = true
			continue
		}
		failed = failed || errReporter.Failed()
		s.CloseReportForFile()
	}
	return
}
