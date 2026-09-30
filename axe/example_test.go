package axe_test

import (
	"context"
	"math"

	"github.com/3-lines-studio/goddard/axe"
)

func Example() {
	provider := axe.NewOpenAI("https://api.openai.com/v1", "sk-...")
	tools := axe.BuildTools("/path/to/project")
	options := &axe.RunOptions{
		Model:    "gpt-4.1-mini",
		System:   axe.SystemPrompt(tools),
		Tools:    tools,
		MaxTurns: math.MaxInt,
	}
	history := []axe.Message{{Role: "user", Content: "list the files"}}
	sink := &axe.SinkBase{}
	end := axe.RunStream(context.Background(), provider, options, history, sink)
	_ = end
}
