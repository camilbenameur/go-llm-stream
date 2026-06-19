package stream_test

import (
	"fmt"

	"github.com/camilbenameur/go-llm-stream/stream"
)

// ExampleWriter demonstrates the push-based API: write chunks as they arrive
// (split at arbitrary byte boundaries) and receive completed tokens via OnToken.
func ExampleWriter() {
	w := stream.NewWriter()
	w.OnToken = func(t stream.Token) {
		if t.Kind == stream.TokenString && !t.IsKey {
			fmt.Printf("value: %s\n", t.Raw)
		}
	}

	// The literal "hello" is split across two Write calls; it is not emitted
	// until it completes.
	_, _ = w.Write([]byte(`{"greeting":"hel`))
	_, _ = w.Write([]byte(`lo","ok":true}`))
	_ = w.Close()

	// Output:
	// value: "hello"
}
