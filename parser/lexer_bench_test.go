package parser

import (
	"os"
	"strings"
	"testing"
)

// benchmarkSource resembles model-written tool scripts: long string and
// template literals (document text, queries) around a little code.
func benchmarkSource() string {
	var b strings.Builder
	b.WriteString("const out = [];\n")
	for i := 0; i < 40; i++ {
		b.WriteString(`out.push(tools.replace_all_text({document_id: "1_abcdefghijklmnopqrstuvwxyz0123456789", find_text: "Everything else has been settled between the parties as of the closing date", replace_text: 'The parties agree that the remaining obligations survive termination and are governed by section 12.3 of the agreement'}));` + "\n")
		b.WriteString("const note" + strings.Repeat("x", i%5) + "_" + string(rune('a'+i%26)) + " = `Summary for item ${out.length}: the reviewer confirmed the change and asked for a follow-up on the open questions.\\n`;\n")
	}
	b.WriteString("return JSON.stringify(out).slice(0, 4000);\n")
	return b.String()
}

func BenchmarkParseToolScript(b *testing.B) {
	src := "(async function(){\n" + benchmarkSource() + "\n})"
	if path := os.Getenv("GOJA_PARSE_BENCH_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		src = "(async function(){\n" + string(data) + "\n})"
	}
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := ParseFile(nil, "bench.js", src, 0, WithDisableSourceMaps); err != nil {
			b.Fatal(err)
		}
	}
}
