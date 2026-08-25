// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"sync"
	"testing"
)

func TestGrammarFor(t *testing.T) {
	t.Parallel()

	t.Run("tsx files select the TSX grammar", func(t *testing.T) {
		t.Parallel()
		if got := grammarFor("component.tsx"); got != grammarTSX {
			t.Fatalf("grammarFor(.tsx) = %v, want grammarTSX", got)
		}
	})

	t.Run("ts files select the TypeScript grammar", func(t *testing.T) {
		t.Parallel()
		for _, path := range []string{"a.ts", "b.mts", "c.cts", "d.d.ts"} {
			if got := grammarFor(path); got != grammarTS {
				t.Fatalf("grammarFor(%q) = %v, want grammarTS", path, got)
			}
		}
	})
}

func TestParseFile(t *testing.T) {
	t.Parallel()

	t.Run("parses a declaration without error", func(t *testing.T) {
		t.Parallel()
		p, err := parseFile("user.ts", []byte("export interface User { id: string; }"))
		if err != nil {
			t.Fatalf("parseFile: %v", err)
		}
		defer p.close()

		if p.root().HasError() {
			t.Fatalf("well-formed source reported a parse error: %s", p.root().ToSexp())
		}
	})

	t.Run("recovers from malformed source rather than failing", func(t *testing.T) {
		t.Parallel()
		// tree-sitter's contract is error recovery: it returns a tree
		// with error nodes rather than refusing. The frontend relies
		// on that to report a positioned diagnostic and keep going,
		// so a change to it would be a change to the frontend.
		p, err := parseFile("broken.ts", []byte("export interface { id: "))
		if err != nil {
			t.Fatalf("parseFile returned an error for recoverable input: %v", err)
		}
		defer p.close()

		if !p.root().HasError() {
			t.Fatal("malformed source parsed clean; expected error nodes")
		}
	})

	t.Run("the two grammars disagree on angle-bracket assertions", func(t *testing.T) {
		t.Parallel()
		// The reason both grammars ship. `<T>x` is a type assertion in
		// .ts and an unclosed JSX element in .tsx. If this ever stops
		// holding, grammarFor has no job.
		const src = "const a = <T>b;"

		asTS, err := parseFile("a.ts", []byte(src))
		if err != nil {
			t.Fatalf("parseFile(.ts): %v", err)
		}
		defer asTS.close()

		asTSX, err := parseFile("a.tsx", []byte(src))
		if err != nil {
			t.Fatalf("parseFile(.tsx): %v", err)
		}
		defer asTSX.close()

		if asTS.root().HasError() {
			t.Error("type assertion did not parse under the .ts grammar")
		}
		if !asTSX.root().HasError() {
			t.Error("type assertion parsed clean under the .tsx grammar; grammars no longer differ")
		}
	})

	t.Run("text reads a node's source and tolerates nil", func(t *testing.T) {
		t.Parallel()
		p, err := parseFile("t.ts", []byte("type A = B;"))
		if err != nil {
			t.Fatalf("parseFile: %v", err)
		}
		defer p.close()

		if got := p.text(p.root().NamedChild(0)); got != "type A = B;" {
			t.Fatalf("text = %q, want the whole declaration", got)
		}
		if got := p.text(nil); got != "" {
			t.Fatalf("text(nil) = %q, want empty", got)
		}
	})

	t.Run("close is safe on a nil parsed", func(t *testing.T) {
		t.Parallel()
		var p *parsed
		p.close()
	})

	t.Run("parsers survive concurrent use", func(t *testing.T) {
		t.Parallel()
		// The pipeline dispatches Load per pattern across goroutines
		// and a tree-sitter parser is not safe for concurrent use, so
		// the pool is what makes the frontend's declared concurrency
		// contract true. Run under -race to mean anything.
		var wg sync.WaitGroup
		for i := range 32 {
			wg.Go(func() {
				path := "a.ts"
				if i%2 == 0 {
					path = "a.tsx"
				}
				p, err := parseFile(path, []byte("export class C { x: number = 1; }"))
				if err != nil {
					t.Errorf("parseFile: %v", err)
					return
				}
				defer p.close()
				if p.root().HasError() {
					t.Error("concurrent parse produced error nodes")
				}
			})
		}
		wg.Wait()
	})
}
