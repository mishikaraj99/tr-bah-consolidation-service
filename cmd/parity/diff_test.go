package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decode(t *testing.T, s string) any {
	var v any
	require.NoError(t, json.Unmarshal([]byte(s), &v))
	return v
}

func TestCompareJSON(t *testing.T) {
	a := decode(t, `{"x":1,"y":"s","nested":{"k":true},"list":[1,2,3]}`)
	assert.Empty(t, CompareJSON(a, decode(t, `{"x":1,"y":"s","nested":{"k":true},"list":[1,2,3]}`), nil))

	// null and a missing key are different
	diffs := CompareJSON(decode(t, `{"a":null}`), decode(t, `{}`), nil)
	require.Len(t, diffs, 1)
	assert.Equal(t, "$.a", diffs[0].Path)
	assert.Equal(t, "missing in b", diffs[0].Note)

	// array order matters
	diffs = CompareJSON(decode(t, `{"l":[1,2]}`), decode(t, `{"l":[2,1]}`), nil)
	assert.Len(t, diffs, 2)

	// length mismatch is reported once plus the compared prefix
	diffs = CompareJSON(decode(t, `{"l":[1,2,3]}`), decode(t, `{"l":[1,2]}`), nil)
	require.NotEmpty(t, diffs)
	assert.Equal(t, "length mismatch", diffs[0].Note)

	// scalar mismatch
	diffs = CompareJSON(decode(t, `{"x":1}`), decode(t, `{"x":2}`), nil)
	require.Len(t, diffs, 1)
	assert.Equal(t, "$.x", diffs[0].Path)

	// ignored paths are skipped, by full path or by leaf name
	assert.Empty(t, CompareJSON(decode(t, `{"txnDate":"a"}`), decode(t, `{"txnDate":"b"}`), map[string]bool{"txnDate": true}))
	assert.Empty(t, CompareJSON(decode(t, `{"rows":[{"txnDate":"a"}]}`), decode(t, `{"rows":[{"txnDate":"b"}]}`), map[string]bool{"txnDate": true}))
	assert.Empty(t, CompareJSON(decode(t, `{"a":{"b":1}}`), decode(t, `{"a":{"b":2}}`), map[string]bool{"$.a.b": true}))

	// type mismatch
	diffs = CompareJSON(decode(t, `{"x":[1]}`), decode(t, `{"x":{"k":1}}`), nil)
	require.Len(t, diffs, 1)
	assert.Equal(t, "type mismatch", diffs[0].Note)
}
