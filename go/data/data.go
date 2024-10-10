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

package data

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/vitessio/vt/go/typ"
)

type (
	Query struct {
		FirstWord string
		Query     string
		Line      int
		Type      typ.CmdType
	}
)

func readData(url string) ([]byte, error) {
	if strings.HasPrefix(url, "http") {
		client := http.Client{}
		res, err := client.Get(url)
		if err != nil {
			return nil, err
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("failed to get data from %s, status code %d", url, res.StatusCode)
		}
		defer res.Body.Close()
		return io.ReadAll(res.Body)
	}
	return os.ReadFile(url)
}

func LoadQueries(url string) ([]Query, error) {
	data, err := readData(url)
	if err != nil {
		return nil, err
	}
	seps := bytes.Split(data, []byte("\n"))
	queries := make([]Query, 0, len(seps))
	newStmt := true
	for i, v := range seps {
		v := bytes.TrimSpace(v)
		s := string(v)
		// we will skip # comment here
		if strings.HasPrefix(s, "#") {
			newStmt = true
			continue
		} else if strings.HasPrefix(s, "--") {
			queries = append(queries, Query{Query: s, Line: i + 1})
			newStmt = true
			continue
		} else if len(s) == 0 {
			continue
		}

		if newStmt {
			queries = append(queries, Query{Query: s, Line: i + 1})
		} else {
			lastQuery := queries[len(queries)-1]
			lastQuery = Query{Query: fmt.Sprintf("%s\n%s", lastQuery.Query, s), Line: lastQuery.Line}
			queries[len(queries)-1] = lastQuery
		}

		// if the line has a ; in the end, we will treat new line as the new statement.
		newStmt = strings.HasSuffix(s, ";")
	}

	return ParseQueries(queries...)
}

// ParseQueries parses an array of string into an array of Query object.
// Note: a Query statement may reside in several lines.
func ParseQueries(qs ...Query) ([]Query, error) {
	queries := make([]Query, 0, len(qs))
	for _, rs := range qs {
		realS := rs.Query
		s := rs.Query
		q := Query{}
		q.Type = typ.Unknown
		q.Line = rs.Line
		// a valid Query's length should be at least 3.
		if len(s) < 3 {
			continue
		}
		// we will skip #comment and line with zero characters here
		switch {
		case s[0] == '#':
			q.Type = typ.Comment
		case s[0:2] == "--":
			q.Type = typ.CommentWithCommand
			if s[2] == ' ' {
				s = s[3:]
			} else {
				s = s[2:]
			}
		case s[0] == '\n':
			q.Type = typ.EmptyLine
		}

		if q.Type != typ.Comment {
			// Calculate first word length(the command), terminated
			// by 'space' , '(' or 'delimiter'
			var i int
			for i = 0; i < len(s) && s[i] != '(' && s[i] != ' ' && s[i] != ';' && s[i] != '\n'; i++ {
				// empty by design
			}
			if i > 0 {
				q.FirstWord = s[:i]
			}
			s = s[i:]

			q.Query = s
			if q.Type == typ.Unknown || q.Type == typ.CommentWithCommand {
				if err := q.getQueryType(realS); err != nil {
					return nil, err
				}
			}
		}

		queries = append(queries, q)
	}
	return queries, nil
}

// for a single query, it has some prefix. Prefix mapps to a query type.
// e.g query_vertical maps to typ.Q_QUERY_VERTICAL
func (q *Query) getQueryType(qu string) error {
	tp := typ.FindType(q.FirstWord)
	if tp > 0 {
		q.Type = tp
	} else {
		// No mysqltest command matched
		if q.Type != typ.CommentWithCommand {
			// A query that will sent to vitess
			q.Query = qu
			q.Type = typ.Query
		} else {
			log.WithFields(log.Fields{"line": q.Line, "command": q.FirstWord, "arguments": q.Query}).Error("invalid command")
			return fmt.Errorf("invalid command %s", q.FirstWord)
		}
	}
	return nil
}
