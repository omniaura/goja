package goja

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

const deepArray200 = `let o = []; for (let i = 0; i < 200; i++) o = [o];`

var (
	stringLimit = Limits{MaxStringLength: 1000}
	arrayLimit  = Limits{MaxArrayElements: 1000}
	bufferLimit = Limits{MaxArrayBufferLength: 1000}
	bigIntLimit = Limits{MaxBigIntBits: 1000}
	depthLimit  = Limits{MaxNestingDepth: 100}
)

// Each case runs the same script three ways. With the limit, over must throw
// want, and fits, which differs only in size, must not. Without any limit over
// must succeed: that is what shows the limit, not some other check, stops it.
// Every over script is small enough to run unlimited; limits_fatal_linux_test.go
// shows the same paths at sizes that crash the process.
var limitCases = []struct {
	name, over, fits, want string
	limits                 Limits
}{
	{name: "repeat", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `"x".repeat(1001).length`, fits: `"x".repeat(1000).length`},
	{name: "padEnd", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `"x".padEnd(1001).length`, fits: `"x".padEnd(1000).length`},
	{name: "padStart", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `"x".padStart(1001, "ab").length`, fits: `"x".padStart(1000, "ab").length`},
	{name: "concat doubling", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `let s = "x"; for (let i = 0; i < 10; i++) s += s; s.length`,
		fits: `let s = "x"; for (let i = 0; i < 9; i++) s += s; s.length`},
	{name: "concat with non-string", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `("x".repeat(998) + 1234).length`, fits: `("x".repeat(996) + 1234).length`},
	{name: "template literal", limits: stringLimit, want: "RangeError: Invalid string length",
		over: "const a = 'x'.repeat(500); `${a}-${a}`.length", fits: "const a = 'x'.repeat(499); `${a}-${a}`.length"},
	{name: "String.prototype.concat", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `"x".repeat(600).concat("y".repeat(401)).length`, fits: `"x".repeat(600).concat("y".repeat(400)).length`},
	{name: "join of sparse array", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `new Array(1002).join().length`, fits: `new Array(1001).join().length`},
	{name: "join of strings", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `["x".repeat(600), "y".repeat(401)].join("").length`, fits: `["x".repeat(600), "y".repeat(400)].join("").length`},
	{name: "toLocaleString", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `new Array(1002).toLocaleString().length`, fits: `new Array(1001).toLocaleString().length`},
	{name: "TypedArray join", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `new Uint8Array(501).join().length`, fits: `new Uint8Array(500).join().length`},
	{name: "replace every match", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `"x".repeat(91).replace(/x/g, "y".repeat(11)).length`, fits: `"x".repeat(90).replace(/x/g, "y".repeat(11)).length`},
	{name: "replace substitution", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `"x".repeat(334).replace(/x+/, "$&$&$&").length`, fits: `"x".repeat(333).replace(/x+/, "$&$&$&").length`},
	{name: "replace through a RegExp subclass", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `class R extends RegExp {}; "x".repeat(91).replace(new R("x", "g"), "y".repeat(11)).length`,
		fits: `class R extends RegExp {}; "x".repeat(90).replace(new R("x", "g"), "y".repeat(11)).length`},
	{name: "replaceAll", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `"x".repeat(91).replaceAll("x", "y".repeat(11)).length`, fits: `"x".repeat(90).replaceAll("x", "y".repeat(11)).length`},
	{name: "String.raw", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `String.raw({raw: {length: 112}}).length`, fits: `String.raw({raw: {length: 111}}).length`},
	{name: "JSON.stringify sparse array", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `JSON.stringify(new Array(200)).length`, fits: `JSON.stringify(new Array(199)).length`},
	{name: "JSON.stringify escapes", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `JSON.stringify("\n".repeat(500)).length`, fits: `JSON.stringify("\n".repeat(499)).length`},
	{name: "Function constructor source", limits: stringLimit, want: "RangeError: Invalid string length",
		over: `new Function(" ".repeat(1000))()`, fits: `new Function(" ".repeat(900))()`},

	{name: "push", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `const a = []; for (let i = 0; i < 1001; i++) a.push(i); a.length`,
		fits: `const a = []; for (let i = 0; i < 1000; i++) a.push(i); a.length`},
	{name: "index writes", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `const a = []; for (let i = 0; i < 1001; i++) a[i] = i; a.length`,
		fits: `const a = []; for (let i = 0; i < 1000; i++) a[i] = i; a.length`},
	{name: "sparse writes", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `const a = []; for (let i = 0; i < 1001; i++) a[i * 10000] = i; a.length`,
		fits: `const a = []; for (let i = 0; i < 1000; i++) a[i * 10000] = i; a.length`},
	{name: "fill", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `new Array(1001).fill(0).length`, fits: `new Array(1000).fill(0).length`},
	{name: "length alone allocates nothing", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `new Array(2 ** 32 - 1).fill(0, 0, 1001).length`, fits: `new Array(2 ** 32 - 1).fill(0, 0, 1000).length`},
	{name: "spread into an array", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `[...new Array(1001)].length`, fits: `[...new Array(1000)].length`},
	{name: "spread into arguments", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `((...a) => a.length)(..."x".repeat(1001))`, fits: `((...a) => a.length)(..."x".repeat(1000))`},
	{name: "apply", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `Math.max.apply(null, {length: 1001})`, fits: `Math.max.apply(null, {length: 1000})`},
	{name: "Reflect.apply", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `Reflect.apply(Math.max, null, {length: 1001})`, fits: `Reflect.apply(Math.max, null, {length: 1000})`},
	{name: "Array.from array-like", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `Array.from({length: 1001}).length`, fits: `Array.from({length: 1000}).length`},
	{name: "Array.from iterable", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `Array.from("x".repeat(1001)).length`, fits: `Array.from("x".repeat(1000)).length`},
	{name: "map of sparse array", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `new Array(1001).map(x => x).length`, fits: `new Array(1000).map(x => x).length`},
	{name: "concat", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `const a = new Array(501).fill(0); a.concat(a).length`, fits: `const a = new Array(500).fill(0); a.concat(a).length`},
	{name: "flat of shared arrays", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `const a = new Array(501).fill(0); [a, a].flat().length`, fits: `const a = new Array(500).fill(0); [a, a].flat().length`},
	{name: "sort of array-like", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `const o = {length: 1001}; for (let i = 0; i < 1001; i++) o[i] = -i; Array.prototype.sort.call(o)[0]`,
		fits: `const o = {length: 1000}; for (let i = 0; i < 1000; i++) o[i] = -i; Array.prototype.sort.call(o)[0]`},
	{name: "toSorted of array-like", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `Array.prototype.toSorted.call({length: 1001}).length`, fits: `Array.prototype.toSorted.call({length: 1000}).length`},
	{name: "iterate sparse array", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `new Set(new Array(1001)).size`, fits: `new Set(new Array(1000)).size`},
	{name: "split", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `"x".repeat(1001).split("").length`, fits: `"x".repeat(1000).split("").length`},
	{name: "split with RegExp", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `"x".repeat(1000).split(/(?:)/).length`, fits: `"x".repeat(999).split(/(?:)/).length`},
	{name: "global match", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `"x".repeat(1001).match(/x/g).length`, fits: `"x".repeat(1000).match(/x/g).length`},
	{name: "global match through a RegExp subclass", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `class R extends RegExp {}; "x".repeat(1001).match(new R("x", "g")).length`,
		fits: `class R extends RegExp {}; "x".repeat(1000).match(new R("x", "g")).length`},
	{name: "global replace matches", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `"x".repeat(1001).replace(/x/g, "").length`, fits: `"x".repeat(1000).replace(/x/g, "").length`},
	{name: "replaceAll matches", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `"x".repeat(1001).replaceAll("x", "").length`, fits: `"x".repeat(1000).replaceAll("x", "").length`},
	{name: "keys of a string", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `Object.keys("x".repeat(1001)).length`, fits: `Object.keys("x".repeat(1000)).length`},
	{name: "keys of a TypedArray", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `Object.keys(new Uint8Array(1001)).length`, fits: `Object.keys(new Uint8Array(1000)).length`},
	{name: "JSON.parse array", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `JSON.parse("[" + "0,".repeat(1000) + "0]").length`, fits: `JSON.parse("[" + "0,".repeat(999) + "0]").length`},
	{name: "JSON.stringify replacer array", limits: arrayLimit, want: "RangeError: Invalid array length",
		over: `JSON.stringify({}, new Array(1001))`, fits: `JSON.stringify({}, new Array(1000))`},

	{name: "ArrayBuffer", limits: bufferLimit, want: "RangeError: Array buffer allocation failed",
		over: `new ArrayBuffer(1001).byteLength`, fits: `new ArrayBuffer(1000).byteLength`},
	{name: "TypedArray length", limits: bufferLimit, want: "RangeError: Array buffer allocation failed",
		over: `new Float64Array(126).byteLength`, fits: `new Float64Array(125).byteLength`},
	{name: "TypedArray from array-like", limits: bufferLimit, want: "RangeError: Array buffer allocation failed",
		over: `new Uint8Array({length: 1001}).byteLength`, fits: `new Uint8Array({length: 1000}).byteLength`},
	{name: "TypedArray.from", limits: bufferLimit, want: "RangeError: Array buffer allocation failed",
		over: `Uint16Array.from({length: 501}).byteLength`, fits: `Uint16Array.from({length: 500}).byteLength`},
	{name: "TypedArray from TypedArray", limits: bufferLimit, want: "RangeError: Array buffer allocation failed",
		over: `new Float64Array(new Uint8Array(126)).byteLength`, fits: `new Float64Array(new Uint8Array(125)).byteLength`},

	{name: "BigInt <<", limits: bigIntLimit, want: "RangeError: Maximum BigInt size exceeded",
		over: `String(1n << 1000n).length`, fits: `String(1n << 999n).length`},
	{name: "BigInt >> by a negative amount", limits: bigIntLimit, want: "RangeError: Maximum BigInt size exceeded",
		over: `String(1n >> -1000n).length`, fits: `String(1n >> -999n).length`},
	{name: "BigInt **", limits: bigIntLimit, want: "RangeError: Maximum BigInt size exceeded",
		over: `const n = 501n; String(2n ** n).length`, fits: `const n = 500n; String(2n ** n).length`},
	{name: "BigInt ** folded at compile time", limits: bigIntLimit, want: "RangeError: Maximum BigInt size exceeded",
		over: `String(2n ** 100000n).length`, fits: `String(2n ** 500n).length`},
	{name: "BigInt *", limits: bigIntLimit, want: "RangeError: Maximum BigInt size exceeded",
		over: `const a = 1n << 500n; String(a * a).length`, fits: `const a = 1n << 499n; String(a * a).length`},
	{name: "BigInt.asUintN", limits: bigIntLimit, want: "RangeError: Maximum BigInt size exceeded",
		over: `String(BigInt.asUintN(1001, -1n)).length`, fits: `String(BigInt.asUintN(1000, -1n)).length`},

	{name: "String of nested arrays", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: deepArray200 + `String(o).length`, fits: deepArrayOf(40) + `String(o).length`},
	{name: "template of nested arrays", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: deepArray200 + "`${o}`.length", fits: deepArrayOf(40) + "`${o}`.length"},
	{name: "loose equality with nested arrays", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: deepArray200 + `o == 1`, fits: deepArrayOf(40) + `o == 1`},
	{name: "join of nested arrays", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: deepArray200 + `o.join().length`, fits: deepArrayOf(40) + `o.join().length`},
	{name: "toLocaleString of nested arrays", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: deepArray200 + `o.toLocaleString().length`, fits: deepArrayOf(80) + `o.toLocaleString().length`},
	{name: "flat of nested arrays", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: deepArray200 + `o.flat(Infinity).length`, fits: deepArrayOf(90) + `o.flat(Infinity).length`},
	{name: "JSON.stringify of nested arrays", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: deepArray200 + `JSON.stringify(o).length`, fits: deepArrayOf(90) + `JSON.stringify(o).length`},
	{name: "JSON.parse of nested arrays", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: `JSON.parse("[".repeat(200) + "]".repeat(200)).length`, fits: `JSON.parse("[".repeat(90) + "]".repeat(90)).length`},
	// The reviver for the first element nests the second one, which the walk
	// has not reached yet, deeper than any parsed value can be.
	{name: "JSON.parse reviver", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: deepArray200 + `JSON.parse("[0,0]", function (k, v) { if (k === "0") this[1] = o; return v; }).length`,
		fits: deepArrayOf(40) + `JSON.parse("[0,0]", function (k, v) { if (k === "0") this[1] = o; return v; }).length`},
	{name: "Error toString chain", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: `let e = new Error("x"); for (let i = 0; i < 200; i++) { const n = new Error(); n.message = e; e = n; } String(e).length`,
		fits: `let e = new Error("x"); for (let i = 0; i < 40; i++) { const n = new Error(); n.message = e; e = n; } String(e).length`},
	{name: "bound function call chain", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: `let f = function () { return 1; }; for (let i = 0; i < 200; i++) f = f.bind(null); f()`,
		fits: `let f = function () { return 1; }; for (let i = 0; i < 90; i++) f = f.bind(null); f()`},
	{name: "bound function construct chain", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: `let C = function () {}; for (let i = 0; i < 200; i++) C = C.bind(null); typeof new C()`,
		fits: `let C = function () {}; for (let i = 0; i < 90; i++) C = C.bind(null); typeof new C()`},
	{name: "bound function instanceof chain", limits: depthLimit, want: "RangeError: Maximum nesting depth exceeded",
		over: `let F = function () {}; for (let i = 0; i < 200; i++) { F = Function.prototype.bind.call(F, null); Object.setPrototypeOf(F, null); } ({}) instanceof F`,
		fits: `let F = function () {}; for (let i = 0; i < 90; i++) { F = Function.prototype.bind.call(F, null); Object.setPrototypeOf(F, null); } ({}) instanceof F`},
	{name: "Object.create chain", limits: depthLimit, want: "RangeError: Maximum prototype chain length exceeded",
		over: `let p = {}; for (let i = 0; i < 200; i++) p = Object.create(p); p.missing`,
		fits: `let p = {}; for (let i = 0; i < 90; i++) p = Object.create(p); p.missing`},
	{name: "setPrototypeOf joins two chains", limits: depthLimit, want: "RangeError: Maximum prototype chain length exceeded",
		over: `function chain(n) { const tail = {}; let p = tail; for (let i = 0; i < n; i++) p = Object.create(p); return [p, tail]; }
			const [, aTail] = chain(60), [b] = chain(60); Object.setPrototypeOf(aTail, b); 1`,
		fits: `function chain(n) { const tail = {}; let p = tail; for (let i = 0; i < n; i++) p = Object.create(p); return [p, tail]; }
			const [, aTail] = chain(40), [b] = chain(40); Object.setPrototypeOf(aTail, b); 1`},
	{name: "class extends chain", limits: depthLimit, want: "RangeError: Maximum prototype chain length exceeded",
		over: `let C = class {}; for (let i = 0; i < 200; i++) C = class extends C {}; typeof new C()`,
		fits: `let C = class {}; for (let i = 0; i < 90; i++) C = class extends C {}; typeof new C()`},
	{name: "constructor prototype chain", limits: depthLimit, want: "RangeError: Maximum prototype chain length exceeded",
		over: `let p = {}; for (let i = 0; i < 200; i++) { function F() {} F.prototype = p; p = new F(); } p.missing`,
		fits: `let p = {}; for (let i = 0; i < 90; i++) { function F() {} F.prototype = p; p = new F(); } p.missing`},
	{name: "Reflect.construct prototype chain", limits: depthLimit, want: "RangeError: Maximum prototype chain length exceeded",
		over: `let p = {}; for (let i = 0; i < 200; i++) { function F() {} F.prototype = p; p = Reflect.construct(Object, [], F); } p.missing`,
		fits: `let p = {}; for (let i = 0; i < 90; i++) { function F() {} F.prototype = p; p = Reflect.construct(Object, [], F); } p.missing`},
	{name: "Proxy target chain", limits: depthLimit, want: "RangeError: Maximum prototype chain length exceeded",
		over: `let p = {}; for (let i = 0; i < 200; i++) p = new Proxy(p, {}); p.missing`,
		fits: `let p = {}; for (let i = 0; i < 90; i++) p = new Proxy(p, {}); p.missing`},
	{name: "eval of nested source", limits: depthLimit, want: "SyntaxError",
		over: `eval("[".repeat(200) + "]".repeat(200)).length`, fits: `eval("[".repeat(20) + "]".repeat(20)).length`},
	{name: "eval of a long operator chain", limits: depthLimit, want: "SyntaxError",
		over: `eval("0" + "+1".repeat(200))`, fits: `eval("0" + "+1".repeat(20))`},
	{name: "eval of a long member chain", limits: depthLimit, want: "SyntaxError",
		over: `const o = {}; o.o = o; eval("o" + ".o".repeat(200)) === o`, fits: `const o = {}; o.o = o; eval("o" + ".o".repeat(20)) === o`},
	{name: "eval of nested unary operators", limits: depthLimit, want: "SyntaxError",
		over: `eval("- ".repeat(200) + "1")`, fits: `eval("- ".repeat(20) + "1")`},
	{name: "eval of nested exponents", limits: depthLimit, want: "SyntaxError",
		over: `eval("1" + "**1".repeat(200))`, fits: `eval("1" + "**1".repeat(20))`},
	{name: "eval of nested new", limits: depthLimit, want: "SyntaxError",
		over: `function C() { return C; } typeof eval("new ".repeat(200) + "C")`,
		fits: `function C() { return C; } typeof eval("new ".repeat(20) + "C")`},
	{name: "eval of nested blocks", limits: depthLimit, want: "SyntaxError",
		over: `eval("{".repeat(200) + "1" + "}".repeat(200))`, fits: `eval("{".repeat(20) + "1" + "}".repeat(20))`},
	{name: "Function constructor of nested source", limits: depthLimit, want: "SyntaxError",
		over: `new Function("return " + "(".repeat(200) + "1" + ")".repeat(200))()`,
		fits: `new Function("return " + "(".repeat(20) + "1" + ")".repeat(20))()`},
}

