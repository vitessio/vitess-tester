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

package benchstat

import (
	"encoding/json"
	"io"
	"os"
	"sort"
	"strconv"

	"github.com/vitessio/vt/go/keys"
)

func readTraceFile(fileName string) TraceFile {
	// Open the JSON file
	file, err := os.Open(fileName)
	if err != nil {
		exit("Error opening file: " + err.Error())
	}
	defer file.Close()

	// Create a decoder
	decoder := json.NewDecoder(file)

	// Read the opening bracket
	_, err = decoder.Token()
	if err != nil {
		exit("Error reading json: " + err.Error())
	}

	// Peek at the first object to determine the file type
	var peekObj json.RawMessage
	if err := decoder.Decode(&peekObj); err != nil {
		exit("Error peeking json object: " + err.Error())
	}

	// Reset the file pointer to the beginning
	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		exit("Error rewinding file: " + err.Error())
	}
	decoder = json.NewDecoder(file)

	// Skip the opening bracket again
	_, err = decoder.Token()
	if err != nil {
		exit("Error reading json: " + err.Error())
	}

	// Determine the type based on the structure of the first object
	var tracedQuery TracedQuery
	var analysedQuery keys.QueryAnalysisResult
	if err := json.Unmarshal(peekObj, &tracedQuery); err == nil && tracedQuery.Query != "" {
		return readTracedQueryFile(decoder, fileName)
	} else if err := json.Unmarshal(peekObj, &analysedQuery); err == nil && analysedQuery.QueryStructure != "" {
		return readAnalysedQueryFile(decoder, fileName)
	}

	exit("Unknown file format")
	panic("unreachable")
}

func readTracedQueryFile(decoder *json.Decoder, fileName string) TraceFile {
	var tracedQueries []TracedQuery
	for decoder.More() {
		var element TracedQuery
		err := decoder.Decode(&element)
		if err != nil {
			exit("Error reading json: " + err.Error())
		}
		tracedQueries = append(tracedQueries, element)
	}

	// Read the closing bracket
	_, err := decoder.Token()
	if err != nil {
		exit("Error reading json: " + err.Error())
	}

	sort.Slice(tracedQueries, func(i, j int) bool {
		a, err := strconv.Atoi(tracedQueries[i].LineNumber)
		if err != nil {
			return false
		}
		b, err := strconv.Atoi(tracedQueries[j].LineNumber)
		if err != nil {
			return false
		}
		return a < b
	})

	return TraceFile{
		Name:          fileName,
		TracedQueries: tracedQueries,
	}
}

func readAnalysedQueryFile(decoder *json.Decoder, fileName string) TraceFile {
	var analysedQueries []keys.QueryAnalysisResult
	for decoder.More() {
		var element keys.QueryAnalysisResult
		err := decoder.Decode(&element)
		if err != nil {
			exit("Error reading json: " + err.Error())
		}
		analysedQueries = append(analysedQueries, element)
	}

	// Read the closing bracket
	_, err := decoder.Token()
	if err != nil {
		exit("Error reading json: " + err.Error())
	}

	return TraceFile{
		Name:            fileName,
		AnalysedQueries: analysedQueries,
	}
}
