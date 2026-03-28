/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package conversation

import "fmt"

// TransitionCondition evaluates whether a transition should be taken.
type TransitionCondition interface {
	Evaluate(ctx *Context) bool
}

// ResponseHandler produces a response for a DAG node.
type ResponseHandler interface {
	Handle(ctx *Context) (*HandlerResult, error)
}

// HandlerResult holds the output from a node handler.
type HandlerResult struct {
	NodeName string
}

// Transition defines a conditional edge between DAG nodes.
type Transition struct {
	Target    string
	Condition TransitionCondition
	Priority  int
}

// DAGNode represents a state in the conversation DAG.
type DAGNode struct {
	Name        string
	Handler     ResponseHandler
	Transitions []Transition
}

// DAG is a directed acyclic graph representing a conversation flow.
type DAG struct {
	initialNode string
	nodes       map[string]*DAGNode
}

// ExecutionResult captures the outcome of a DAG execution.
type ExecutionResult struct {
	TerminalNode string
	Path         []string
}

// NewDAG creates a new DAG with the given initial node name.
func NewDAG(initial string) *DAG {
	return &DAG{
		initialNode: initial,
		nodes:       make(map[string]*DAGNode),
	}
}

// AddNode registers a node with a handler.
func (d *DAG) AddNode(name string, handler ResponseHandler) {
	d.nodes[name] = &DAGNode{
		Name:    name,
		Handler: handler,
	}
}

// AddTransition adds a conditional transition from one node to another.
func (d *DAG) AddTransition(from, to string, condition TransitionCondition, priority int) {
	node, ok := d.nodes[from]
	if !ok {
		return
	}
	node.Transitions = append(node.Transitions, Transition{
		Target:    to,
		Condition: condition,
		Priority:  priority,
	})
}

// Execute traverses the DAG from the initial node, following transitions
// whose conditions evaluate to true. Each execution uses its own path
// slice, ensuring concurrent executions are isolated.
func (d *DAG) Execute(ctx *Context) (*ExecutionResult, error) {
	var path []string
	current := d.initialNode

	for {
		node, ok := d.nodes[current]
		if !ok {
			return nil, fmt.Errorf("dag: node %q not found", current)
		}
		path = append(path, current)

		if len(node.Transitions) == 0 {
			return &ExecutionResult{TerminalNode: current, Path: path}, nil
		}

		next := ""
		for _, t := range node.Transitions {
			if t.Condition.Evaluate(ctx) {
				next = t.Target
				break
			}
		}

		if next == "" {
			return nil, fmt.Errorf("dag: no matching transition from node %q", current)
		}
		current = next
	}
}
