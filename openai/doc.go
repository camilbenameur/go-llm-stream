// Package openai provides an OpenAI-compatible streaming adapter for go-llm-stream.
// It wraps the SSE decoder and JSON healer to provide a familiar interface for
// users of go-openai or similar libraries.
//
// Example usage:
//
//	resp, _ := http.Get("https://api.openai.com/v1/chat/completions")
//	stream := openai.NewStream(ctx, resp.Body)
//	for {
//	    delta, err := stream.NextDelta()
//	    if err == io.EOF {
//	        break
//	    }
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    fmt.Print(delta)
//	}
package openai