func deepArrayOf(n int) string {
	return strings.Replace(deepArray200, "200", strconv.Itoa(n), 1)
}

// runCaught runs script in a fresh Runtime and reports its completion value,
// or the name and message of the exception it threw and false.
func runCaught(t *testing.T, limits Limits, script string) (string, bool) {
	t.Helper()
	r := New()
	r.SetLimits(limits)
	v, err := r.RunString("try {\n" + script + "\n} catch (e) { 'threw ' + e.name + ': ' + e.message }")
	if err != nil {
		t.Fatalf("RunString: %v", err)
	}
	s := v.String()
	if rest, ok := strings.CutPrefix(s, "threw "); ok {
		return rest, false
	}
	return s, true
}

func TestLimits(t *testing.T) {
	for _, c := range limitCases {
		t.Run(c.name, func(t *testing.T) {
			if got, ok := runCaught(t, c.limits, c.over); ok || !strings.HasPrefix(got, c.want) {
				t.Errorf("over the limit: got %q, want %s", got, c.want)
			}
			if got, ok := runCaught(t, c.limits, c.fits); !ok {
				t.Errorf("within the limit: threw %s", got)
			}
			if got, ok := runCaught(t, Limits{}, c.over); !ok {
				t.Errorf("unlimited: threw %s", got)
			}
		})
	}
}

