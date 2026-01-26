// Package sse provides a Server-Sent Events (SSE) decoder for streaming
// text/event-stream responses from LLM APIs like OpenAI and Anthropic.
//
// The decoder handles partial frames across reads, ignores comment lines and
// keep-alives, and supports multi-line data payloads.
//
// Example usage:
//
//	resp, _ := http.Get("https://api.openai.com/v1/chat/completions")
//	decoder := sse.NewDecoder(resp.Body)
//	for {
//	    event, err := decoder.Next()
//	    if err == io.EOF {
//	        break
//	    }
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    fmt.Println("Data:", event.Data)
//	}
package sse
