/*
MIT License

Copyright (c) 2025 Ashesh Goplani

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.

Copyright 2009 The Go Authors.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are
met:

   * Redistributions of source code must retain the above copyright
notice, this list of conditions and the following disclaimer.
   * Redistributions in binary form must reproduce the above
copyright notice, this list of conditions and the following disclaimer
in the documentation and/or other materials provided with the
distribution.
   * Neither the name of Google LLC nor the names of its
contributors may be used to endorse or promote products derived from
this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
OWNER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cli

import (
	"encoding/json"
	"testing"
)

// Lifted from golang/go src/encoding/json/encode_test.go L966-L969 (BSD-3-Clause) @3901409b5d0fb7c85a3e6730a59943cc93b2835c.
// rawclaw: fixture values/call direction adapted only as marked per row.
var rawStringTests = []struct {
	in  string
	out string
}{
	// golang/go decode_test.go L435; value fixture originally Decoder.Decode, adapted helper call.
	{`"a\u1234"`, "a\u1234"},
	// golang/go decode_test.go L436; value fixture originally Decoder.Decode, adapted helper call.
	{`"http:\/\/"`, "http://"},
	// golang/go decode_test.go L437; value fixture originally Decoder.Decode, adapted helper call.
	{`"g-clef: \uD834\uDD1E"`, "g-clef: \U0001D11E"},
	// golang/go decode_test.go L2229,L2238-L2241 direct Unmarshal wrongtype; empty expectation agent-deck hook_handler.go L102-L103.
	{`{"user": "name"}`, ""},
	// golang/go decode_test.go L2030-L2037,L2103-L2111 null; zero-initialized empty result agent-deck L101-L105, not preinitialized14.
	{"null", ""},
	// golang/go decode_test.go L720-L722 invalid UTF8 fixture; original Decoder.Decode, adapted helper call.
	{"\"hello\xffworld\"", "hello\ufffdworld"},
	// golang/go encode_test.go L980 newline; encoding input/output swapped for decoding.
	{`"\n"`, "\x0a"},
	// golang/go decode_test.go L484 string plus invalid suffix; scanner-only fixture, public Unmarshal validation decode.go L97-L108; error collapse agent-deck L102-L103.
	{" \"string\" \x01", ""},
}

// Lifted from golang/go src/encoding/json/encode_test.go L1004-L1016 (BSD-3-Clause) @3901409b5d0fb7c85a3e6730a59943cc93b2835c.
// rawclaw: names; RawMessage conversion; one string result; delete Marshal error branch and byte conversion; diagnostic name.
func TestRawString(t *testing.T) {
	for _, tt := range rawStringTests {
		out := rawString(json.RawMessage(tt.in))
		if out != tt.out {
			t.Errorf("rawString(%q) = %#q, want %#q", tt.in, out, tt.out)
		}
	}
}