func TestLimitsDefaultIsUnlimited(t *testing.T) {
	var l Limits
	if New().limits != l {
		t.Fatal("a new Runtime has limits")
	}
}

// A [[SetPrototypeOf]] stops looking for a cycle at the first Proxy, so the
// spec allows this one. Every property lookup on p then recurses forever in Go,
// which is fatal to the process, so with MaxNestingDepth set it is refused.
func TestLimitsProxyPrototypeCycle(t *testing.T) {
	got, ok := runCaught(t, depthLimit, `const t = {}; const p = new Proxy(t, {}); Object.setPrototypeOf(t, p); p.missing`)
	if ok || !strings.HasPrefix(got, "TypeError: Cyclic __proto__ value") {
		t.Errorf("Object.setPrototypeOf: got %q", got)
	}
	got, ok = runCaught(t, depthLimit, `const t = {}; const p = new Proxy(t, {}); Reflect.setPrototypeOf(t, p)`)
	if !ok || got != "false" {
		t.Errorf("Reflect.setPrototypeOf: got %q", got)
	}
	// An ordinary prototype change is not mistaken for a cycle.
	got, ok = runCaught(t, depthLimit, `const a = {}, b = Object.create(a); Object.setPrototypeOf(a, Object.create(null)); Object.getPrototypeOf(b) === a`)
	if !ok || got != "true" {
		t.Errorf("ordinary setPrototypeOf: got %q", got)
	}
}

