package graph

import (
	"context"
	"errors"
	"fmt"
)

// END is a special constant used to represent the end node in the graph.
const END = "END"

var (
	// ErrEntryPointNotSet is returned when the entry point of the graph is not set.
	ErrEntryPointNotSet = errors.New("entry point not set")

	// ErrNodeNotFound is returned when a node is not found in the graph.
	ErrNodeNotFound = errors.New("node not found")

	// ErrNoOutgoingEdge is returned when no outgoing edge is found for a node.
	ErrNoOutgoingEdge = errors.New("no outgoing edge found for node")
)

// Node represents a node in the message graph.
type Node[T any] struct {
	// Name is the unique identifier for the node.
	Name string

	// Function is the function associated with the node.
	// It takes a context and a slice of MessageContent as input and returns a slice of MessageContent and an error.
	Function func(ctx context.Context, state *T) error
}

// Edge represents an edge in the message graph.
type Edge[T any] interface {
	// From is the name of the node from which the edge originates.
	From() string

	// To is the name of the node to which the edge points.
	To(ctx context.Context, state *T) []string
}
type SimpleEdge[state any] struct {
	from string
	to   string
}

func (e *SimpleEdge[state]) From() string {
	return e.from
}
func (e *SimpleEdge[state]) To(ctx context.Context, _ *state) []string {
	return []string{e.to}
}

type Branch[state any] struct {
	Path    func(ctx context.Context, state *state) ([]string, error)
	Mapping func(x string) string
	Then    string
	Source  string
}

func (b *Branch[s]) From() string {
	return b.Source
}

func (b *Branch[s]) To(ctx context.Context, state *s) []string {
	paths, err := b.Path(ctx, state)
	if err != nil {
		return []string{}
	}
	n := []string{}
	for _, path := range paths {
		n = append(n, b.Mapping(path))
	}
	return append(n, b.Then)
}

type ConditionalEdgeOptions[T any] struct {
	Mapping func(x string) string
	Then    string
}

func WithMap[T any](pathMap map[string]string) ConditionalEdgeOptions[T] {
	return ConditionalEdgeOptions[T]{
		Mapping: func(x string) string {
			return pathMap[x]
		},
	}
}

func WithThen[T any](then string) ConditionalEdgeOptions[T] {
	return ConditionalEdgeOptions[T]{
		Then: then,
	}
}

// AddConditionalEdges adds a conditional edge from the starting node to any number of destination nodes.
// It allows for dynamic determination of the next nodes based on the provided path function.
//
// Parameters:
// - source (string): The starting node. This conditional edge will run when exiting this node.
// - path (Callable[T, []string]): The callable that determines the next node or nodes. If not specifying pathMap, it should return one or more nodes. If it returns "END", the graph will stop execution.
// - pathMap (map[string]string, optional): Optional mapping of paths to node names. If omitted, the paths returned by path should be node names.
// - then (string, optional): The name of a node to execute after the nodes selected by path.
//
// Returns:
// - *StateGraph[T]: The StateGraph instance to allow method chaining.
func (g *StateGraph[T]) AddConditionalEdges(
	source string,
	path func(ctx context.Context, state *T) ([]string, error),
	options ...ConditionalEdgeOptions[T],
) *StateGraph[T] {
	// Create a Branch edge with the provided parameters
	branch := &Branch[T]{
		Source: source,
		Path:   path,
		Mapping: func(x string) string {
			return x
		},
	}

	// If a pathMap is provided, set the Mapping function
	for _, option := range options {
		if option.Mapping != nil {
			branch.Mapping = option.Mapping
		}
		if option.Then != "" {
			branch.Then = option.Then
		}
	}

	// Add the Branch edge to the graph's edges
	g.edges = append(g.edges, branch)

	return g
}

// StateGraph represents a message graph.
type StateGraph[T any] struct {
	// nodes is a map of node names to their corresponding Node objects.
	nodes map[string]Node[T]

	// edges is a slice of Edge objects representing the connections between nodes.
	edges []Edge[T]

	// entryPoint is the name of the entry point node in the graph.
	entryPoint string
}

// NewStateGraph creates a new instance of StateGraph.
func NewStateGraph[T any]() *StateGraph[T] {
	return &StateGraph[T]{
		nodes: make(map[string]Node[T]),
	}
}

// AddNode adds a new node to the message graph with the given name and function.
func (g *StateGraph[T]) AddNode(name string, fn func(ctx context.Context, state *T) error) {
	g.nodes[name] = Node[T]{
		Name:     name,
		Function: fn,
	}
}

// AddEdge adds a new edge to the message graph between the "from" and "to" nodes.
func (g *StateGraph[T]) AddEdge(from, to string) {
	g.edges = append(g.edges, &SimpleEdge[T]{
		from: from,
		to:   to,
	})
}

// SetEntryPoint sets the entry point node name for the message graph.
func (g *StateGraph[T]) SetEntryPoint(name string) {
	g.entryPoint = name
}

// Runnable represents a compiled message graph that can be invoked.
type Runnable[T any] struct {
	// Graph is the underlying StateGraph object.
	Graph *StateGraph[T]
}

// Compile compiles the message graph and returns a Runnable instance.
// It returns an error if the entry point is not set.
func (g *StateGraph[T]) Compile() (*Runnable[T], error) {
	if g.entryPoint == "" {
		return nil, ErrEntryPointNotSet
	}

	return &Runnable[T]{
		Graph: g,
	}, nil
}

// Invoke executes the compiled message graph with the given input messages.
// It returns the resulting messages and an error if any occurs during the execution.
// Invoke executes the compiled message graph with the given input messages.
// It returns the resulting messages and an error if any occurs during the execution.
func (r *Runnable[T]) Invoke(ctx context.Context, state *T) error {
	nextNodes := []string{r.Graph.entryPoint}

	pop := func() string {
		if len(nextNodes) == 0 {
			return END
		}
		item := nextNodes[len(nextNodes)-1]
		nextNodes = nextNodes[:len(nextNodes)-1]
		return item
	}
	peek := func() string {
		if len(nextNodes) == 0 {
			return END
		}
		return nextNodes[len(nextNodes)-1]
	}

	for {

		currentNode := pop()
		if currentNode == END {
			break
		}
		if currentNode == "" {
			continue
		}
		node, ok := r.Graph.nodes[currentNode]
		if !ok {
			return fmt.Errorf("node '%s' not found: %w", currentNode, ErrNodeNotFound)
		}
		err := node.Function(ctx, state)
		if err != nil {
			return fmt.Errorf("error in node '%s': %w", currentNode, err)
		}

		foundNext := false
		// this mean's there's another node
		if peek() != END {
			foundNext = true
		}
		for _, edge := range r.Graph.edges {
			if foundNext {
				break
			}
			if edge.From() == currentNode {
				nextNodes = append(nextNodes, edge.To(ctx, state)...)
				foundNext = true
			}
		}

		if !foundNext {
			return fmt.Errorf("no outgoing edge found for node '%s': %w", currentNode, ErrNoOutgoingEdge)
		}
	}
	return nil
}
