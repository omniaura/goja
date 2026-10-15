package parser

import (
	"strings"
	"testing"
)

func TestWithMaxNestingDepth(t *testing.T) {
	nested := []struct {
		name               string
		open, inner, close string
	}{
		{"arrays", "[", "", "]"},
		{"parentheses", "(", "1", ")"},
		{"objects", "({a:", "1", "})"},
		{"unary operators", "- ", "1", ""},
		{"exponents", "1 ** (", "1", ")"},
		{"right-associative exponents", "1 ** ", "1", ""},
		{"new", "new ", "C", ""},
		{"conditionals", "1 ? 1 : ", "1", ""},
		{"arrow functions", "a => ", "1", ""},
		{"template literals", "`${", "1", "}`"},
		{"blocks", "{", "", "}"},
		{"if statements", "if (1) ", ";", ""},
		{"functions", "function f() {", "", "}"},
	}
	src := func(n int, open, inner, close string) string {
		return strings.Repeat(open, n) + inner + strings.Repeat(close, n)
	}
	for _, c := range nested {
		t.Run(c.name, func(t *testing.T) {
			deep := src(200, c.open, c.inner, c.close)
			if _, err := ParseFile(nil, "", deep, 0); err != nil {
				t.Fatalf("unlimited: %v", err)
			}
			_, err := ParseFile(nil, "", deep, 0, WithMaxNestingDepth(100))
			if err == nil || !strings.Contains(err.Error(), "Maximum nesting depth exceeded") {
				t.Errorf("over the limit: got %v", err)
			}
			// Some repetitions nest two levels: ({a: is a parenthesis and an object.
			if _, err := ParseFile(nil, "", src(40, c.open, c.inner, c.close), 0, WithMaxNestingDepth(100)); err != nil {
				t.Errorf("within the limit: %v", err)
			}
		})
	}
}

// Ordinary code does not come near a modest limit.
func TestWithMaxNestingDepthOrdinaryCode(t *testing.T) {
	const src = `
	class A extends Object {
		#x = 1;
		static *gen(a = {b: [1, {c: [2, 3]}]}, ...rest) {
			for (const v of rest) {
				try {
					if (v?.x ?? false) yield* [v.x ** 2, -(+v.y)];
					else switch (typeof v) { case "number": yield v; break; default: throw new Error(` + "`bad ${v}`" + `); }
				} catch ({message}) { console.log(message); } finally { label: { break label; } }
			}
		}
	}
	const f = (a) => (b) => (c) => a + b * c - (a ? b : c ? a : b);
	async function g(x) { return await f(1)(2)(await x); }
	`
	if _, err := ParseFile(nil, "", src, 0, WithMaxNestingDepth(32)); err != nil {
		t.Fatal(err)
	}
}
