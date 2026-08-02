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
package handlers

import (
	"strings"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/scenarios"
)

// flattenToolCallChain walks a NextToolCall linked list starting at head and
// returns it as an ordered slice. This lifts the historical 2-tool-call cap
// (issue #1853): scenarios like investigate -> discover_workflows ->
// select_workflow -> watch can now be scripted as a single chain instead of
// requiring N independently-keyword-triggered turns.
func flattenToolCallChain(head *scenarios.MultiToolCallEntry) []*scenarios.MultiToolCallEntry {
	var chain []*scenarios.MultiToolCallEntry
	for node := head; node != nil; node = node.NextToolCall {
		chain = append(chain, node)
	}
	return chain
}

// nextChainCallByCount returns the chain entry due to fire given the total
// number of tool/function responses already present in the conversation
// (priorResponseCount), or nil if the chain hasn't started yet or has fully
// completed. priorResponseCount==1 fires chain[0] (the same trigger point as
// the original single-NextToolCall behavior), priorResponseCount==2 fires
// chain[1], and so on; priorResponseCount > len(chain) means the whole chain
// has already fired, so the caller should fall through to its normal
// DAG/text response path (mirrors the pre-#1853 "fire at most once" guard,
// generalized to N links).
func nextChainCallByCount(chain []*scenarios.MultiToolCallEntry, priorResponseCount int) *scenarios.MultiToolCallEntry {
	if priorResponseCount < 1 || priorResponseCount > len(chain) {
		return nil
	}
	return chain[priorResponseCount-1]
}

// resolveTemplateArgsMap resolves "$from_tool:<toolName>:<field>" placeholders
// in args using extractFn to look up each referenced tool's prior response
// field, returning a NEW map — args is never mutated in place. This avoids
// data races and cross-request state leaks when args is a field on a shared
// scenario singleton (e.g. cfg.ToolCallArgs or a NextToolCall chain node's
// Arguments, both of which are read concurrently by other in-flight
// requests matching the same scenario).
//
// If at least one placeholder cannot be resolved (extractFn returns "" —
// the referenced tool was never called earlier in this conversation) AND
// fallback is non-empty, the ENTIRE argument set is replaced with a clone
// of fallback instead of returning a partially-resolved map. This mirrors
// tool schemas with mutually exclusive argument sets — e.g.
// kubernaut_investigate accepts EITHER rr_id (continue/take over an
// existing session) OR namespace/kind/name (create a new RR) — so a
// scenario scripted for the "continue" case degrades gracefully to the
// "create new" case instead of sending a half-resolved/literal template
// string as a real argument value (issue #1853; found live on a shared FP
// cluster where af_investigate was invoked with no prior kubernaut_remediate
// call in the session).
func resolveTemplateArgsMap(args, fallback map[string]interface{}, extractFn func(toolName, field string) string) map[string]interface{} {
	if len(args) == 0 {
		return args
	}
	resolved := cloneAnyMap(args)
	anyUnresolved := false
	for k, v := range resolved {
		sv, ok := v.(string)
		if !ok || !strings.HasPrefix(sv, templatePrefix) {
			continue
		}
		parts := strings.SplitN(sv[len(templatePrefix):], ":", 2)
		if len(parts) != 2 {
			continue
		}
		toolName, field := parts[0], parts[1]
		if resolvedVal := extractFn(toolName, field); resolvedVal != "" {
			resolved[k] = resolvedVal
		} else {
			anyUnresolved = true
		}
	}
	if anyUnresolved && len(fallback) > 0 {
		return cloneAnyMap(fallback)
	}
	return resolved
}
