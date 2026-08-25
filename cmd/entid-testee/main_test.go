// Copyright The EntID Authors.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The tests below check the wire protocol of spec/testee.proto, not the rules.
// Whether a value is valid is the conformance runner's question, and this
// executable exists so that it can ask it.

// requestOf builds a TesteeRequest.
type requestOf struct {
	caseID    string
	operation uint64
	input     string
	// profile is only written when presentProfile is set, so that a test can
	// tell an absent profile from one named "".
	profile        string
	presentProfile bool
	kind           string
	country        string
	payload        []byte
}

func (r requestOf) encode() []byte {
	body := appendString(nil, 1, r.caseID)
	body = appendVarint(body, 2, r.operation)
	body = appendString(body, 3, r.input)
	if r.presentProfile || r.profile != "" {
		body = appendString(body, 4, r.profile)
	}
	if r.kind != "" {
		body = appendString(body, 5, r.kind)
	}
	if r.country != "" {
		body = appendString(body, 6, r.country)
	}
	if r.payload != nil {
		body = appendBytes(body, 7, r.payload)
	}
	return body
}

// exchange runs one request through the server and returns the decoded fields
// of the response.
func exchange(t *testing.T, req requestOf) map[int][]byte {
	t.Helper()
	var in bytes.Buffer
	frame(&in, req.encode())

	var out bytes.Buffer
	if err := serve(&in, &out); err != nil {
		t.Fatalf("serve: %v", err)
	}
	body := out.Bytes()
	if len(body) < 4 {
		t.Fatal("the server wrote no framed response")
	}
	n := binary.LittleEndian.Uint32(body[:4])
	// The length covers the message alone.
	if int(n) != len(body)-4 {
		t.Fatalf("frame declares %d bytes but %d follow", n, len(body)-4)
	}
	resp := fields(t, body[4:])
	if got := string(resp[1]); got != req.caseID {
		t.Fatalf("the case id must be echoed, got %q want %q", got, req.caseID)
	}
	return resp
}

func frame(w *bytes.Buffer, body []byte) {
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(body)))
	w.Write(header[:])
	w.Write(body)
}

// fields decodes a message into its length delimited and varint fields.
func fields(t *testing.T, body []byte) map[int][]byte {
	t.Helper()
	out := map[int][]byte{}
	for i := 0; i < len(body); {
		tag, n := readVarint(body[i:])
		if n == 0 {
			t.Fatal("malformed response")
		}
		i += n
		field, wire := int(tag>>3), int(tag&7)
		switch wire {
		case 0:
			v, n := readVarint(body[i:])
			if n == 0 {
				t.Fatal("malformed response")
			}
			i += n
			out[field] = binary.AppendUvarint(nil, v)
		case 2:
			size, n := readVarint(body[i:])
			if n == 0 || int(size) > len(body)-i-n {
				t.Fatal("malformed response")
			}
			i += n
			out[field] = body[i : i+int(size)]
			i += int(size)
		default:
			t.Fatalf("unexpected wire type %d", wire)
		}
	}
	return out
}

func varintOf(t *testing.T, b []byte) uint64 {
	t.Helper()
	v, n := readVarint(b)
	if n == 0 {
		t.Fatal("malformed varint")
	}
	return v
}

// Statuses and reason codes of the shared enums, for the assertions below.
const (
	statusValid       = 1
	statusInvalid     = 2
	statusUnsupported = 3
	reasonOK          = 1
)

func TestValidateRoundTrip(t *testing.T) {
	resp := exchange(t, requestOf{
		caseID: "vat-be-valid-official-001", operation: opValidate,
		kind: "vat", input: "BE 0123.456.749", profile: "compatible",
	})

	report, ok := resp[3]
	if !ok {
		t.Fatalf("expected a validation report, got fields %v", keys(resp))
	}
	m := fields(t, report)
	if got := string(m[1]); got != "vat" {
		t.Errorf("kind %q", got)
	}
	if got := string(m[2]); got != "BE0123456749" {
		t.Errorf("canonical value %q", got)
	}
	if got := string(m[3]); got != "BE" {
		t.Errorf("country %q", got)
	}
	format := fields(t, m[4])
	checksum := fields(t, m[5])
	if varintOf(t, format[1]) != statusValid || varintOf(t, checksum[1]) != statusValid {
		t.Errorf("a published example must be valid on both steps")
	}
	if varintOf(t, format[2]) != reasonOK || varintOf(t, checksum[2]) != reasonOK {
		t.Errorf("a valid step must carry the ok reason")
	}
}

