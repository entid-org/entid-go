// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

// Command businessid-testee exposes this engine to the LibBusinessID
// conformance runner over the protocol of spec/testee.proto.
//
// It reads one request on standard input, writes one response on standard
// output, and repeats. Each message is preceded by its length as a 32 bit
// unsigned integer in little endian order; the length covers the message
// alone. The exchange is strictly synchronous.
//
// It never reads the corpus, never sees an expected result, and never adapts
// its behaviour to the case it receives: the case identifier is echoed so that
// a desynchronized exchange is detected, and used for nothing else. It
// translates a request into an API call and a result into a response, and
// nothing more. Keeping it this small is what makes the absence of cheating
// verifiable by reading it.
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	businessid "github.com/libbusinessid/businessid-go"
	"github.com/libbusinessid/businessid-go/internal/gen"
)

func main() {
	if err := serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "businessid-testee: %v\n", err)
		os.Exit(1)
	}
}

func serve(stdin io.Reader, stdout io.Writer) error {
	in := bufio.NewReader(stdin)
	out := bufio.NewWriter(stdout)

	engine := businessid.New()
	for {
		frame, err := readFrame(in)
		if errors.Is(err, io.EOF) {
			return out.Flush()
		}
		if err != nil {
			return err
		}
		req, err := decodeRequest(frame)
		if err != nil {
			return err
		}
		if err := writeFrame(out, answer(engine, req)); err != nil {
			return err
		}
		// The runner reads exactly one response before sending the next
		// request, so nothing may sit in the buffer.
		if err := out.Flush(); err != nil {
			return err
		}
	}
}

// Operations of conformance.proto, which testee.proto reuses.
const (
	opCanonicalize     = 1
	opValidateFormat   = 2
	opValidateChecksum = 3
	opValidate         = 4
	opLoadRuleset      = 5
)

// Failure kinds of testee.proto.
const (
	failureUnsupportedOperation = 1
	failureInternalError        = 2
)

// request is one decoded TesteeRequest.
//
// profile carries explicit presence: an absent profile is what lets a
// definition apply its own default, and testee.proto is emphatic that it must
// never be conflated with a profile named "".
type request struct {
	caseID       string
	operation    uint64
	input        string
	profile      string
	hasProfile   bool
	kind         string
	countryCode  string
	rulesPayload []byte
}

// answer runs one request through the public API and encodes the response.
func answer(engine *businessid.Engine, req request) []byte {
	if req.operation == opLoadRuleset {
		// Section 8.8 addresses these cases to the generator: this engine
		// compiles its rules ahead of time and never loads a bundle at run
		// time, so the generator is what answers them.
		bundle, err := gen.Load(req.rulesPayload)
		return encodeLoad(req.caseID, bundle, err)
	}

	if req.hasProfile && req.profile == "" {
		// The empty string is how this API spells "no profile", so a profile
		// explicitly named "" has no representation here. Reporting a failure
		// keeps the gap visible instead of silently answering as if none had
		// been requested.
		return encodeFailure(req.caseID, failureInternalError,
			`a profile named "" cannot be expressed by this engine`)
	}
	in := businessid.Input{
		Kind:        req.kind,
		Value:       req.input,
		CountryCode: req.countryCode,
		Profile:     businessid.Profile(req.profile),
	}

	if req.operation == opCanonicalize {
		got, err := engine.Canonicalize(in)
		if err != nil {
			return encodeFailure(req.caseID, failureInternalError, err.Error())
		}
		return encodeCanonicalization(req.caseID, got)
	}

	var (
		got businessid.Report
		err error
	)
	switch req.operation {
	case opValidateFormat:
		got, err = engine.ValidateFormat(in)
	case opValidateChecksum:
		got, err = engine.ValidateChecksum(in)
	case opValidate:
		got, err = engine.Validate(in)
	default:
		return encodeFailure(req.caseID, failureUnsupportedOperation,
			fmt.Sprintf("operation %d", req.operation))
	}
	if err != nil {
		return encodeFailure(req.caseID, failureInternalError, err.Error())
	}
	return encodeReport(req.caseID, got)
}

// Framing.

// maxFrame bounds one message. A request describes a single identifier, which
// the specification bounds to 1024 bytes, or one rule bundle, which it bounds
// to 16 MiB; a little more than that stops a runaway length before it
// allocates.
const maxFrame = 17 << 20