// A BigInt that does not need to change is returned as it is, however many
// bits are asked for (here far more than MaxBigIntBits): the old code
// computed 2**bits first.
func TestLimitsBigIntAsNFastPath(t *testing.T) {
	got, ok := runCaught(t, bigIntLimit, `BigInt.asUintN(100000, 5n) === 5n && BigInt.asIntN(100000, -5n) === -5n && BigInt.asIntN(8, 127n) === 127n && BigInt.asIntN(8, -128n) === -128n && BigInt.asIntN(8, 128n) === -128n && BigInt.asUintN(8, 256n) === 0n && 1n ** (1n << 900n) === 1n && 0n << 5000n === 0n`)
	if !ok || got != "true" {
		t.Errorf("got %q", got)
	}
}

// RunString itself refuses source nested too deeply, before running any of it.
func TestLimitsRunStringNesting(t *testing.T) {
	r := New()
	r.SetLimits(depthLimit)
	_, err := r.RunString(strings.Repeat("[", 200) + strings.Repeat("]", 200))
	var ex *Exception
	if !errors.As(err, &ex) || !strings.Contains(err.Error(), "SyntaxError") || !strings.Contains(err.Error(), "Maximum nesting depth exceeded") {
		t.Fatalf("got %v", err)
	}
	if _, err = r.RunString(strings.Repeat("[", 50) + strings.Repeat("]", 50)); err != nil {
		t.Fatal(err)
	}
}

