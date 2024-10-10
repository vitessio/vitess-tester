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

package state

import (
	"fmt"
)

const (
	None     theState = 0
	SkipNext theState = 1 << iota
	SkipBelowVersion
	Reference
	VitessOnly
	MySQLOnly
	ErrorExpected
)

type theState uint8

func NewState(checkBinaryIsAtLeast func(majorVersion int, binary string) bool) *State {
	return &State{
		checkBinaryIsAtLeast: checkBinaryIsAtLeast,
		state:                None,
	}
}

type State struct {
	state                theState
	checkBinaryIsAtLeast func(majorVersion int, binary string) bool

	skipBinary  string
	skipVersion int
}

func (s theState) getStateName() string {
	switch s {
	case None:
		return "None"
	case SkipNext:
		return "SkipNext"
	case Reference:
		return "Reference"
	case VitessOnly:
		return "VitessOnly"
	case MySQLOnly:
		return "MySQLOnly"
	case SkipBelowVersion:
		return "SkipBelowVersion"
	case ErrorExpected:
		return "ErrorExpected"
	default:
		return "Unknown"
	}
}

func (s *State) changeStateTo(newState theState) error {
	if s.state != None {
		return fmt.Errorf("cannot set skip next: %s state is already active", s.state.getStateName())
	}
	s.state = newState
	return nil
}

func (s *State) checkAndClear(state theState) bool {
	isSet := s.state == state
	if isSet {
		s.state = None
	}
	return isSet
}

func (s *State) endState(oldState theState) error {
	if s.state != oldState {
		return fmt.Errorf("cannot end %s: current state is %s", oldState.getStateName(), s.state.getStateName())
	}
	s.state = None
	return nil
}

func (s *State) isSet(state theState) bool {
	return s.state == state
}

func (s *State) SetSkipNext() error {
	return s.changeStateTo(SkipNext)
}

func (s *State) SetErrorExpected() error {
	return s.changeStateTo(ErrorExpected)
}

func (s *State) IsErrorExpectedSet() bool {
	return s.state == ErrorExpected
}

func (s *State) CheckAndClearErrorExpected() bool {
	return s.checkAndClear(ErrorExpected)
}

func (s *State) SetSkipBelowVersion(binary string, version int) error {
	err := s.changeStateTo(SkipBelowVersion)
	if err != nil {
		return err
	}

	s.skipBinary = binary
	s.skipVersion = version
	return nil
}

func (s *State) SetReference() error {
	return s.changeStateTo(Reference)
}

func (s *State) IsReferenceSet() bool {
	return s.state == Reference
}

func (s *State) CheckAndClearReference() bool {
	return s.checkAndClear(Reference)
}

func (s *State) BeginVitessOnly() error {
	return s.changeStateTo(VitessOnly)
}

func (s *State) EndVitessOnly() error {
	return s.endState(VitessOnly)
}

func (s *State) IsVitessOnlySet() bool {
	return s.state == VitessOnly
}

func (s *State) BeginMySQLOnly() error {
	return s.changeStateTo(MySQLOnly)
}

func (s *State) EndMySQLOnly() error {
	return s.endState(MySQLOnly)
}

func (s *State) IsMySQLOnlySet() bool {
	return s.state == MySQLOnly
}

func (s *State) NormalExecution() bool {
	return s.state == None
}

func (s *State) ShouldSkip() bool {
	if s.checkAndClear(SkipNext) {
		return true
	}

	if s.state != SkipBelowVersion {
		return false
	}

	if s.skipBinary == "" {
		panic("skip below version state is active but skip binary is empty")
	}

	okayToRun := s.checkBinaryIsAtLeast(s.skipVersion, s.skipBinary)
	s.skipBinary = ""
	s.state = None
	return !okayToRun
}

func (s *State) RunOnVitess() bool {
	return s.state == None || s.state == VitessOnly
}

func (s *State) RunOnMySQL() bool {
	return s.state == None || s.state == MySQLOnly
}