func readFrame(r *bufio.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(header[:])
	if n > maxFrame {
		return nil, fmt.Errorf("frame of %d bytes exceeds the %d byte limit", n, maxFrame)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}

func writeFrame(w *bufio.Writer, body []byte) error {
	if len(body) > maxFrame {
		return fmt.Errorf("response of %d bytes exceeds the %d byte limit", len(body), maxFrame)
	}
	var header [4]byte
	// The length covers the message alone, not the four bytes announcing it.
	binary.LittleEndian.PutUint32(header[:], uint32(len(body))) //nolint:gosec // bounded just above
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// Decoding.

func decodeRequest(body []byte) (request, error) {
	var req request
	for i := 0; i < len(body); {
		tag, n := readVarint(body[i:])
		if n == 0 {
			return req, errors.New("malformed request")
		}
		i += n
		field, wire := int(tag>>3), int(tag&7)
		switch wire {
		case 0:
			v, n := readVarint(body[i:])
			if n == 0 {
				return req, errors.New("malformed request")
			}
			i += n
			if field == 2 {
				req.operation = v
			}
		case 2:
			size, n := readVarint(body[i:])
			if n == 0 {
				return req, errors.New("malformed request")
			}
			i += n
			remaining := len(body) - i
			// The bound is compared unsigned: narrowing the length first would
			// let a value above the platform int wrap into something
			// acceptable, and read past the frame.
			if remaining < 0 || size > uint64(remaining) {
				return req, errors.New("malformed request")
			}
			width := int(size) //nolint:gosec // bounded by remaining, an int
			value := body[i : i+width]
			i += width
			switch field {
			case 1:
				req.caseID = string(value)
			case 3:
				req.input = string(value)
			case 4:
				req.profile, req.hasProfile = string(value), true
			case 5:
				req.kind = string(value)
			case 6:
				req.countryCode = string(value)
			case 7:
				req.rulesPayload = value
			}
		default:
			return req, fmt.Errorf("unsupported wire type %d in a request", wire)
		}
	}
	return req, nil
}

func readVarint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7f) << (7 * uint(i))
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return 0, 0
}

// Encoding.

func encodeCanonicalization(caseID string, got businessid.Canonical) []byte {
	var body []byte
	body = appendString(body, 1, got.Kind)
	body = appendString(body, 2, got.CanonicalValue)
	if got.CountryCode != "" {
		body = appendString(body, 3, got.CountryCode)
	}
	body = appendVarint(body, 4, uint64(got.Status))
	body = appendVarint(body, 5, uint64(got.Reason))
	return wrap(caseID, 2, body)
}

func encodeReport(caseID string, got businessid.Report) []byte {
	var body []byte
	body = appendString(body, 1, got.Kind)
	body = appendString(body, 2, got.CanonicalValue)
	if got.CountryCode != "" {
		body = appendString(body, 3, got.CountryCode)
	}
	body = appendBytes(body, 4, encodeStep(got.Format))
	body = appendBytes(body, 5, encodeStep(got.Checksum))
	return wrap(caseID, 3, body)
}

func encodeStep(s businessid.Step) []byte {
	body := appendVarint(nil, 1, uint64(s.Status))
	body = appendVarint(body, 2, uint64(s.Reason))
	// Field 3 carries the key the rule names, and stays absent when the
	// result precedes every assertion: a dispatch outcome, a safety bound, or
	// a step that did not run. Section 11.2 of engine.md compares the key
	// alongside the reason code, so an absent one has to mean "no rule spoke"
	// rather than "the engine did not say".
	if s.MessageKey != "" {
		body = appendString(body, 3, s.MessageKey)
	}
	return body
}

// encodeLoad reports what the generator made of a bundle. A hostile bundle must
// never be accepted, and the typed error says which kind of refusal it was.
func encodeLoad(caseID string, bundle *gen.Bundle, err error) []byte {
	var body []byte
	switch {
	case err == nil && bundle != nil:
		body = appendVarint(body, 1, 1)
	case errors.Is(err, gen.ErrIncompatibleRuleset):
		body = appendString(body, 2, "incompatible_ruleset")
	case errors.Is(err, gen.ErrInvalidRuleset):
		body = appendString(body, 2, "invalid_ruleset")
	default:
		return encodeFailure(caseID, failureInternalError, fmt.Sprint(err))
	}
	return wrap(caseID, 4, body)
}

func encodeFailure(caseID string, kind uint64, detail string) []byte {
	body := appendVarint(nil, 1, kind)
	return wrap(caseID, 5, appendString(body, 2, detail))
}

// wrap builds a TesteeResponse carrying one branch of its result oneof.
func wrap(caseID string, field int, payload []byte) []byte {
	body := appendString(nil, 1, caseID)
	return appendBytes(body, field, payload)
}

// appendTag writes a field tag. Field numbers and wire types are literals in
// this file, so the conversion cannot lose anything.
func appendTag(dst []byte, field, wire int) []byte {
	return appendRawVarint(dst, uint64(field)<<3|uint64(wire)) //nolint:gosec // field and wire are literals
}

func appendVarint(dst []byte, field int, v uint64) []byte {
	return appendRawVarint(appendTag(dst, field, 0), v)
}

func appendBytes(dst []byte, field int, payload []byte) []byte {
	dst = appendRawVarint(appendTag(dst, field, 2), uint64(len(payload)))
	return append(dst, payload...)
}

func appendString(dst []byte, field int, s string) []byte {
	return appendBytes(dst, field, []byte(s))
}

func appendRawVarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}
