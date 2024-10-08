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
	"fmt"
	"os"
	"time"

	"github.com/jstemmer/go-junit-report/v2/junit"
)

type XMLTestSuite struct {
	ts            junit.Testsuites
	startTime     time.Time
	currTestSuite junit.Testsuite
	currTestCase  *junit.Testcase
}

var _ Suite = (*XMLTestSuite)(nil)

func NewXMLTestSuite() *XMLTestSuite {
	return &XMLTestSuite{}
}

func (xml *XMLTestSuite) NewReporterForFile(name string) Reporter {
	xml.startTime = time.Now()
	xml.currTestSuite = junit.Testsuite{
		Name:      name,
		Timestamp: xml.startTime.String(),
	}
	return xml
}

func (xml *XMLTestSuite) CloseReportForFile() {
	xml.currTestSuite.Time = fmt.Sprintf("%v", time.Since(xml.startTime).Seconds())
	xml.ts.AddSuite(xml.currTestSuite)
}

func (xml *XMLTestSuite) Close() string {
	fileName := "report.xml"
	file, err := os.Create(fileName)
	exitIf(err, "creating report.xml file")
	defer file.Close()
	err = xml.ts.WriteXML(file)
	exitIf(err, "writing report.xml file")
	return fileName
}

func (xml *XMLTestSuite) AddTestCase(query string, lineNo int) {
	xml.currTestCase = &junit.Testcase{
		Name:   query,
		Status: fmt.Sprintf("Line No. - %v", lineNo),
	}
}

func (xml *XMLTestSuite) EndTestCase() {
	xml.currTestSuite.AddTestcase(*xml.currTestCase)
	xml.currTestCase = nil
}

func (xml *XMLTestSuite) AddFailure(err error) {
	if xml.currTestCase == nil {
		xml.AddTestCase("SETUP", 0)
		xml.AddFailure(err)
		xml.EndTestCase()
		return
	}

	if xml.currTestCase.Failure != nil {
		xml.currTestCase.Failure.Message += "\n" + err.Error()
		return
	}
	xml.currTestCase.Failure = &junit.Result{
		Message: err.Error(),
		Type:    fmt.Sprintf("%T", err),
	}
}

func (xml *XMLTestSuite) Report() string {
	return fmt.Sprintf(
		"%s: ok! Ran %d queries, %d successfully and %d failures take time %v\n",
		xml.currTestSuite.Name,
		xml.currTestSuite.Tests,
		xml.currTestSuite.Tests-xml.currTestSuite.Failures,
		xml.currTestSuite.Failures,
		time.Since(xml.startTime),
	)
}

func (xml *XMLTestSuite) Failed() bool {
	return xml.currTestSuite.Failures != 0
}

func (xml *XMLTestSuite) AddInfo(info string) {
	if xml.currTestCase != nil {
		if xml.currTestCase.SystemOut == nil {
			xml.currTestCase.SystemOut = &junit.Output{}
		}
		xml.currTestCase.SystemOut.Data += info + "\n"
	}
}

func (xml *XMLTestSuite) Errorf(format string, args ...interface{}) {
	xml.AddFailure(fmt.Errorf(format, args...))
}

func (xml *XMLTestSuite) FailNow() {
	// we don't need to do anything here
}

func (xml *XMLTestSuite) Helper() {}