func TestValidateReportsAnInvalidChecksum(t *testing.T) {
	resp := exchange(t, requestOf{
		caseID: "c2", operation: opValidate,
		kind: "siren", input: "012345675", profile: "compatible",
	})
	m := fields(t, resp[3])
	if got := varintOf(t, fields(t, m[4])[1]); got != statusValid {
		t.Fatalf("format status %d, want valid", got)
	}
	if got := varintOf(t, fields(t, m[5])[1]); got != statusInvalid {
		t.Fatalf("checksum status %d, want invalid", got)
	}
}

func TestCanonicalizeRoundTrip(t *testing.T) {
	resp := exchange(t, requestOf{
		caseID: "c3", operation: opCanonicalize,
		kind: "siren", input: "012 345-674", profile: "compatible",
	})
	m := fields(t, resp[2])
	if got := string(m[2]); got != "012345674" {
		t.Fatalf("canonical value %q", got)
	}
	if got := varintOf(t, m[4]); got != statusValid {
		t.Fatalf("status %d, want valid", got)
	}
}

func TestAbsentCountryIsOmitted(t *testing.T) {
	resp := exchange(t, requestOf{
		caseID: "c4", operation: opValidate,
		kind: "lei", input: "00000000000000000098", profile: "compatible",
	})
	m := fields(t, resp[3])
	if _, has := m[3]; has {
		t.Fatal("a global definition with no country context must omit the country")
	}
}

func TestAbsentProfileIsAccepted(t *testing.T) {
	// A case that declares no profile leaves the field out entirely, which is
	// what lets the selected definition apply its own default.
	resp := exchange(t, requestOf{
		caseID: "c5", operation: opValidate,
		kind: "siren", input: "012345674",
	})
	m := fields(t, resp[3])
	if got := varintOf(t, fields(t, m[4])[1]); got != statusValid {
		t.Fatalf("format status %d, want valid", got)
	}
}

func TestLoadRulesetIsAnsweredByTheGenerator(t *testing.T) {
	// A truncated bundle: the generator must refuse it, and the refusal must
	// be reported as an observation rather than a failure.
	resp := exchange(t, requestOf{
		caseID: "loader-truncated-001", operation: opLoadRuleset,
		payload: []byte{0x08, 0x01, 0x12, 0xff},
	})
	load, ok := resp[4]
	if !ok {
		t.Fatalf("expected a load observation, got fields %v", keys(resp))
	}
	m := fields(t, load)
	if _, accepted := m[1]; accepted {
		t.Fatal("a truncated bundle must never be accepted")
	}
	if got := string(m[2]); got != "invalid_ruleset" {
		t.Fatalf("engine error %q, want invalid_ruleset", got)
	}
}

func TestLoadRulesetAcceptsTheShippedBundle(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "spec", "entid-rules.binpb"))
	if err != nil {
		t.Fatal(err)
	}
	resp := exchange(t, requestOf{caseID: "c7", operation: opLoadRuleset, payload: raw})
	m := fields(t, resp[4])
	if varintOf(t, m[1]) != 1 {
		t.Fatal("the shipped bundle must be accepted")
	}
	if _, refused := m[2]; refused {
		t.Fatal("an accepted bundle must carry no engine error")
	}
}

