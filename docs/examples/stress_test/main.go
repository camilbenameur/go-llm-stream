package main

import (
	"context"
	"fmt"
	"io"
	"runtime"
	"time"

	"github.com/camilbenameur/go-llm-stream/stream"
)

func main() {
	fmt.Println("Starting Scenario A: Memory Stability Stress Test")

	const count = 100000

	pr, pw := io.Pipe()

	go func() {
		defer pw.Close()
		pw.Write([]byte("["))
		for i := 0; i < count; i++ {
			if i > 0 {
				pw.Write([]byte(","))
			}
			pw.Write([]byte(fmt.Sprintf(`{"id":%d,"data":"some string content that is long enough to exercise the scanner's buffer handling and pool reuse logic. %d"}`, i, i)))
		}
		pw.Write([]byte("]"))
	}()

	var memStart runtime.MemStats
	runtime.ReadMemStats(&memStart)

	start := time.Now()

	h := stream.NewHealer(context.Background(), pr)

	tokenCount := 0
	for {
		t := h.NextToken()
		if t.Kind == stream.TokenEOF {
			break
		}
		if t.Kind == stream.TokenError {
			panic(fmt.Sprintf("Token error: %v", t.Err))
		}
		tokenCount++
	}

	duration := time.Since(start)

	var memEnd runtime.MemStats
	runtime.ReadMemStats(&memEnd)

	fmt.Printf("Processed %d tokens in %v\n", tokenCount, duration)
	fmt.Printf("Total Allocations: %d KB\n", (memEnd.TotalAlloc-memStart.TotalAlloc)/1024)
	fmt.Printf("Heap In Use: %d KB\n", memEnd.HeapInuse/1024)
	fmt.Printf("Mallocs: %d\n", memEnd.Mallocs-memStart.Mallocs)
}
