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
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"vitess.io/vitess/go/test/endtoend/utils"
)

type Suite interface {
	NewReporterForFile(name string) Reporter
	CloseReportForFile()
	Close() string // returns the path to the file or directory with files
}

type Reporter interface {
	utils.TestingT
	AddTestCase(query string, lineNo int)
	EndTestCase()
	AddFailure(err error)
	AddInfo(info string)
	Report() string
	Failed() bool
}

type FileReporterSuite struct {
	getVschema func() []byte
}

func (frs *FileReporterSuite) NewReporterForFile(name string) Reporter {
	return newFileReporter(name, frs.getVschema)
}

func (frs *FileReporterSuite) CloseReportForFile() {}

func (frs *FileReporterSuite) Close() string {
	return "errors"
}

func NewFileReporterSuite(getVschema func() []byte) *FileReporterSuite {
	return &FileReporterSuite{
		getVschema: getVschema,
	}
}

type FileReporter struct {
	name      string
	errorFile *os.File
	startTime time.Time

	currentQuery        string
	currentQueryLineNum int
	currentStartTime    time.Time
	currentQueryFailed  bool

	failureCount int
	queryCount   int
	successCount int

	getVschema func() []byte
}

func newFileReporter(name string, getVschema func() []byte) *FileReporter {
	return &FileReporter{
		name:       name,
		startTime:  time.Now(),
		getVschema: getVschema,
	}
}

func (e *FileReporter) Failed() bool {
	return e.failureCount > 0
}

func (e *FileReporter) Report() string {
	return fmt.Sprintf(
		"%s: ok! Ran %d queries, %d successfully and %d failures take time %v s\n",
		e.name,
		e.queryCount,
		e.successCount,
		e.queryCount-e.successCount,
		time.Since(e.startTime).Seconds(),
	)
}

func (e *FileReporter) AddTestCase(query string, lineNum int) {
	e.currentQuery = query
	e.currentQueryLineNum = lineNum
	e.currentStartTime = time.Now()
	e.queryCount++
	e.currentQueryFailed = false
}

func (e *FileReporter) EndTestCase() {
	if !e.currentQueryFailed {
		e.successCount++
	}
	if e.errorFile != nil {
		err := e.errorFile.Close()
		exitIf(err, "closing error file")
		e.errorFile = nil
	}
}

func (e *FileReporter) AddFailure(err error) {
	e.failureCount++
	e.currentQueryFailed = true
	if e.currentQuery == "" {
		e.currentQuery = "GENERAL"
	}
	if e.errorFile == nil {
		e.errorFile = e.createErrorFileFor()
	}

	_, err = e.errorFile.WriteString(err.Error())
	exitIf(err, "writing to error file")

	e.createVSchemaDump()
}

func (e *FileReporter) AddInfo(info string) {
	if e.errorFile == nil {
		e.errorFile = e.createErrorFileFor()
	}
	_, err := e.errorFile.WriteString(info + "\n")
	exitIf(err, "failed to write info to error file")
}

func (e *FileReporter) createErrorFileFor() *os.File {
	qc := strconv.Itoa(e.currentQueryLineNum)
	err := os.MkdirAll(e.errorDir(), PERM)
	exitIf(err, "creating error directory")
	errorPath := path.Join(e.errorDir(), qc)
	file, err := os.Create(errorPath)
	exitIf(err, "creating error file")
	_, err = file.WriteString(fmt.Sprintf("Error log for query on line %d:\n%s\n\n", e.currentQueryLineNum, e.currentQuery))
	exitIf(err, "writing to error file")

	return file
}

func (e *FileReporter) createVSchemaDump() {
	errorDir := e.errorDir()
	err := os.MkdirAll(errorDir, PERM)
	exitIf(err, "creating error directory")

	err = os.WriteFile(path.Join(errorDir, "vschema.json"), e.getVschema(), PERM)
	exitIf(err, "writing vschema")
}

func (e *FileReporter) errorDir() string {
	errFileName := e.name
	if strings.HasPrefix(e.name, "http") {
		u, err := url.Parse(e.name)
		if err == nil {
			errFileName = path.Base(u.Path)
			if errFileName == "" || errFileName == "/" {
				errFileName = url.QueryEscape(e.name)
			}
		}
	}
	return path.Join("errors", errFileName)
}

func (e *FileReporter) Errorf(format string, args ...interface{}) {
	e.AddFailure(fmt.Errorf(format, args...))
}

func (e *FileReporter) FailNow() {
	// we don't need to do anything here
}

func (e *FileReporter) Helper() {}

var _ Reporter = (*FileReporter)(nil)