func TestUnknownProfileBecomesAFailure(t *testing.T) {
	resp := exchange(t, requestOf{
		caseID: "c8", operation: opValidate,
		kind: "siren", input: "012345674", profile: "lenient",
	})
	failure, ok := resp[5]
	if !ok {
		t.Fatalf("expected a failure, got fields %v", keys(resp))
	}
	m := fields(t, failure)
	if got := varintOf(t, m[1]); got != failureInternalError {
		t.Fatalf("failure kind %d, want internal error", got)
	}
	if !strings.Contains(string(m[2]), "profile") {
		t.Fatalf("the detail must name the profile, got %q", m[2])
	}
}

func TestUnsupportedOperationBecomesAFailure(t *testing.T) {
	resp := exchange(t, requestOf{
		caseID: "c9", operation: 99,
		kind: "siren", input: "012345674", profile: "compatible",
	})
	m := fields(t, resp[5])
	if got := varintOf(t, m[1]); got != failureUnsupportedOperation {
		t.Fatalf("failure kind %d, want unsupported operation", got)
	}
}

func TestSeveralRequestsInOneStream(t *testing.T) {
	ids := []string{"a", "b", "c"}
	var in bytes.Buffer
	for _, id := range ids {
		frame(&in, requestOf{
			caseID: id, operation: opValidate,
			kind: "siren", input: "012345674", profile: "compatible",
		}.encode())
	}
	var out bytes.Buffer
	if err := serve(&in, &out); err != nil {
		t.Fatal(err)
	}
	body := out.Bytes()
	for _, want := range ids {
		n := binary.LittleEndian.Uint32(body[:4])
		m := fields(t, body[4:4+n])
		if got := string(m[1]); got != want {
			t.Fatalf("response carries case id %q, want %q", got, want)
		}
		body = body[4+n:]
	}
	if len(body) != 0 {
		t.Fatalf("%d trailing bytes after three responses", len(body))
	}
}

func TestOversizedFrameIsRefused(t *testing.T) {
	var in bytes.Buffer
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], maxFrame+1)
	in.Write(header[:])
	var out bytes.Buffer
	if err := serve(&in, &out); err == nil {
		t.Fatal("a frame above the limit must be refused")
	}
}

func TestTruncatedStreamIsRefused(t *testing.T) {
	var in bytes.Buffer
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], 32)
	in.Write(header[:])
	in.Write([]byte{0x08}) // one byte where thirty two were announced
	var out bytes.Buffer
	if err := serve(&in, &out); err == nil {
		t.Fatal("a truncated frame must be refused")
	}
}