// After an operation throws deep inside nested native calls, the runtime is
// back at depth zero and works as before.
func TestLimitsNestingUnwinds(t *testing.T) {
	r := New()
	r.SetLimits(depthLimit)
	for i := 0; i < 3; i++ {
		v, err := r.RunString("{" + deepArray200 + `let n = 0; for (let i = 0; i < 50; i++) { try { String(o); } catch (e) { n++; } } n + ":" + String([[1], [2]]) }`)
		if err != nil {
			t.Fatal(err)
		}
		if v.String() != "50:1,2" {
			t.Fatalf("got %s", v)
		}
		if r.nestingDepth != 0 {
			t.Fatalf("nesting depth %d after the script", r.nestingDepth)
		}
	}
}

// SetMaxCallStackSize already bounds recursion through JavaScript frames,
// including recursion that passes through native code and accessors. Its
// error cannot be caught by the script.
func TestMaxCallStackSizeCoversJSRecursion(t *testing.T) {
	for _, script := range []string{
		`function f() { return f(); } try { f(); } catch (e) { "caught" }`,
		`function g() { return [0].map(g); } try { g(); } catch (e) { "caught" }`,
		`const o = { get x() { return this.x; } }; try { o.x; } catch (e) { "caught" }`,
		`const o = { toString() { return String(this); } }; try { String(o); } catch (e) { "caught" }`,
		`async function a() { return await a(); } a(); "returned"`,
	} {
		r := New()
		r.SetMaxCallStackSize(1000)
		_, err := r.RunString(script)
		var so *StackOverflowError
		if !errors.As(err, &so) {
			t.Errorf("%s: got %v, want a *StackOverflowError", script, err)
		}
	}
}
