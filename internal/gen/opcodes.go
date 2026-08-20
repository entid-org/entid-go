// Copyright The LibBusinessID Authors.
// SPDX-License-Identifier: Apache-2.0

package gen

// Opcode identifies one of the 61 V1 operations across all seven categories.
// The wire form splits them into per category enums; folding them into a single
// space lets validation and emission share one dispatch table.
type Opcode int

// Category groups opcodes the way rules.proto splits its operation messages.
// A program kind accepts a fixed set of categories.
type Category int

// Operation categories.
const (
	CatString Category = iota
	CatInteger
	CatPredicate
	CatCanonicalization
	CatAssertion
	CatChecksum
	CatCall
)

// The 61 V1 opcodes.
const (
	OpInvalid Opcode = iota

	// String operations.
	OpConstant
	OpValue
	OpSubject
	OpCountryCode
	OpSlice
	OpSliceFrom
	OpSliceTo
	OpBeforeFirst
	OpAfterFirst
	OpStripPrefix
	OpConcat

	// Integer operations.
	OpDigitsToInteger
	OpModDigits
	OpWeightedSum
	OpModulo
	OpComplement
	OpRemainderMap

	// Predicate operations.
	OpIsEmpty
	OpIsAbsent
	OpEquals
	OpLengthEq
	OpLengthIn
	OpLengthBetween
	OpASCIIDigits
	OpASCIIUpperLetters
	OpASCIIAlphanumeric
	OpASCIICharset
	OpStartsWith
	OpEndsWith
	OpPrefixIn
	OpCharAtIn
	OpContains
	OpAll
	OpAny
	OpNot
	OpProfileIs
	OpIntegerIs

	// Canonicalization operations.
	OpCanonSequence
	OpTrimWhitespace
	OpRemoveWhitespace
	OpUppercaseASCII
	OpRemoveChars
	OpReplacePrefix
	OpPrepend
	OpAppend
	OpInsert
	OpLeftPad
	OpPrependCountryIfMissing
	OpCanonWhen

	// Assertion operations.
	OpAssertSequence
	OpRequire

	// Checksum operations.
	OpLuhn
	OpISO7064Mod97_10
	OpCompareDigit
	OpCompareSlice
	OpChoose
	OpChecksumWhen
	OpAllChecks
	OpAnyCheck
	OpUnsupportedChecksum
	OpCompareConstant

	// Call operations.
	OpCallFormat
	OpCallChecksum

	opCount
)

// param is a bit identifying one parameter field of an operation message. The
// allowed set refuses a stray parameter and the required set refuses a missing
// one, as check 12 of ir.md section 10 demands.
type param uint32

const (
	pText param = 1 << iota
	pReplacement
	pStart
	pEnd
	pIndex
	pLength
	pMinLength
	pMaxLength
	pValues
	pLengths
	pModulus
	pWeights
	pAlignment
	pMapping
	pRemainderValues
	pReasonCode
	pMessageKey
	pProgramID
	pConstant
	pAlphabet
)

var paramNames = map[param]string{
	pText:            "text",
	pReplacement:     "replacement",
	pStart:           "start",
	pEnd:             "end",
	pIndex:           "index",
	pLength:          "length",
	pMinLength:       "min_length",
	pMaxLength:       "max_length",
	pValues:          "values",
	pLengths:         "lengths",
	pModulus:         "modulus",
	pWeights:         "weights",
	pAlignment:       "alignment",
	pMapping:         "mapping",
	pRemainderValues: "remainder_values",
	pReasonCode:      "reason_code",
	pMessageKey:      "message_key",
	pProgramID:       "program_id",
	pConstant:        "constant",
	pAlphabet:        "alphabet",
}

// unbounded marks an operand list with no upper bound.
const unbounded = -1

// opSpec is the frozen contract of one operation: its category, its output
// type, its operand shape, its parameters and the capabilities it requires.
type opSpec struct {
	name     string
	cat      Category
	output   ValueType
	fixed    []ValueType // operands at fixed positions
	repeated ValueType   // type of the trailing repeated operands
	minRep   int
	maxRep   int // unbounded for no upper limit
	required param
	optional param
	features []uint32
}

func (s *opSpec) allowed() param { return s.required | s.optional }