func TestEmptyStreamEndsCleanly(t *testing.T) {
	var out bytes.Buffer
	if err := serve(bytes.NewReader(nil), &out); err != nil {
		t.Fatalf("an empty stream is a clean end, got %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("an empty stream must produce no response")
	}
}

func keys(m map[int][]byte) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestProfilePresenceIsFollowed checks that an absent profile and one named ""
// are not conflated. testee.proto gives the field explicit presence precisely
// so that a definition's own default can apply, and this engine spells "no
// profile" as the empty string, so a profile literally named "" has no
// representation and is reported as a failure rather than silently answered.
func TestProfilePresenceIsFollowed(t *testing.T) {
	// Absent: the request carries no field 4 at all.
	body := appendString(nil, 1, "absent")
	body = appendVarint(body, 2, opValidate)
	body = appendString(body, 3, "012345674")
	body = appendString(body, 5, "siren")

	var in bytes.Buffer
	frame(&in, body)
	var out bytes.Buffer
	if err := serve(&in, &out); err != nil {
		t.Fatal(err)
	}
	resp := fields(t, out.Bytes()[4:])
	if _, ok := resp[3]; !ok {
		t.Fatalf("an absent profile must be answered, got fields %v", keys(resp))
	}

	// Present and empty: a profile named "".
	got := exchange(t, requestOf{
		caseID: "named-empty", operation: opValidate,
		kind: "siren", input: "012345674", profile: "", presentProfile: true,
	})
	failure, ok := got[5]
	if !ok {
		t.Fatalf(`a profile named "" must be reported as a failure, got fields %v`, keys(got))
	}
	if k := varintOf(t, fields(t, failure)[1]); k != failureInternalError {
		t.Fatalf("failure kind %d, want internal error", k)
	}
}

// TestInvalidEncodingIsObserved checks that the safety bound of section 5
// reaches the wire as an observation, not a failure.
func TestInvalidEncodingIsObserved(t *testing.T) {
	const reasonInvalidEncoding = 21
	resp := exchange(t, requestOf{
		caseID: "enc", operation: opValidate,
		kind: "siren", input: "0123\xff", profile: "compatible", presentProfile: true,
	})
	m := fields(t, resp[3])
	if got := string(m[2]); got != "0123\xff" {
		t.Fatalf("the value must be reported verbatim, got %q", got)
	}
	format := fields(t, m[4])
	if got := varintOf(t, format[1]); got != statusUnsupported {
		t.Fatalf("format status %d, want unsupported", got)
	}
	if got := varintOf(t, format[2]); got != reasonInvalidEncoding {
		t.Fatalf("format reason %d, want invalid_encoding", got)
	}
}

// TestMessageKeyCrossesTheProtocol covers field 3 of ObservedStep. Section 11.2
// of engine.md states that the common tests compare the reason code and the
// message key; until the field existed they compared only the code, and an
// engine could emit any key at all without a case noticing.
//
// The key is filled from the rule that produced the result, and stays absent
// when the result precedes every assertion.
func TestMessageKeyCrossesTheProtocol(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  requestOf
		// want is the key expected on the format step, empty meaning the
		// field must not be present at all.
		want string
	}{
		{
			// A length assertion of the SIREN rule, which names a key.
			name: "an assertion that names a key",
			req:  requestOf{caseID: "k-1", operation: opValidate, kind: "siren", input: "0123"},
			want: "fr.siren.length",
		},
		{
			// An EUID applies the national rule, so the key reported is the
			// national one rather than an EUID specific restatement.
			name: "a key that comes from a called rule",
			req:  requestOf{caseID: "k-2", operation: opValidate, kind: "euid", input: "BE0400.9123456744"},
			want: "be.enterprise_number.leading",
		},
		{
			name: "a valid format asserts nothing",
			req:  requestOf{caseID: "k-3", operation: opValidate, kind: "vat", input: "BE0123456749"},
		},
		{
			name: "an unresolved kind precedes every rule",
			req:  requestOf{caseID: "k-4", operation: opValidate, kind: "not-an-identifier", input: "X"},
		},
		{
			name: "a dispatch failure precedes every rule",
			req:  requestOf{caseID: "k-5", operation: opValidate, kind: "vat", input: "0123456749"},
		},
		{
			name: "a value above the byte limit is refused before any rule",
			req:  requestOf{caseID: "k-6", operation: opValidate, kind: "vat", input: strings.Repeat("9", 1025)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := exchange(t, tc.req)
			report := fields(t, resp[3])
			format := fields(t, report[4])
			key, present := format[3]
			if tc.want == "" {
				if present {
					t.Fatalf("the format step carries the key %q, none was expected", key)
				}
				return
			}
			if !present {
				t.Fatalf("the format step carries no message key, %q was expected", tc.want)
			}
			if string(key) != tc.want {
				t.Fatalf("message key %q, want %q", key, tc.want)
			}
		})
	}
}

// TestChecksumStepReportsItsKey covers the other step: a checksum rule may name
// a key too, and a step that never ran must not invent one.
func TestChecksumStepReportsItsKey(t *testing.T) {
	// A format that did not hold leaves the checksum not_run, which is a
	// result no rule produced.
	resp := exchange(t, requestOf{caseID: "k-7", operation: opValidate, kind: "siren", input: "0123"})
	report := fields(t, resp[3])
	checksum := fields(t, report[5])
	if key, present := checksum[3]; present {
		t.Fatalf("a checksum that never ran carries the key %q", key)
	}

	// validate_format leaves the checksum not_requested, likewise.
	resp = exchange(t, requestOf{caseID: "k-8", operation: opValidateFormat, kind: "vat", input: "BE0123456749"})
	report = fields(t, resp[3])
	checksum = fields(t, report[5])
	if key, present := checksum[3]; present {
		t.Fatalf("a checksum that was not requested carries the key %q", key)
	}
}

