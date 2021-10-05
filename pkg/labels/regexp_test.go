// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package labels

import (
	"regexp/syntax"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewFastRegexMatcher(t *testing.T) {
	cases := []struct {
		regex    string
		value    string
		expected bool
	}{
		{regex: "(foo|bar)", value: "foo", expected: true},
		{regex: "(foo|bar)", value: "foo bar", expected: false},
		{regex: "(foo|bar)", value: "bar", expected: true},
		{regex: "foo.*", value: "foo bar", expected: true},
		{regex: "foo.*", value: "bar foo", expected: false},
		{regex: ".*foo", value: "foo bar", expected: false},
		{regex: ".*foo", value: "bar foo", expected: true},
		{regex: ".*foo", value: "foo", expected: true},
		{regex: "^.*foo$", value: "foo", expected: true},
		{regex: "^.+foo$", value: "foo", expected: false},
		{regex: "^.+foo$", value: "bfoo", expected: true},
		{regex: ".*", value: "\n", expected: false},
		{regex: ".*", value: "\nfoo", expected: false},
		{regex: ".*foo", value: "\nfoo", expected: false},
		{regex: "foo.*", value: "foo\n", expected: false},
		{regex: "foo\n.*", value: "foo\n", expected: true},
		{regex: ".*foo.*", value: "foo", expected: true},
		{regex: ".*foo.*", value: "foo bar", expected: true},
		{regex: ".*foo.*", value: "hello foo world", expected: true},
		{regex: ".*foo.*", value: "hello foo\n world", expected: false},
		{regex: ".*foo\n.*", value: "hello foo\n world", expected: true},
		{regex: ".*", value: "foo", expected: true},
		{regex: "", value: "foo", expected: false},
		{regex: "", value: "", expected: true},
	}

	for _, c := range cases {
		m, err := NewFastRegexMatcher(c.regex)
		require.NoError(t, err)
		require.Equal(t, c.expected, m.MatchString(c.value))
	}
}

func TestOptimizeConcatRegex(t *testing.T) {
	cases := []struct {
		regex    string
		prefix   string
		suffix   string
		contains string
	}{
		{regex: "foo(hello|bar)", prefix: "foo", suffix: "", contains: ""},
		{regex: "foo(hello|bar)world", prefix: "foo", suffix: "world", contains: ""},
		{regex: "foo.*", prefix: "foo", suffix: "", contains: ""},
		{regex: "foo.*hello.*bar", prefix: "foo", suffix: "bar", contains: "hello"},
		{regex: ".*foo", prefix: "", suffix: "foo", contains: ""},
		{regex: "^.*foo$", prefix: "", suffix: "foo", contains: ""},
		{regex: ".*foo.*", prefix: "", suffix: "", contains: "foo"},
		{regex: ".*foo.*bar.*", prefix: "", suffix: "", contains: "foo"},
		{regex: ".*(foo|bar).*", prefix: "", suffix: "", contains: ""},
		{regex: ".*[abc].*", prefix: "", suffix: "", contains: ""},
		{regex: ".*((?i)abc).*", prefix: "", suffix: "", contains: ""},
		{regex: ".*(?i:abc).*", prefix: "", suffix: "", contains: ""},
		{regex: "(?i:abc).*", prefix: "", suffix: "", contains: ""},
		{regex: ".*(?i:abc)", prefix: "", suffix: "", contains: ""},
		{regex: ".*(?i:abc)def.*", prefix: "", suffix: "", contains: "def"},
		{regex: "(?i).*(?-i:abc)def", prefix: "", suffix: "", contains: "abc"},
		{regex: ".*(?msU:abc).*", prefix: "", suffix: "", contains: "abc"},
		{regex: "[aA]bc.*", prefix: "", suffix: "", contains: "bc"},
	}

	for _, c := range cases {
		parsed, err := syntax.Parse(c.regex, syntax.Perl)
		require.NoError(t, err)

		prefix, suffix, contains := optimizeConcatRegex(parsed)
		require.Equal(t, c.prefix, prefix)
		require.Equal(t, c.suffix, suffix)
		require.Equal(t, c.contains, contains)
	}
}

// Refer to https://github.com/prometheus/prometheus/issues/2651.
func TestFindSetMatches(t *testing.T) {
	for _, c := range []struct {
		pattern string
		exp     []string
	}{
		// Single value, coming from a `bar=~"foo"` selector.
		{"foo", []string{"foo"}},
		// Simple sets alternates.
		{"foo|bar|zz", []string{"foo", "bar", "zz"}},
		// Simple sets alternate and concat (bar|baz is parsed as ba(r|z)).
		{"foo|bar|baz", []string{"foo", "bar", "baz"}},
		// Simple sets alternate and concat and capture
		{"foo|bar|baz|(zz)", []string{"foo", "bar", "baz", "zz"}},
		// Simple sets alternate and concat and alternates with empty matches
		// parsed as  b(ar|(?:)|uzz) where b(?:) means literal b.
		{"bar|b|buzz", []string{"bar", "b", "buzz"}},
		// Simple sets containing escaped characters.
		{"fo\\.o|bar\\?|\\^baz", []string{"fo.o", "bar?", "^baz"}},

		// high low charset different => A(B[CD]|EF)|BC[XY]
		{"ABC|ABD|AEF|BCX|BCY", []string{"ABC", "ABD", "AEF", "BCX", "BCY"}},
		// triple concat
		{"api_(v1|prom)_push", []string{"api_v1_push", "api_prom_push"}},
		// triple concat with multiple alternates
		{"(api|rpc)_(v1|prom)_push", []string{"api_v1_push", "api_prom_push", "rpc_v1_push", "rpc_prom_push"}},
		{"(api|rpc)_(v1|prom)_(push|query)", []string{"api_v1_push", "api_v1_query", "api_prom_push", "api_prom_query", "rpc_v1_push", "rpc_v1_query", "rpc_prom_push", "rpc_prom_query"}},
		// OpPlus with concat
		{"(.+)/(foo|bar)", nil},
		// Simple sets containing special characters without escaping.
		{"fo.o|bar?|^baz", nil},
		// case sensitive wrapper.
		{"(?i)foo", nil},
		// case sensitive wrapper on alternate.
		{"(?i)foo|bar|baz", nil},
		// case sensitive wrapper on concat.
		{"(api|rpc)_(v1|prom)_((?i)push|query)", nil},
		// too high charset combination
		{"(api|rpc)_[^0-9]", nil},
	} {
		c := c
		t.Run(c.pattern, func(t *testing.T) {
			t.Parallel()
			parsed, err := syntax.Parse(c.pattern, syntax.Perl)
			require.NoError(t, err)
			matches := findSetMatches(parsed, "")
			require.Equal(t, c.exp, matches)
		})

	}
}