var opSpecs = [opCount]opSpec{
	// String operations.
	OpConstant:    {name: "STRING_OP_KIND_CONSTANT", cat: CatString, output: ValueString, required: pText, features: []uint32{1}},
	OpValue:       {name: "STRING_OP_KIND_VALUE", cat: CatString, output: ValueString, features: []uint32{1}},
	OpSubject:     {name: "STRING_OP_KIND_SUBJECT", cat: CatString, output: ValueString, features: []uint32{1}},
	OpCountryCode: {name: "STRING_OP_KIND_COUNTRY_CODE", cat: CatString, output: ValueString, features: []uint32{1, 5}},
	OpSlice: {name: "STRING_OP_KIND_SLICE", cat: CatString, output: ValueString,
		fixed: []ValueType{ValueString}, required: pStart | pEnd, features: []uint32{1, 10}},
	OpSliceFrom: {name: "STRING_OP_KIND_SLICE_FROM", cat: CatString, output: ValueString,
		fixed: []ValueType{ValueString}, required: pStart, features: []uint32{1, 10}},
	OpSliceTo: {name: "STRING_OP_KIND_SLICE_TO", cat: CatString, output: ValueString,
		fixed: []ValueType{ValueString}, required: pEnd, features: []uint32{1, 10}},
	OpBeforeFirst: {name: "STRING_OP_KIND_BEFORE_FIRST", cat: CatString, output: ValueString,
		fixed: []ValueType{ValueString}, required: pText, features: []uint32{1, 10}},
	OpAfterFirst: {name: "STRING_OP_KIND_AFTER_FIRST", cat: CatString, output: ValueString,
		fixed: []ValueType{ValueString}, required: pText, features: []uint32{1, 10}},
	OpStripPrefix: {name: "STRING_OP_KIND_STRIP_PREFIX", cat: CatString, output: ValueString,
		fixed: []ValueType{ValueString}, required: pText, features: []uint32{1, 10}},
	OpConcat: {name: "STRING_OP_KIND_CONCAT", cat: CatString, output: ValueString,
		repeated: ValueString, minRep: 1, maxRep: 256, features: []uint32{1, 10}},

	// Integer operations.
	OpDigitsToInteger: {name: "INTEGER_OP_KIND_DIGITS_TO_INTEGER", cat: CatInteger, output: ValueInteger,
		fixed: []ValueType{ValueString}, features: []uint32{1, 30}},
	OpModDigits: {name: "INTEGER_OP_KIND_MOD_DIGITS", cat: CatInteger, output: ValueInteger,
		fixed: []ValueType{ValueString}, required: pModulus, features: []uint32{1, 30}},
	OpWeightedSum: {name: "INTEGER_OP_KIND_WEIGHTED_SUM", cat: CatInteger, output: ValueInteger,
		fixed: []ValueType{ValueString}, required: pWeights | pAlignment | pMapping, optional: pAlphabet,
		features: []uint32{1, 30, 33}},
	OpModulo: {name: "INTEGER_OP_KIND_MODULO", cat: CatInteger, output: ValueInteger,
		fixed: []ValueType{ValueInteger}, required: pModulus, features: []uint32{1, 30}},
	OpComplement: {name: "INTEGER_OP_KIND_COMPLEMENT", cat: CatInteger, output: ValueInteger,
		fixed: []ValueType{ValueInteger}, required: pModulus, features: []uint32{1, 30}},
	OpRemainderMap: {name: "INTEGER_OP_KIND_REMAINDER_MAP", cat: CatInteger, output: ValueInteger,
		fixed: []ValueType{ValueInteger}, required: pRemainderValues, features: []uint32{1, 30}},

	// Predicate operations.
	OpIsEmpty: {name: "PREDICATE_OP_KIND_IS_EMPTY", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, features: []uint32{1, 20}},
	OpIsAbsent: {name: "PREDICATE_OP_KIND_IS_ABSENT", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, features: []uint32{1, 10}},
	OpEquals: {name: "PREDICATE_OP_KIND_EQUALS", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString, ValueString}, features: []uint32{1, 20}},
	OpLengthEq: {name: "PREDICATE_OP_KIND_LENGTH_EQ", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pLength, features: []uint32{1, 20}},
	OpLengthIn: {name: "PREDICATE_OP_KIND_LENGTH_IN", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pLengths, features: []uint32{1, 20}},
	OpLengthBetween: {name: "PREDICATE_OP_KIND_LENGTH_BETWEEN", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pMinLength | pMaxLength, features: []uint32{1, 20}},
	OpASCIIDigits: {name: "PREDICATE_OP_KIND_ASCII_DIGITS", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, features: []uint32{1, 2}},
	OpASCIIUpperLetters: {name: "PREDICATE_OP_KIND_ASCII_UPPER_LETTERS", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, features: []uint32{1, 2}},
	OpASCIIAlphanumeric: {name: "PREDICATE_OP_KIND_ASCII_ALPHANUMERIC", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, features: []uint32{1, 2}},
	OpASCIICharset: {name: "PREDICATE_OP_KIND_ASCII_CHARSET", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pText, features: []uint32{1, 2}},
	OpStartsWith: {name: "PREDICATE_OP_KIND_STARTS_WITH", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pText, features: []uint32{1, 20}},
	OpEndsWith: {name: "PREDICATE_OP_KIND_ENDS_WITH", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pText, features: []uint32{1, 20}},
	OpPrefixIn: {name: "PREDICATE_OP_KIND_PREFIX_IN", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pValues, features: []uint32{1, 20}},
	OpCharAtIn: {name: "PREDICATE_OP_KIND_CHAR_AT_IN", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pIndex | pText, features: []uint32{1, 20}},
	OpContains: {name: "PREDICATE_OP_KIND_CONTAINS", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueString}, required: pText, features: []uint32{1, 20}},
	OpAll: {name: "PREDICATE_OP_KIND_ALL", cat: CatPredicate, output: ValueBoolean,
		repeated: ValueBoolean, minRep: 1, maxRep: unbounded, features: []uint32{1, 20}},
	OpAny: {name: "PREDICATE_OP_KIND_ANY", cat: CatPredicate, output: ValueBoolean,
		repeated: ValueBoolean, minRep: 1, maxRep: unbounded, features: []uint32{1, 20}},
	OpNot: {name: "PREDICATE_OP_KIND_NOT", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueBoolean}, features: []uint32{1, 20}},
	OpProfileIs: {name: "PREDICATE_OP_KIND_PROFILE_IS", cat: CatPredicate, output: ValueBoolean,
		required: pText, features: []uint32{1, 21}},
	OpIntegerIs: {name: "PREDICATE_OP_KIND_INTEGER_IS", cat: CatPredicate, output: ValueBoolean,
		fixed: []ValueType{ValueInteger}, required: pConstant, features: []uint32{1, 30, 35}},

	// Canonicalization operations.
	OpCanonSequence: {name: "CANONICALIZATION_OP_KIND_SEQUENCE", cat: CatCanonicalization, output: ValueCanonicalizationStep,
		repeated: ValueCanonicalizationStep, minRep: 0, maxRep: unbounded, features: []uint32{1, 3}},
	OpTrimWhitespace: {name: "CANONICALIZATION_OP_KIND_TRIM_WHITESPACE", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, features: []uint32{1, 2, 3}},
	OpRemoveWhitespace: {name: "CANONICALIZATION_OP_KIND_REMOVE_WHITESPACE", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, features: []uint32{1, 2, 3}},
	OpUppercaseASCII: {name: "CANONICALIZATION_OP_KIND_UPPERCASE_ASCII", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, features: []uint32{1, 2, 3}},
	OpRemoveChars: {name: "CANONICALIZATION_OP_KIND_REMOVE_CHARS", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, required: pText, features: []uint32{1, 3}},
	OpReplacePrefix: {name: "CANONICALIZATION_OP_KIND_REPLACE_PREFIX", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, required: pText | pReplacement, features: []uint32{1, 3}},
	OpPrepend: {name: "CANONICALIZATION_OP_KIND_PREPEND", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, required: pText, features: []uint32{1, 3}},
	OpAppend: {name: "CANONICALIZATION_OP_KIND_APPEND", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, required: pText, features: []uint32{1, 3}},
	OpInsert: {name: "CANONICALIZATION_OP_KIND_INSERT", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, required: pIndex | pText, features: []uint32{1, 3}},
	OpLeftPad: {name: "CANONICALIZATION_OP_KIND_LEFT_PAD", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, required: pLength | pText, features: []uint32{1, 3}},
	OpPrependCountryIfMissing: {name: "CANONICALIZATION_OP_KIND_PREPEND_COUNTRY_IF_MISSING", cat: CatCanonicalization,
		output: ValueCanonicalizationStep, features: []uint32{1, 3, 5}},
	OpCanonWhen: {name: "CANONICALIZATION_OP_KIND_WHEN", cat: CatCanonicalization, output: ValueCanonicalizationStep,
		fixed: []ValueType{ValueBoolean}, repeated: ValueCanonicalizationStep, minRep: 1, maxRep: unbounded,
		features: []uint32{1, 4}},

	// Assertion operations.
	OpAssertSequence: {name: "ASSERTION_OP_KIND_SEQUENCE", cat: CatAssertion, output: ValueAssertion,
		repeated: ValueAssertion, minRep: 1, maxRep: unbounded, features: []uint32{1, 20}},
	OpRequire: {name: "ASSERTION_OP_KIND_REQUIRE", cat: CatAssertion, output: ValueAssertion,
		fixed: []ValueType{ValueBoolean}, required: pReasonCode, optional: pMessageKey, features: []uint32{1, 20}},

	// Checksum operations.
	OpLuhn: {name: "CHECKSUM_OP_KIND_LUHN", cat: CatChecksum, output: ValueChecksumOutcome,
		fixed: []ValueType{ValueString}, optional: pMessageKey, features: []uint32{1, 30, 31}},
	OpISO7064Mod97_10: {name: "CHECKSUM_OP_KIND_ISO7064_MOD97_10", cat: CatChecksum, output: ValueChecksumOutcome,
		fixed: []ValueType{ValueString}, optional: pMessageKey, features: []uint32{1, 30, 32}},
	OpCompareDigit: {name: "CHECKSUM_OP_KIND_COMPARE_DIGIT", cat: CatChecksum, output: ValueChecksumOutcome,
		fixed: []ValueType{ValueInteger, ValueString}, required: pIndex, optional: pMessageKey, features: []uint32{1, 30}},
	OpCompareSlice: {name: "CHECKSUM_OP_KIND_COMPARE_SLICE", cat: CatChecksum, output: ValueChecksumOutcome,
		fixed: []ValueType{ValueInteger, ValueString}, required: pStart | pEnd, optional: pMessageKey, features: []uint32{1, 30}},
	OpChoose: {name: "CHECKSUM_OP_KIND_CHOOSE", cat: CatChecksum, output: ValueChecksumOutcome,
		repeated: ValueChecksumOutcome, minRep: 1, maxRep: unbounded, features: []uint32{1, 30}},
	OpChecksumWhen: {name: "CHECKSUM_OP_KIND_WHEN", cat: CatChecksum, output: ValueChecksumOutcome,
		fixed: []ValueType{ValueBoolean, ValueChecksumOutcome}, features: []uint32{1, 30}},
	OpAllChecks: {name: "CHECKSUM_OP_KIND_ALL_CHECKS", cat: CatChecksum, output: ValueChecksumOutcome,
		repeated: ValueChecksumOutcome, minRep: 1, maxRep: unbounded, features: []uint32{1, 30}},
	OpAnyCheck: {name: "CHECKSUM_OP_KIND_ANY_CHECK", cat: CatChecksum, output: ValueChecksumOutcome,
		repeated: ValueChecksumOutcome, minRep: 1, maxRep: unbounded, features: []uint32{1, 30}},
	OpUnsupportedChecksum: {name: "CHECKSUM_OP_KIND_UNSUPPORTED", cat: CatChecksum, output: ValueChecksumOutcome,
		required: pReasonCode, optional: pMessageKey, features: []uint32{1, 30}},
	OpCompareConstant: {name: "CHECKSUM_OP_KIND_COMPARE_CONSTANT", cat: CatChecksum, output: ValueChecksumOutcome,
		fixed: []ValueType{ValueInteger}, required: pConstant, optional: pMessageKey, features: []uint32{1, 30, 34}},

	// Call operations.
	OpCallFormat: {name: "CALL_OP_KIND_FORMAT", cat: CatCall, output: ValueAssertion,
		fixed: []ValueType{ValueString}, required: pProgramID, features: []uint32{1, 11, 20}},
	OpCallChecksum: {name: "CALL_OP_KIND_CHECKSUM", cat: CatCall, output: ValueChecksumOutcome,
		fixed: []ValueType{ValueString}, required: pProgramID, features: []uint32{1, 11, 30}},
}