// The paths below are the ones a conformance run never takes: a malformed
// frame, a stream that stops mid message, a standard output that fails. The
// runner always speaks the protocol correctly, so without these the code that
// handles it speaking incorrectly never ran.

// errWriter fails after letting a fixed number of bytes through, which is how a
// closed pipe behaves to the process writing into it.
type errWriter struct {
	allow int
	err   error
}

func (w *errWriter) Write(p []byte) (int, error) {
	if w.allow <= 0 {
		return 0, w.err
	}
	if len(p) > w.allow {
		n := w.allow
		w.allow = 0
		return n, w.err
	}
	w.allow -= len(p)
	return len(p), nil
}

func TestDecodeRequestRefusesMalformedFrames(t *testing.T) {
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"a tag varint that never ends", []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{"a varint field whose value never ends", []byte{0x10, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{"a length that never ends", []byte{0x0a, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{"a length beyond the frame", []byte{0x0a, 0x40, 'a'}},
		{"a fixed width field, which no request carries", []byte{0x0d, 0x01, 0x02, 0x03, 0x04}},
		{"a group, which proto3 removed", []byte{0x0b, 0x01}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := decodeRequest(tc.body); err == nil {
				t.Fatal("a malformed request must be refused, not guessed at")
			}
		})
	}

	t.Run("a country code crosses the protocol", func(t *testing.T) {
		body := appendString(nil, 6, "BE")
		req, err := decodeRequest(body)
		if err != nil {
			t.Fatal(err)
		}
		if req.countryCode != "BE" {
			t.Fatalf("country %q", req.countryCode)
		}
	})
}

func TestReadVarintStopsAtTenBytes(t *testing.T) {
	// Eleven continuation bytes cannot encode a 64 bit value.
	if _, n := readVarint([]byte{
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01,
	}); n != 0 {
		t.Fatalf("a varint of eleven bytes was accepted after %d", n)
	}
}

func TestWriteFrameRefusesAnOversizedResponse(t *testing.T) {
	var out bytes.Buffer
	w := bufio.NewWriter(&out)
	if err := writeFrame(w, make([]byte, maxFrame+1)); err == nil {
		t.Fatal("a response above the frame limit must be refused")
	}
}

func TestServeReportsAWriteFailure(t *testing.T) {
	req := requestOf{caseID: "w", operation: opValidate, kind: "siren", input: "012345674"}
	var in bytes.Buffer
	frame(&in, req.encode())

	for _, allow := range []int{0, 4} {
		w := &errWriter{allow: allow, err: errors.New("the pipe is closed")}
		if err := serve(bytes.NewReader(in.Bytes()), w); err == nil {
			t.Fatalf("allowing %d bytes: a failing standard output must be reported", allow)
		}
	}
}

func TestAnAmbiguousProfileIsReportedAsAFailure(t *testing.T) {
	// Canonicalize refuses a profile no definition declares, and the testee
	// has to carry that back rather than invent a result.
	resp := exchange(t, requestOf{
		caseID: "p", operation: opCanonicalize,
		kind: "siren", input: "012345674", profile: "strict", presentProfile: true,
	})
	failure, ok := resp[5]
	if !ok {
		t.Fatalf("an unknown profile must be a failure, got fields %v", keys(resp))
	}
	if k := varintOf(t, fields(t, failure)[1]); k != failureInternalError {
		t.Fatalf("failure kind %d", k)
	}
}

func TestAnUnexpectedLoadErrorBecomesAFailure(t *testing.T) {
	// encodeLoad maps the two documented refusals and reports anything else as
	// a failure rather than claiming a refusal it did not get.
	body := encodeLoad("x", nil, errors.New("something the generator never returns"))
	m := fields(t, body)
	failure, ok := m[5]
	if !ok {
		t.Fatalf("an undocumented error must become a failure, got fields %v", keys(m))
	}
	if k := varintOf(t, fields(t, failure)[1]); k != failureInternalError {
		t.Fatalf("failure kind %d", k)
	}
}
