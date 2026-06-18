package scanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
)

// generateLargeJSON builds a JSON object of roughly targetSize bytes,
// shaped like a typical "structured output" / tool-call arguments payload:
// a single large object containing an array of records.
func generateLargeJSON(targetSize int) []byte {
	var buf bytes.Buffer
	buf.WriteString(`{"type":"result","items":[`)
	i := 0
	for buf.Len() < targetSize {
		if i > 0 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(&buf, `{"id":%d,"name":"item-%d","description":"This is a longer description field used to pad out the payload to a realistic size.","value":%d.%d,"active":%t}`,
			i, i, i, i%100, i%2 == 0)
		i++
	}
	buf.WriteString(`],"count":`)
	fmt.Fprintf(&buf, "%d}", i)
	return buf.Bytes()
}

// chunks splits data into chunks of the given size.
func chunks(data []byte, chunkSize int) [][]byte {
	var out [][]byte
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		out = append(out, data[i:end])
	}
	return out
}

// chunkReader is an io.Reader that yields data in fixed-size chunks,
// one Read call per chunk, simulating a chunked network stream.
type chunkReader struct {
	chunks [][]byte
	idx    int
}

func newChunkReader(data []byte, chunkSize int) *chunkReader {
	return &chunkReader{chunks: chunks(data, chunkSize)}
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.idx >= len(r.chunks) {
		return 0, io.EOF
	}
	c := r.chunks[r.idx]
	n := copy(p, c)
	if n < len(c) {
		// Shouldn't happen with our buffer sizes, but guard anyway.
		r.chunks[r.idx] = c[n:]
	} else {
		r.idx++
	}
	return n, nil
}

// --- Benchmark scenarios -------------------------------------------------
//
// All three approaches receive the SAME large JSON value delivered in the
// SAME size chunks. They differ only in how they process the growing data:
//
//   - Naive: re-parse the entire accumulated buffer on every chunk
//     (the anti-pattern described in the README).
//   - StdlibDecoder: encoding/json.Decoder reading tokens from a chunked
//     reader (a fair "streaming stdlib" baseline).
//   - Scanner: go-llm-stream's StreamReader, processing each byte once.

const (
	smallSize = 4 * 1024  // 4 KB
	largeSize = 16 * 1024 // 16 KB
	hugeSize  = 64 * 1024 // 64 KB
)

var chunkSizesToTest = []int{32, 64}

func BenchmarkCompareNaiveReparse(b *testing.B) {
	for _, totalSize := range []int{smallSize, largeSize, hugeSize} {
		data := generateLargeJSON(totalSize)
		for _, chunkSize := range chunkSizesToTest {
			parts := chunks(data, chunkSize)
			name := fmt.Sprintf("size=%d/chunk=%d", len(data), chunkSize)
			b.Run(name, func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					var buf bytes.Buffer
					buf.Grow(len(data))
					for _, p := range parts {
						buf.Write(p)
						// Anti-pattern: re-parse (validate) the WHOLE
						// accumulated buffer on every chunk.
						_ = json.Valid(buf.Bytes())
					}
					// Final full unmarshal, as a real consumer would do
					// once the value is complete.
					var v any
					_ = json.Unmarshal(buf.Bytes(), &v)
				}
			})
		}
	}
}

func BenchmarkCompareStdlibDecoder(b *testing.B) {
	for _, totalSize := range []int{smallSize, largeSize, hugeSize} {
		data := generateLargeJSON(totalSize)
		for _, chunkSize := range chunkSizesToTest {
			name := fmt.Sprintf("size=%d/chunk=%d", len(data), chunkSize)
			b.Run(name, func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					r := newChunkReader(data, chunkSize)
					dec := json.NewDecoder(r)
					for {
						_, err := dec.Token()
						if err != nil {
							break
						}
					}
				}
			})
		}
	}
}

func BenchmarkCompareStreamReaderScanner(b *testing.B) {
	for _, totalSize := range []int{smallSize, largeSize, hugeSize} {
		data := generateLargeJSON(totalSize)
		for _, chunkSize := range chunkSizesToTest {
			name := fmt.Sprintf("size=%d/chunk=%d", len(data), chunkSize)
			b.Run(name, func(b *testing.B) {
				b.SetBytes(int64(len(data)))
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					r := newChunkReader(data, chunkSize)
					sr := NewStreamReader(context.Background(), r)
					for {
						token := sr.NextToken()
						if token.Kind == TokenEOF || token.Kind == TokenError {
							break
						}
					}
					sr.Close()
				}
			})
		}
	}
}
