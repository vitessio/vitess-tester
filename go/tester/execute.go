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
	"context"
	"errors"
	"fmt"

	"vitess.io/vitess/go/mysql"
	"vitess.io/vitess/go/test/endtoend/cluster"
	"vitess.io/vitess/go/test/endtoend/utils"
	"vitess.io/vitess/go/vt/vtgate/vindexes"

	"github.com/vitessio/vt/go/data"
)

const (
	defaultKeyspaceName = "mysqltest"
	defaultCellName     = "mysqltest"
)

func ExecuteTests(
	info ClusterInfo,
	cfg Config,
	s Suite,
	factory QueryRunnerFactory,
) (failed bool) {
	vschemaF := cfg.VSchemaFile
	if vschemaF == "" {
		vschemaF = cfg.VtExplainVschemaFile
	}

	for _, name := range cfg.Tests {
		errReporter := s.NewReporterForFile(name)
		if cfg.Verbose {
			errReporter = &loggingReporter{inner: errReporter}
		}
		vTester := NewTester(name, errReporter, info, cfg.OLAP, info.vschema, vschemaF, factory, cfg.Loader)
		err := vTester.Run()
		if err != nil {
			failed = true
			continue
		}
		failed = failed || errReporter.Failed()
		s.CloseReportForFile()
	}

	factory.Close()

	return failed
}

type ClusterInfo struct {
	clusterInstance *cluster.LocalProcessCluster
	vtParams        mysql.ConnParams
	mysqlParams     *mysql.ConnParams
	ksNames         []string
	vschema         *vindexes.VSchema
	closer          func()
}

func SetupCluster(cfg Config) (_ ClusterInfo, err error) {
	clusterInstance := cluster.NewCluster(defaultCellName, "localhost")

	defer func() {
		if err != nil {
			clusterInstance.Teardown()
		}
	}()

	// Start topo server
	err = clusterInstance.StartTopo()
	if err != nil {
		return ClusterInfo{}, err
	}

	if cfg.BackupDir != "" {
		clusterInstance.VtTabletExtraArgs = append(clusterInstance.VtTabletExtraArgs,
			"--backup_storage_implementation", "file",
			"--file_backup_storage_root", cfg.BackupDir)
	}

	var ksNames []string
	keyspaces, vschema, err := data.GetKeyspaces(cfg.VSchemaFile, cfg.VtExplainVschemaFile, defaultKeyspaceName, cfg.Sharded)
	exitIf(err, "failed to get keyspaces")
	for _, keyspace := range keyspaces {
		ksNames = append(ksNames, keyspace.Name)
		err := startKeyspace(cfg, vschema, keyspace, clusterInstance)
		if err != nil {
			return ClusterInfo{}, err
		}
	}

	// Start vtgate
	err = clusterInstance.StartVtgate()
	if err != nil {
		return ClusterInfo{}, err
	}

	if len(ksNames) == 0 {
		return ClusterInfo{}, errors.New("no keyspaces found in vschema")
	}

	vtParams := clusterInstance.GetVTParams(ksNames[0])

	var mysqlParams *mysql.ConnParams
	var closer func()
	if cfg.Compare {
		mysqlParams, closer, err = setupExternalMySQL(keyspaces, clusterInstance)
		if err != nil {
			return ClusterInfo{}, err
		}
	}

	return ClusterInfo{
		clusterInstance: clusterInstance,
		vtParams:        vtParams,
		mysqlParams:     mysqlParams,
		ksNames:         ksNames,
		vschema:         vschema,
		closer: func() {
			clusterInstance.Teardown()
			if closer != nil {
				closer()
			}
		},
	}, nil
}

func startKeyspace(cfg Config, vschema *vindexes.VSchema, keyspace *cluster.Keyspace, clusterInstance *cluster.LocalProcessCluster) error {
	vschemaKs, ok := vschema.Keyspaces[keyspace.Name]
	if !ok {
		return fmt.Errorf("keyspace '%s' not found in vschema", keyspace.Name)
	}

	if vschemaKs.Keyspace.Sharded {
		shardRanges := generateShardRanges(cfg.GetNumberOfShards())
		fmt.Printf("starting sharded keyspace: '%s' with shards %v\n", keyspace.Name, shardRanges)
		err := clusterInstance.StartKeyspace(*keyspace, shardRanges, 0, false)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("starting unsharded keyspace: '%s'\n", keyspace.Name)
		err := clusterInstance.StartUnshardedKeyspace(*keyspace, 0, false)
		if err != nil {
			return err
		}
	}
	return nil
}

func setupExternalMySQL(keyspaces []*cluster.Keyspace, clusterInstance *cluster.LocalProcessCluster) (_ *mysql.ConnParams, closer func(), err error) {
	// Create the mysqld server we will use to compare the results.
	// We go through all the keyspaces we found in the vschema, and
	// simply create the mysqld process during the first iteration with
	// the first database, following iterations will create new databases.
	var conn *mysql.Conn
	defer func() {
		if conn != nil {
			conn.Close()
		}
	}()

	var mysqlParamsValue mysql.ConnParams
	for i, keyspace := range keyspaces {
		if i > 0 {
			_, err = conn.ExecuteFetch(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s;", keyspace.Name), 0, false)
			if err != nil {
				return nil, nil, err
			}
			continue
		}

		mysqlParamsValue, closer, err = utils.NewMySQL(clusterInstance, keyspace.Name, "")
		if err != nil {
			return nil, nil, err
		}
		conn, err = mysql.Connect(context.Background(), &mysqlParamsValue)
		if err != nil {
			return nil, nil, err
		}
	}
	return &mysqlParamsValue, closer, nil
}

func generateShardRanges(numberOfShards int) []string {
	if numberOfShards <= 0 {
		return []string{}
	}

	if numberOfShards == 1 {
		return []string{"-"}
	}

	ranges := make([]string, numberOfShards)
	step := 0x100 / numberOfShards

	for i := range numberOfShards {
		start := i * step
		end := (i + 1) * step

		switch i {
		case 0:
			ranges[i] = fmt.Sprintf("-%02x", end)
		case numberOfShards - 1:
			ranges[i] = fmt.Sprintf("%02x-", start)
		default:
			ranges[i] = fmt.Sprintf("%02x-%02x", start, end)
		}
	}

	return ranges
}
