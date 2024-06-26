// Copyright 2020 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"vitess.io/vitess/go/mysql"
	"vitess.io/vitess/go/test/endtoend/cluster"
	"vitess.io/vitess/go/test/endtoend/utils"
	"vitess.io/vitess/go/vt/sqlparser"
	"vitess.io/vitess/go/vt/vtgate/vindexes"

	vitess_tester "github.com/vitessio/vitess-tester/src/vitess-tester"
)

var (
	logLevel    string
	sharded     bool
	olap        bool
	vschemaFile string
	xunit       bool
)

func init() {
	flag.BoolVar(&olap, "olap", false, "Use OLAP to run the queries.")
	flag.StringVar(&logLevel, "log-level", "error", "The log level of vitess-tester: info, warn, error, debug.")
	flag.BoolVar(&sharded, "sharded", false, "run all tests on a sharded keyspace")
	flag.StringVar(&vschemaFile, "vschema", "", "Disable auto-vschema by providing your own vschema file")
	flag.BoolVar(&xunit, "xunit", false, "Get output in an xml file instead of errors directory")
}

func executeTests(clusterInstance *cluster.LocalProcessCluster, vtParams, mysqlParams mysql.ConnParams, fileNames []string, s vitess_tester.Suite) (failed bool) {
	for _, name := range fileNames {
		errReporter := s.NewReporterForFile(name)
		vTester := vitess_tester.NewTester(name, errReporter, clusterInstance, vtParams, mysqlParams, olap, keyspaceName, vschema, vschemaFile)
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

var (
	keyspaceName = "mysqltest"
	cell         = "mysqltest"
)

type rawKeyspaceVindex struct {
	Keyspaces map[string]interface{} `json:"keyspaces"`
}

type hashVindex struct {
	vindexes.Hash
	Type string `json:"type"`
}

func (hv hashVindex) String() string {
	return "xxhash"
}

var vschema = vindexes.VSchema{
	Keyspaces: map[string]*vindexes.KeyspaceSchema{
		keyspaceName: {
			Keyspace: &vindexes.Keyspace{},
			Tables:   map[string]*vindexes.Table{},
			Vindexes: map[string]vindexes.Vindex{
				"xxhash": &hashVindex{Type: "xxhash"},
			},
			Views: map[string]sqlparser.SelectStatement{},
		},
	},
}

func setupCluster(sharded bool) (clusterInstance *cluster.LocalProcessCluster, vtParams, mysqlParams mysql.ConnParams, close func()) {
	clusterInstance = cluster.NewCluster(cell, "localhost")

	// Start topo server
	err := clusterInstance.StartTopo()
	if err != nil {
		clusterInstance.Teardown()
		panic(err)
	}

	if sharded {
		keyspace := getKeyspace()

		println("starting sharded keyspace")
		err = clusterInstance.StartKeyspace(*keyspace, []string{"-80", "80-"}, 0, false)
		if err != nil {
			fmt.Printf("Failed to start vitess cluster: %s\n", err.Error())
			os.Exit(1)
		}
	} else {
		// Start Unsharded keyspace
		ukeyspace := &cluster.Keyspace{
			Name: keyspaceName,
		}
		println("starting unsharded keyspace")
		err = clusterInstance.StartUnshardedKeyspace(*ukeyspace, 0, false)
		if err != nil {
			clusterInstance.Teardown()
			panic(err)
		}
	}

	// Start vtgate
	err = clusterInstance.StartVtgate()
	if err != nil {
		clusterInstance.Teardown()
		panic(err)
	}

	vtParams = clusterInstance.GetVTParams(keyspaceName)

	// create mysql instance and connection parameters
	conn, closer, err := utils.NewMySQL(clusterInstance, keyspaceName, "")
	if err != nil {
		clusterInstance.Teardown()
		panic(err)
	}
	mysqlParams = conn

	return clusterInstance, vtParams, mysqlParams, func() {
		clusterInstance.Teardown()
		closer()
	}
}

func getKeyspace() *cluster.Keyspace {
	var ksSchema []byte
	var err error
	if vschemaFile != "" {
		// Get the struct representation of the vschema, this is used
		// to have an updated view of the vschema at all time, although
		// we could probably remove this in the future if a vschema file
		// is provided and just send a call to vtctld to get the vschema.
		formal, err := vindexes.LoadFormal(vschemaFile)
		if err != nil {
			panic(err.Error())
		}
		vschema = *(vindexes.BuildVSchema(formal, sqlparser.NewTestParser()))
		if len(vschema.Keyspaces) != 1 {
			panic("please use only one keyspace when giving your own vschema")
		}
		for name := range vschema.Keyspaces {
			keyspaceName = name
		}

		// Get the full string representation of the keyspace vschema
		// we want to send the complete string to StartKeyspace instead
		// of marshalling the vschema structure back and forth, this will
		// prevent an issue where fields are missing (like the vindex type)
		// in the structure. FWIW, in the Vitess' end-to-end tests we also
		// send the text version of the vschema to StartKeyspace.
		vschemaContent, err := os.ReadFile(vschemaFile)
		if err != nil {
			panic(err.Error())
		}
		var rk rawKeyspaceVindex
		err = json.Unmarshal(vschemaContent, &rk)
		if err != nil {
			panic(err.Error())
		}
		for _, v := range rk.Keyspaces {
			ksSchema, err = json.Marshal(v)
			if err != nil {
				panic(err.Error())
			}
		}
	} else {
		vschema.Keyspaces[keyspaceName].Keyspace.Sharded = true
		ksSchema, err = json.Marshal(vschema.Keyspaces[keyspaceName])
		if err != nil {
			panic(err)
		}
	}
	keyspace := &cluster.Keyspace{
		Name:    keyspaceName,
		VSchema: string(ksSchema),
	}
	return keyspace
}

func main() {
	flag.Parse()
	tests := flag.Args()

	err := vitess_tester.CheckEnvironment()
	if err != nil {
		fmt.Println("Fatal error:")
		fmt.Println(err.Error())
		os.Exit(1)
	}

	if ll := os.Getenv("LOG_LEVEL"); ll != "" {
		logLevel = ll
	}
	if logLevel != "" {
		ll, err := log.ParseLevel(logLevel)
		if err != nil {
			log.Errorf("error parsing log level %s: %v", logLevel, err)
		}
		log.SetLevel(ll)
	}

	if len(tests) == 0 {
		log.Errorf("no tests specified")
		os.Exit(1)
	}

	log.Infof("running tests: %v", tests)

	clusterInstance, vtParams, mysqlParams, closer := setupCluster(sharded)
	defer closer()

	// remove errors folder if exists
	err = os.RemoveAll("errors")
	if err != nil {
		panic(err.Error())
	}

	var reporterSuite vitess_tester.Suite
	if xunit {
		reporterSuite = vitess_tester.NewXMLTestSuite()
	} else {
		reporterSuite = vitess_tester.NewFileReporterSuite()
	}
	failed := executeTests(clusterInstance, vtParams, mysqlParams, tests, reporterSuite)
	outputFile := reporterSuite.Close()
	if failed {
		log.Errorf("some tests failed 😭\nsee errors in %v", outputFile)
		os.Exit(1)
	}
	println("Great, All tests passed")
}
