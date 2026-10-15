package goja

import (
	"math"
	"math/big"
)

// Limits bounds the memory and Go stack a single operator or builtin call can
// use on behalf of a script.
//
// Interrupt only takes effect between instructions, so it cannot stop an
// operation that is already running. Without limits one call such as
// "x".repeat(2**40), new ArrayBuffer(2**40), 2n ** (2n ** 40n), or String() of
// an array nested a million levels deep can exhaust the host's memory or Go
// stack. Both are fatal to the whole Go process rather than to the script, and
// recover() cannot catch them.
//
// A zero field means no limit, and the zero Limits is the default. An
// operation that would exceed a limit throws a RangeError instead, which the
// script can catch; source text nested too deeply is a SyntaxError.
//
// The limits apply to each operation, not to the total memory a script holds,
// so a loop can still accumulate memory one bounded step at a time. Each step
// is a separate instruction, however, so Interrupt can stop it.
type Limits struct {
	// MaxStringLength is the longest string, in UTF-16 code units, that string
	// concatenation (+, += and template literals) may create, and that the
	// builtins whose result is not bounded by a small multiple of their inputs
	// may create: String.prototype.repeat, padStart, padEnd, concat, replace,
	// replaceAll and the RegExp replace they delegate to, String.raw,
	// Array.prototype.join and toLocaleString (and the %TypedArray% versions),
	// JSON.stringify, and the source the Function constructor assembles.
	MaxStringLength int

	// MaxArrayElements is the most elements a single Array may store, and the
	// most values a builtin collects into one list: an argument list built by
	// spread, Function.prototype.apply or Reflect.apply, the elements an
	// iterator over an Array or TypedArray yields (to the Map and Set
	// constructors, Promise.all or Object.fromEntries, for example), the keys
	// of a String or TypedArray, and the matches of a global RegExp,
	// String.prototype.split or replaceAll.
	//
	// Setting length allocates nothing, so new Array(2**32 - 1) is allowed;
	// filling, spreading or copying it is not.
	MaxArrayElements int

	// MaxArrayBufferLength is the largest ArrayBuffer, in bytes, including the
	// buffers that TypedArray constructors and %TypedArray%.from allocate.
	MaxArrayBufferLength int

	// MaxBigIntBits is the longest BigInt, in bits, that *, **, << (and >> by a
	// negative amount), BigInt.asIntN and BigInt.asUintN may produce. The check
	// uses an upper bound on the size of the result, so it refuses before any
	// work is done.
	MaxBigIntBits int

	// MaxNestingDepth bounds recursion that happens in Go rather than in
	// JavaScript, which SetMaxCallStackSize does not cover:
	//
	//   - native functions calling native functions, as in implicit
	//     toString/valueOf conversions (String(), template literals, ==, join),
	//     toLocaleString, toJSON, accessors, bound functions and proxy traps;
	//   - how deeply JSON.stringify, JSON.parse (and its reviver) and
	//     Array.prototype.flat descend into nested values;
	//   - the length of prototype chains and Proxy target chains, which every
	//     property lookup walks recursively. A [[SetPrototypeOf]] that would
	//     close a cycle through a Proxy, which property lookups could never
	//     leave, is refused as well;
	//   - how deeply source text compiled by RunString, eval and the Function
	//     constructor may nest. This one is a SyntaxError.
	//
	// The depth counts levels of Go recursion, so one level of a data
	// structure can cost more than one: String() of nested arrays uses two
	// per level (toString, then join).
	MaxNestingDepth int
}

// SetLimits sets the limits described by Limits. The zero Limits removes them.
//
// Like the other Set* methods it is not safe for concurrent use. Call it before
// running any code: values a script created earlier, such as bound functions,
// may escape some of the accounting.
func (r *Runtime) SetLimits(limits Limits) {
	r.limits = limits
}

const (
	errMsgStringLength = "Invalid string length"
	errMsgArrayLength  = "Invalid array length"
	errMsgBufferLength = "Array buffer allocation failed"
	errMsgBigIntSize   = "Maximum BigInt size exceeded"
	errMsgNestingDepth = "Maximum nesting depth exceeded"
	errMsgChainLength  = "Maximum prototype chain length exceeded"
)

// checkStringLength throws a RangeError if a string of length n exceeds
// MaxStringLength.
func (r *Runtime) checkStringLength(n int64) {
	if limit := r.limits.MaxStringLength; limit > 0 && n > int64(limit) {
		panic(rangeError(errMsgStringLength))
	}
}

// checkArrayLength throws a RangeError if an array or list of n elements
// exceeds MaxArrayElements.
func (r *Runtime) checkArrayLength(n int64) {
	if limit := r.limits.MaxArrayElements; limit > 0 && n > int64(limit) {
		panic(rangeError(errMsgArrayLength))
	}
}

// arrayPrealloc returns how much room to reserve for a list that will hold at
// most n elements: n, or less if n exceeds MaxArrayElements, so that the
// reservation itself cannot exceed the limit.
func (r *Runtime) arrayPrealloc(n int64) int64 {
	if limit := int64(r.limits.MaxArrayElements); limit > 0 && n > limit {
		return limit
	}
	return n
}

