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

package summarize

import (
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
)

type (
	// TracedQuery represents the structure of each element in the JSON file
	TracedQuery struct {
		Trace      Trace  `json:"Trace"`
		Query      string `json:"Query"`
		LineNumber string `json:"LineNumber"`
	}

	// Trace represents the recursive structure of the Trace field
	Trace struct {
		OperatorType       string  `json:"OperatorType"`
		Variant            string  `json:"Variant"`
		NoOfCalls          int     `json:"NoOfCalls"`
		AvgNumberOfRows    float64 `json:"AvgNumberOfRows"`
		MedianNumberOfRows float64 `json:"MedianNumberOfRows"`
		ShardsQueried      int     `json:"ShardsQueried"`
		Inputs             []Trace `json:"Inputs,omitempty"`
	}

	QuerySummary struct {
		Q TracedQuery
		RouteCalls,
		RowsSent,
		RowsInMemory,
		ShardsQueried int
	}
)

const significantChangeThreshold = 10

func visit(trace Trace, f func(Trace)) {
	f(trace)
	for _, input := range trace.Inputs {
		visit(input, f)
	}
}

func summarizeTraces(tq []TracedQuery) map[string]QuerySummary {
	summary := make(map[string]QuerySummary)
	for _, traceElement := range tq {
		summary[traceElement.Query] = summarizeTrace(traceElement)
	}
	return summary
}

func (trace *Trace) TotalRows() int {
	return int(trace.AvgNumberOfRows * float64(trace.NoOfCalls))
}

func summarizeTrace(t TracedQuery) QuerySummary {
	summary := QuerySummary{
		Q: t,
	}

	visit(t.Trace, func(trace Trace) {
		summary.ShardsQueried += trace.ShardsQueried
		switch trace.OperatorType {
		case "Route":
			summary.RouteCalls += trace.NoOfCalls
			summary.RowsSent += trace.TotalRows()
		case "Sort":
			if trace.Variant == "Memory" {
				summary.RowsInMemory += int(trace.AvgNumberOfRows)
			}
		case "Join":
			if trace.Variant == "HashJoin" {
				// HashJoin has to keep the LHS in memory
				summary.RowsInMemory += trace.Inputs[0].TotalRows()
			}
		}
	})

	return summary
}

func compareTraces(out io.Writer, termWidth int, highLighter Highlighter, tq1, tq2 traceSummary) {
	summary1 := summarizeTraces(tq1.TracedQueries)
	summary2 := summarizeTraces(tq2.TracedQueries)

	var significantChanges, totalQueries int
	var s1RouteCalls, s1DataSent, s1MemoryRows, s1ShardsQueried int
	var s2RouteCalls, s2DataSent, s2MemoryRows, s2ShardsQueried int

	for _, q := range tq1.TracedQueries {
		s1, ok1 := summary1[q.Query]
		s2, ok2 := summary2[q.Query]
		if !ok1 || !ok2 {
			continue
		}
		totalQueries++

		table := tablewriter.NewTable(
			out,
			tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
				Symbols: tw.NewSymbols(tw.StyleASCII),
			})),
		)
		table.Configure(func(cfg *tablewriter.Config) {
			cfg.Header.Formatting.AutoFormat = tw.Off
			cfg.Row.Alignment.PerColumn = []tw.Align{
				tw.AlignLeft,
				tw.AlignRight,
				tw.AlignRight,
				tw.AlignRight,
				tw.AlignLeft,
			}
		})
		table.Header("Metric", tq1.Name, tq2.Name, "Diff", "% Change")

		m1 := compareMetric(table, "Route Calls", s1.RouteCalls, s2.RouteCalls)
		m2 := compareMetric(table, "Rows Sent", s1.RowsSent, s2.RowsSent)
		m3 := compareMetric(table, "Rows In Memory", s1.RowsInMemory, s2.RowsInMemory)
		m4 := compareMetric(table, "Shards Queried", s1.ShardsQueried, s2.ShardsQueried)

		// we introduce variables to make sure we don't shortcut the evaluation
		significant := m1 || m2 || m3 || m4
		if significant {
			significantChanges++
		}

		s1RouteCalls += s1.RouteCalls
		s1DataSent += s1.RowsSent
		s1MemoryRows += s1.RowsInMemory
		s1ShardsQueried += s1.ShardsQueried
		s2RouteCalls += s2.RouteCalls
		s2DataSent += s2.RowsSent
		s2MemoryRows += s2.RowsInMemory
		s2ShardsQueried += s2.ShardsQueried

		printQuery(out, termWidth, highLighter, s1.Q, significant)
		table.Render()
		fmt.Fprintln(out)
	}

	totalRouteCallsChange := float64(s2RouteCalls-s1RouteCalls) / float64(s1RouteCalls) * 100
	totalDataSentChange := float64(s2DataSent-s1DataSent) / float64(s1DataSent) * 100
	totalMemoryRowsChange := float64(s2MemoryRows-s1MemoryRows) / float64(s1MemoryRows) * 100
	totalShardsQueriedChange := float64(s2ShardsQueried-s1ShardsQueried) / float64(s1ShardsQueried) * 100

	// Print summary
	fmt.Fprintln(out, "Summary:")
	fmt.Fprintf(out, "- %d out of %d queries showed significant change\n", significantChanges, totalQueries)
	fmt.Fprintf(out, "- Average change in Route Calls: %.2f%%\n", totalRouteCallsChange)
	fmt.Fprintf(out, "- Average change in Data Sent: %.2f%%\n", totalDataSentChange)
	fmt.Fprintf(out, "- Average change in Rows In Memory: %.2f%%\n", totalMemoryRowsChange)
	fmt.Fprintf(out, "- Average change in Shards Queried: %.2f%%\n", totalShardsQueriedChange)
}

// compareMetric compares two metrics and appends the result to the table, returning true if the change is significant
func compareMetric(table *tablewriter.Table, metricName string, val1, val2 int) bool {
	diff := val2 - val1
	percentChange := float64(diff) / float64(val1) * 100
	percentChangeStr := fmt.Sprintf("%.2f%%", percentChange)
	if math.IsInf(percentChange, 0) {
		percentChangeStr = "∞%"
		percentChange = 0 // To not skew the average calculation
	}

	table.Append([]string{
		metricName,
		strconv.Itoa(val1),
		strconv.Itoa(val2),
		strconv.Itoa(diff),
		percentChangeStr,
	})

	return percentChange < -significantChangeThreshold
}

func printTraceSummary(out io.Writer, termWidth int, highLighter Highlighter, tq traceSummary) {
	summary := summarizeTraces(tq.TracedQueries)
	for i, query := range tq.TracedQueries {
		if i > 0 {
			fmt.Fprintln(out)
		}
		querySummary := summary[query.Query]
		printQuery(out, termWidth, highLighter, query, false)
		table := tablewriter.NewTable(
			out,
			tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
				Symbols: tw.NewSymbols(tw.StyleASCII),
			})),
		)
		table.Configure(func(cfg *tablewriter.Config) {
			cfg.Header.Formatting.AutoFormat = tw.Off
			cfg.Row.Alignment.Global = tw.AlignRight
		})
		table.Header(
			"Route Calls",
			"Rows Sent",
			"Rows In Memory",
			"Shards Queried",
		)
		table.Append([]string{
			strconv.Itoa(querySummary.RouteCalls),
			strconv.Itoa(querySummary.RowsSent),
			strconv.Itoa(querySummary.RowsInMemory),
			strconv.Itoa(querySummary.ShardsQueried),
		})
		table.Render()
	}
}
