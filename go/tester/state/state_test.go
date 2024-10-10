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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_SetAndCheckStates(t *testing.T) {
	s := NewState(func(int, string) bool { return false })

	// Test setReferenceNext
	require.NoError(t, s.SetReference())
	assert.True(t, s.IsReferenceSet())
	assert.True(t, s.CheckAndClearReference())
	assert.False(t, s.IsReferenceSet())

	// Test setSkipNext
	require.NoError(t, s.SetSkipNext(), "setSkipNext should not fail")
	assert.True(t, s.isSet(SkipNext), "isSkipNext should return true after setSkipNext")
	assert.True(t, s.checkAndClear(SkipNext), "isSkipNext should return true after setSkipNext")
	assert.False(t, s.isSet(SkipNext), "isSkipNext should return false on second call")

	// Test beginVitessOnly and endVitessOnly
	require.NoError(t, s.BeginVitessOnly(), "beginVitessOnly should not fail")
	assert.True(t, s.IsVitessOnlySet(), "isVitessOnly should return true after beginVitessOnly")
	require.NoError(t, s.EndVitessOnly(), "endVitessOnly should not fail")
	assert.False(t, s.IsVitessOnlySet(), "isVitessOnly should return false after endVitessOnly")

	// Test beginMySQLOnly and endMySQLOnly
	require.NoError(t, s.BeginMySQLOnly(), "beginMySQLOnly should not fail")
	assert.True(t, s.IsMySQLOnlySet(), "isMySQLOnly should return true after beginMySQLOnly")
	require.NoError(t, s.EndMySQLOnly(), "endMySQLOnly should not fail")
	assert.False(t, s.IsMySQLOnlySet(), "isMySQLOnly should return false after endMySQLOnly")
}

func TestState_StateMutualExclusion(t *testing.T) {
	s := NewState(func(int, string) bool { return false })

	// Set initial state
	require.NoError(t, s.SetReference(), "setReferenceNext should not fail")

	// Try to set other states
	require.Error(t, s.SetSkipNext(), "setSkipNext should fail when Reference state is active")
	require.Error(t, s.BeginVitessOnly(), "beginVitessOnly should fail when Reference state is active")
	require.Error(t, s.BeginMySQLOnly(), "beginMySQLOnly should fail when Reference state is active")

	// Clear state and try again
	s.CheckAndClearReference()

	assert.NoError(t, s.SetSkipNext(), "setSkipNext should not fail after clearing state")
}

func TestState_NormalExecution(t *testing.T) {
	s := NewState(func(int, string) bool { return false })
	assert.True(t, s.NormalExecution(), "normalExecution should return true when state is None")

	_ = s.SetReference()
	assert.False(t, s.NormalExecution(), "normalExecution should return false when state is not None")

	s.CheckAndClearReference() // Clear the state
	assert.True(t, s.NormalExecution(), "normalExecution should return true after clearing state")
}

func TestState_RunOnVitessAndMySQL(t *testing.T) {
	s := NewState(func(int, string) bool { return false })

	assert.True(t, s.RunOnVitess(), "runOnVitess should return true when state is None")
	assert.True(t, s.RunOnMySQL(), "runOnMySQL should return true when state is None")

	_ = s.BeginVitessOnly()
	assert.True(t, s.RunOnVitess(), "runOnVitess should return true when state is VitessOnly")
	assert.False(t, s.RunOnMySQL(), "runOnMySQL should return false when state is VitessOnly")

	_ = s.EndVitessOnly()
	_ = s.BeginMySQLOnly()
	assert.False(t, s.RunOnVitess(), "runOnVitess should return false when state is MySQLOnly")
	assert.True(t, s.RunOnMySQL(), "runOnMySQL should return true when state is MySQLOnly")
}

func TestState_ShouldSkip(t *testing.T) {
	var isBinaryAtLeast bool
	s := NewState(func(int, string) bool { return isBinaryAtLeast })

	assert.False(t, s.ShouldSkip(), "shouldSkip should return false when state is None")

	_ = s.SetSkipNext()
	assert.True(t, s.ShouldSkip(), "shouldSkip should return true after setSkipNext")
	assert.False(t, s.ShouldSkip(), "shouldSkip should return false on second call after setSkipNext")

	isBinaryAtLeast = true
	_ = s.SetSkipBelowVersion("testBinary", 1000)
	assert.False(t, s.ShouldSkip(), "shouldSkip should return false if binary is at least the required version")
	assert.False(t, s.ShouldSkip(), "shouldSkip should always return false here")

	isBinaryAtLeast = false
	_ = s.SetSkipBelowVersion("testBinary", 1000)
	assert.True(t, s.ShouldSkip(), "shouldSkip should return true after setSkipBelowVersion")
	assert.False(t, s.ShouldSkip(), "shouldSkip should always return false here")
}
