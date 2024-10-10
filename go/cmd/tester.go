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
	"github.com/spf13/cobra"

	vttester "github.com/vitessio/vt/go/tester"
)

func testerCmd() *cobra.Command {
	var cfg vttester.Config

	cmd := &cobra.Command{
		Use:     "tester ",
		Short:   "Test the given workload against both Vitess and MySQL.",
		Example: "vt tester ",
		Args:    cobra.MinimumNArgs(1),
		Run: func(_ *cobra.Command, args []string) {
			cfg.Tests = args
			vttester.Run(cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.LogLevel, "log-level", "error", "The log level of vt tester: info, warn, error, debug.")
	cmd.Flags().StringVar(&cfg.TraceFile, "trace-file", "", "Do a vexplain trace on all queries and store the output in the given file.")
	cmd.Flags().StringVar(&cfg.VschemaFile, "vschema", "", "Disable auto-vschema by providing your own vschema file. This cannot be used with either -vtexplain-vschema or -sharded.")
	cmd.Flags().StringVar(&cfg.VtExplainVschemaFile, "vtexplain-vschema", "", "Disable auto-vschema by providing your own vtexplain vschema file. This cannot be used with either -vschema or -sharded.")

	cmd.Flags().BoolVar(&cfg.OLAP, "olap", false, "Use OLAP to run the queries.")
	cmd.Flags().BoolVar(&cfg.XUnit, "xunit", false, "Get output in an xml file instead of errors directory")
	cmd.Flags().BoolVar(&cfg.Sharded, "sharded", false, "Run all tests on a sharded keyspace and using auto-vschema. This cannot be used with either -vschema or -vtexplain-vschema.")
	cmd.Flags().IntVar(&cfg.NumberOfShards, "number-of-shards", 0, "Number of shards to use for the sharded keyspace.")

	return cmd
}