// matchLimit returns how many results to ask for when collecting all matches:
// one more than MaxArrayElements, so that checkArrayLength can tell that there
// are too many, or -1 if it is not set.
func (r *Runtime) matchLimit() int {
	if limit := r.limits.MaxArrayElements; limit > 0 && limit < math.MaxInt {
		return limit + 1
	}
	return -1
}

// limitStringBuilder makes b throw a RangeError if it grows past MaxStringLength.
func (r *Runtime) limitStringBuilder(b *StringBuilder) {
	b.limit = r.limits.MaxStringLength
}

func (r *Runtime) allocByteSlice(size int) []byte {
	if limit := r.limits.MaxArrayBufferLength; limit > 0 && size > limit {
		panic(rangeError(errMsgBufferLength))
	}
	return allocByteSlice(size)
}

// checkBigIntBits throws a RangeError if a BigInt result of up to bits bits
// exceeds MaxBigIntBits. A negative value stands for a bound too large to
// represent.
func (r *Runtime) checkBigIntBits(bits int64) {
	if limit := r.limits.MaxBigIntBits; limit > 0 && (bits < 0 || bits > int64(limit)) {
		panic(rangeError(errMsgBigIntSize))
	}
}

// checkBigIntMul checks x * y.
func (r *Runtime) checkBigIntMul(x, y *big.Int) {
	if r.limits.MaxBigIntBits > 0 {
		r.checkBigIntBits(int64(x.BitLen()) + int64(y.BitLen()))
	}
}

// checkBigIntExp checks x ** y for y >= 0.
func (r *Runtime) checkBigIntExp(x, y *big.Int) {
	if r.limits.MaxBigIntBits <= 0 || x.CmpAbs(big.NewInt(1)) <= 0 || y.Sign() == 0 {
		// 0, 1 and -1 stay that small; x ** 0 is 1.
		return
	}
	if !y.IsInt64() {
		r.checkBigIntBits(-1)
	}
	xBits, n := int64(x.BitLen()), y.Int64()
	if n > (1<<62)/xBits {
		r.checkBigIntBits(-1)
	}
	r.checkBigIntBits(xBits * n)
}

// checkBigIntShift checks a left shift of x by |n| bits.
func (r *Runtime) checkBigIntShift(x, n *big.Int) {
	if r.limits.MaxBigIntBits > 0 && x.Sign() != 0 {
		if n.BitLen() > 62 {
			r.checkBigIntBits(-1)
		}
		r.checkBigIntBits(int64(x.BitLen()) + new(big.Int).Abs(n).Int64())
	}
}

// enterNesting starts one level of Go recursion. It throws a RangeError if
// that exceeds MaxNestingDepth, and reports whether the level was counted.
// Pair it with leaveNesting:
//
//	defer r.leaveNesting(r.enterNesting())
func (r *Runtime) enterNesting() bool {
	limit := r.limits.MaxNestingDepth
	if limit <= 0 {
		return false
	}
	if r.nestingDepth >= limit {
		panic(rangeError(errMsgNestingDepth))
	}
	r.nestingDepth++
	return true
}

func (r *Runtime) leaveNesting(counted bool) {
	if counted {
		r.nestingDepth--
	}
}

// guardNative wraps a native function about to be called from Go code, rather
// than from the VM, so that the call counts towards MaxNestingDepth. Calls the
// VM makes are already bounded by SetMaxCallStackSize.
func (r *Runtime) guardNative(f func(FunctionCall) Value) func(FunctionCall) Value {
	if r.limits.MaxNestingDepth <= 0 {
		return f
	}
	return func(call FunctionCall) Value {
		defer r.leaveNesting(r.enterNesting())
		return f(call)
	}
}

// chainNext returns the object that a property lookup on o continues into when
// o does not have the property itself: a Proxy's target (in the absence of a
// trap), otherwise o's prototype. It never calls a trap.
func chainNext(o *Object) *Object {
	if p, ok := o.self.(*proxyObject); ok {
		return p.target
	}
	return o.self.proto()
}

// admitChainLink checks a prototype or Proxy target link from child to parent
// against MaxNestingDepth before it is made, and records it. child is nil when
// the link belongs to an object that is being created.
//
// Property lookups recurse once per object in the chain, so the limit applies
// to the longest chain through the new link: from child's deepest descendant
// to the end of parent's chain. Each object records in chainHeight the longest
// chain known to lead to it, which is how re-linking an object that already
// has descendants is accounted for.
//
// It throws a RangeError if the chain would be too long, and reports false if
// parent's chain contains child: a cycle that [[SetPrototypeOf]] does not
// detect because it stops at the first Proxy.
func (r *Runtime) admitChainLink(child, parent *Object) bool {
	limit := r.limits.MaxNestingDepth
	if limit <= 0 || parent == nil {
		return true
	}
	var height uint32
	if child != nil {
		height = child.chainHeight
	} else if parent.chainHeight > 0 {
		// A new object has no descendants, and an existing descendant of
		// parent at least as deep was already admitted against the same chain.
		return true
	}
	n := int64(height) + 1
	for p := parent; p != nil; p = chainNext(p) {
		if p == child {
			return false
		}
		if n++; n > int64(limit) {
			panic(rangeError(errMsgChainLength))
		}
	}
	h := height + 1
	for p := parent; p != nil && p.chainHeight < h; p = chainNext(p) {
		p.chainHeight = h
		h++
	}
	return true
}