func (o Opcode) spec() *opSpec { return &opSpecs[o] }

// String returns the IR name of the opcode.
func (o Opcode) String() string {
	if o <= OpInvalid || o >= opCount {
		return "unknown operation"
	}
	return opSpecs[o].name
}

// Category returns the operation category of the opcode.
func (o Opcode) Category() Category { return opSpecs[o].cat }

// Per category wire enum tables. A value absent from a table is an unknown
// operation, refused by check 10.
var (
	stringOps = map[int32]Opcode{
		1: OpConstant, 2: OpValue, 3: OpSubject, 4: OpCountryCode, 5: OpSlice,
		6: OpSliceFrom, 7: OpSliceTo, 8: OpBeforeFirst, 9: OpAfterFirst,
		10: OpStripPrefix, 11: OpConcat,
	}
	integerOps = map[int32]Opcode{
		1: OpDigitsToInteger, 2: OpModDigits, 3: OpWeightedSum,
		4: OpModulo, 5: OpComplement, 6: OpRemainderMap,
	}
	predicateOps = map[int32]Opcode{
		1: OpIsEmpty, 2: OpIsAbsent, 3: OpEquals, 4: OpLengthEq, 5: OpLengthIn,
		6: OpLengthBetween, 7: OpASCIIDigits, 8: OpASCIIUpperLetters,
		9: OpASCIIAlphanumeric, 10: OpASCIICharset, 11: OpStartsWith,
		12: OpEndsWith, 13: OpPrefixIn, 14: OpCharAtIn, 15: OpContains,
		16: OpAll, 17: OpAny, 18: OpNot, 19: OpProfileIs, 20: OpIntegerIs,
	}
	canonicalizationOps = map[int32]Opcode{
		1: OpCanonSequence, 2: OpTrimWhitespace, 3: OpRemoveWhitespace,
		4: OpUppercaseASCII, 5: OpRemoveChars, 6: OpReplacePrefix, 7: OpPrepend,
		8: OpAppend, 9: OpInsert, 10: OpLeftPad, 11: OpPrependCountryIfMissing,
		12: OpCanonWhen,
	}
	assertionOps = map[int32]Opcode{1: OpAssertSequence, 2: OpRequire}
	checksumOps  = map[int32]Opcode{
		1: OpLuhn, 2: OpISO7064Mod97_10, 3: OpCompareDigit, 4: OpCompareSlice,
		5: OpChoose, 6: OpChecksumWhen, 7: OpAllChecks, 8: OpAnyCheck,
		9: OpUnsupportedChecksum, 10: OpCompareConstant,
	}
	callOps = map[int32]Opcode{1: OpCallFormat, 2: OpCallChecksum}
)

// categoriesByProgramKind lists the operation categories each program kind
// accepts, per section 2 of ir.md.
var categoriesByProgramKind = map[ProgramKind][]Category{
	ProgramCanonicalization: {CatString, CatPredicate, CatCanonicalization},
	ProgramFormat:           {CatString, CatPredicate, CatAssertion, CatCall},
	ProgramChecksum:         {CatString, CatPredicate, CatInteger, CatChecksum, CatCall},
}

// callOpForProgram is the only call opcode a program kind may contain.
var callOpForProgram = map[ProgramKind]Opcode{
	ProgramFormat:   OpCallFormat,
	ProgramChecksum: OpCallChecksum,
}

// preCanonicalizationOps is the restricted step set a dispatcher
// pre-canonicalization program may use, per section 5 of ir.md.
var preCanonicalizationOps = map[Opcode]bool{
	OpCanonSequence:    true,
	OpTrimWhitespace:   true,
	OpRemoveWhitespace: true,
	OpUppercaseASCII:   true,
	OpRemoveChars:      true,
}
