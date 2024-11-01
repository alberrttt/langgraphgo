package graph_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/alberrttt/langgraphgo/graph"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

func ExampleMessageGraph() {
	model, err := openai.New()
	if err != nil {
		panic(err)
	}

	g := graph.NewStateGraph[graph.MessageState]()

	g.AddNode("oracle", func(ctx context.Context, state *graph.MessageState) error {
		r, err := model.GenerateContent(ctx, state.Messages, llms.WithTemperature(0.0))
		if err != nil {
			return err
		}
		state.Messages = append(state.Messages,
			llms.TextParts(llms.ChatMessageTypeAI, r.Choices[0].Content),
		)
		return nil
	})
	g.AddNode(graph.END, func(_ context.Context, state *graph.MessageState) error {
		return nil
	})

	g.AddEdge("oracle", graph.END)
	g.SetEntryPoint("oracle")

	runnable, err := g.Compile()
	if err != nil {
		panic(err)
	}

	ctx := context.Background()
	// Let's run it!
	msgs := graph.NewMessageState()
	err = runnable.Invoke(ctx, &msgs)
	if err != nil {
		panic(err)
	}

	fmt.Println(msgs)

	// Output:
	// [{human [{What is 1 + 1?}]} {ai [{1 + 1 equals 2.}]}]
}

func TestMessageGraph(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name           string
		buildGraph     func() *graph.StateGraph[graph.MessageState]
		inputMessages  []llms.MessageContent
		expectedOutput []llms.MessageContent
		expectedError  error
	}{
		{
			name: "Simple graph",
			buildGraph: func() *graph.StateGraph[graph.MessageState] {
				g := graph.NewStateGraph[graph.MessageState]()
				g.AddNode("node1", func(_ context.Context, state *graph.MessageState) error {
					state.Messages = append(state.Messages, llms.TextParts(llms.ChatMessageTypeAI, "Node 1"))
					return nil
				})
				g.AddNode("node2", func(_ context.Context, state *graph.MessageState) error {
					state.Messages = append(state.Messages, llms.TextParts(llms.ChatMessageTypeAI, "Node 2"))
					return nil
				})
				g.AddEdge("node1", "node2")
				g.AddEdge("node2", graph.END)
				g.SetEntryPoint("node1")
				return g
			},
			inputMessages: []llms.MessageContent{llms.TextParts(llms.ChatMessageTypeHuman, "Input")},
			expectedOutput: []llms.MessageContent{
				llms.TextParts(llms.ChatMessageTypeHuman, "Input"),
				llms.TextParts(llms.ChatMessageTypeAI, "Node 1"),
				llms.TextParts(llms.ChatMessageTypeAI, "Node 2"),
			},
			expectedError: nil,
		},
		{
			name: "Entry point not set",
			buildGraph: func() *graph.StateGraph[graph.MessageState] {
				g := graph.NewStateGraph[graph.MessageState]()
				g.AddNode("node1", func(_ context.Context, state *graph.MessageState) error {
					return nil
				})
				return g
			},
			expectedError: graph.ErrEntryPointNotSet,
		},
		{
			name: "Node not found",
			buildGraph: func() *graph.StateGraph[graph.MessageState] {
				g := graph.NewStateGraph[graph.MessageState]()
				g.AddNode("node1", func(_ context.Context, state *graph.MessageState) error {
					return nil
				})
				g.AddEdge("node1", "node2")
				g.SetEntryPoint("node1")
				return g
			},
			expectedError: fmt.Errorf("%w: node2", graph.ErrNodeNotFound),
		},
		{
			name: "No outgoing edge",
			buildGraph: func() *graph.StateGraph[graph.MessageState] {
				g := graph.NewStateGraph[graph.MessageState]()
				g.AddNode("node1", func(_ context.Context, state *graph.MessageState) error {
					return nil
				})
				g.SetEntryPoint("node1")
				return g
			},
			expectedError: fmt.Errorf("%w: node1", graph.ErrNoOutgoingEdge),
		},
		{
			name: "Error in node function",
			buildGraph: func() *graph.StateGraph[graph.MessageState] {
				g := graph.NewStateGraph[graph.MessageState]()
				g.AddNode("node1", func(_ context.Context, _ *graph.MessageState) error {
					return errors.New("node error")
				})
				g.AddEdge("node1", graph.END)
				g.SetEntryPoint("node1")
				return g
			},
			expectedError: errors.New("error in node node1: node error"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := tc.buildGraph()
			runnable, err := g.Compile()
			if err != nil {
				if tc.expectedError == nil || !errors.Is(err, tc.expectedError) {
					t.Fatalf("unexpected compile error: %v", err)
				}
				return
			}

			output := &graph.MessageState{Messages: tc.inputMessages}
			err = runnable.Invoke(context.Background(), output)
			if err != nil {
				if tc.expectedError == nil || err.Error() != tc.expectedError.Error() {
					t.Fatalf("unexpected invoke error: '%v', expected '%v'", err, tc.expectedError)
				}
				return
			}

			if tc.expectedError != nil {
				t.Fatalf("expected error %v, but got nil", tc.expectedError)
			}

			if len(output.Messages) != len(tc.expectedOutput) {
				t.Fatalf("expected output length %d, but got %d", len(tc.expectedOutput), len(output.Messages))
			}

			for i, msg := range output.Messages {
				got := fmt.Sprint(msg)
				expected := fmt.Sprint(tc.expectedOutput[i])
				if got != expected {
					t.Errorf("expected output[%d] content %q, but got %q", i, expected, got)
				}
			}
		})
	}
}
