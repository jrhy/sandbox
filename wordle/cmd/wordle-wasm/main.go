//go:build js && wasm

// Command wordle-wasm exposes the Wordle solver to JavaScript.
package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"

	"github.com/jrhy/sandbox/wordle/core"
)

type request struct {
	Guesses []core.Guess `json:"guesses"`
}

type response struct {
	Candidates []core.Candidate `json:"candidates"`
	Error      string             `json:"error,omitempty"`
}

func solve(this js.Value, args []js.Value) any {
	if len(args) != 1 {
		return encode(response{Error: "solve expects one JSON argument"})
	}
	var req request
	if err := json.Unmarshal([]byte(args[0].String()), &req); err != nil {
		return encode(response{Error: fmt.Sprintf("invalid request: %v", err)})
	}
	if err := core.NormalizeGuesses(req.Guesses); err != nil {
		return encode(response{Error: err.Error()})
	}
	candidates, err := core.GetCandidates(req.Guesses)
	if err != nil {
		return encode(response{Error: err.Error()})
	}
	return encode(response{Candidates: candidates})
}

func encode(v response) js.Value {
	b, err := json.Marshal(v)
	if err != nil {
		return js.ValueOf(`{"error":"could not encode response"}`)
	}
	return js.ValueOf(string(b))
}

func main() {
	js.Global().Set("wordleSolve", js.FuncOf(solve))
	select {}
}
