---
name: hoyan-implementation-summary
description: Explain completed implementations in hoyan repository folders with concrete network-behavior examples instead of terse code summaries. Use when Codex has finished implementing a user request inside a hoyan folder or hoyan-related lab and is preparing the final response or implementation summary, especially for routing, RIB/FIB, gNMI, SR Linux, containerlab, topology, config, test, or network-control-plane changes.
---

# Hoyan Implementation Summary

## Overview

Prepare the final answer after a hoyan implementation so the user can understand what changed, why it works, and where the behavior matters in a network. The answer should read like a short implementation note grounded in concrete lab behavior, not a terse file summary.

Prefer specific network outcomes over generic statements like "updated routing logic" or "added tests." When the change adds support for a protocol attribute, policy field, route type, gNMI path, topology feature, or verifier behavior, explain:

- The topology or lab shape where the feature appears.
- The input that now matters: config stanza, intent field, CLI/gNMI data, route attribute, interface state, or topology edge.
- The internal object or code path that can now read, store, compare, render, or verify that input.
- The observable behavior: selected route, exported route, programmed next hop, generated config, diff output, live-check result, packet path, or policy decision.
- The situation where the change becomes useful, such as filtering routes, preserving attributes across import/export, catching an SR Linux mismatch, or building a more realistic lab.

## Final Response Requirements

When finishing the task, include these items when relevant:

- State the concrete user-visible behavior implemented, not only the files or functions changed.
- Explain the main data flow or control-plane flow in domain terms: topology input, config input, route derivation, RIB selection, FIB/programming result, gNMI response, packet forwarding result, generated config, or generated lab artifact.
- Give a small example when the change affects network behavior. Prefer the actual lab nodes, topology file, and config names from the implementation. If those are not available, use a clearly labeled illustrative topology such as `leaf1 -- spine1 -- leaf2` or `host1 -- r1 -- host2`.
- Mention concrete values when they make the behavior clearer: prefixes, next hops, ASNs, router IDs, interfaces, route preference, metrics, labels, communities, local-pref, MED, origin, route targets, address families, VRFs, policy names, gNMI paths, or containerlab node names.
- Connect the example back to the implementation: identify the module, function, config file, or test that now handles that case.
- Explain the "why this matters" scenario in one sentence for non-trivial protocol/model changes.
- Report verification commands or tests that were run, including meaningful results. If verification could not be run, say so directly and explain the remaining risk.
- Keep the answer concise, but make the explanation specific enough that the user can reason about the implementation without rereading the diff. Usually 2-4 short paragraphs or a compact bullet list is enough.

## Before Writing

Quickly inspect the changed files and tests so the summary uses real names instead of placeholders:

- Use `git diff --stat`, `git diff --name-only`, and targeted `git diff -- <path>` for the files that define behavior.
- For lab work, check the changed `*.clab.yml`, `README.md`, `configs/`, intent files, scripts, and test fixtures.
- For Go code, identify the structs, parser/collector functions, model builder, verifier, renderer, simulator, RIB/FIB logic, or CLI command touched by the diff.
- For tests, name the test or fixture and the case it proves, not just "added tests."

## Explanation Pattern

Use this structure unless the task is very small:

1. Start with a direct completion statement: what was implemented.
2. Add a concrete behavior example using actual lab objects when possible.
3. Describe the code path in implementation terms: where the input is parsed/collected, how it is stored or transformed, and where it is consumed.
4. Explain when the behavior matters operationally.
5. End with validation: tests, commands, or "not run" with reason.

## Detail Checklist

Choose the relevant details from this checklist. Do not force every item into every answer.

- **Topology/lab**: nodes, links, VRFs, ASNs, address families, generated topology filename, and whether a new lab or fixture was added.
- **Input/config**: the exact field or stanza now supported, such as a BGP community list, route policy term, OSPF area type, static route next hop, interface address, or gNMI path.
- **Parsed/model attributes**: the struct fields or normalized attributes that now exist or are populated.
- **Decision point**: how the new data affects route selection, import/export policy, SPF, RIB/FIB comparison, config rendering, live collection, or diff reporting.
- **Observable result**: what the user would see in generated config, live-check output, test fixture output, SR Linux CLI/gNMI data, or packet forwarding.
- **Verification**: exact command and meaningful outcome, for example `go test ./...` passed, a named test covers `community: 65000:100`, or live-check compared expected and actual RIB entries.

## Example Style

Prefer:

"Implemented IPv4 route export filtering for the hoyan BGP lab. For example, in a `host1 -- leaf1 -- spine1 -- leaf2 -- host2` topology, a route for `10.0.2.0/24` learned on `leaf1` is now installed in the local RIB, evaluated against the export policy, and advertised to `spine1` only when the prefix set matches. The receiving node then sees the selected next hop in its RIB and programs the corresponding FIB entry toward the spine-facing interface. This is handled in `...` and covered by `...`; I verified it with `...`."

Avoid:

"Updated the BGP code and added a test."

## BGP Community Example

If the implementation adds BGP community support, explain it with this level of specificity:

"Implemented BGP community parsing and propagation for the BGP lab. In a `ce1 -- leaf1 -- spine1 -- leaf2` example, `ce1` can advertise `10.10.0.0/24` with community `65000:100`. The collector/parser now reads that community from the route data, the BGP route model stores it on the route attributes, and the policy/verifier path can compare or preserve it instead of treating the route as only prefix plus next hop. This matters when an export policy on `leaf1` matches `65000:100` to advertise the customer route to `spine1`, while routes without that community stay local. The behavior is covered by `Test...` using fixture `...`, and I verified it with `go test ./...`."

Adapt this pattern to the real diff. Replace the topology, prefix, ASN, community, function names, lab name, and tests with actual values. If the change only parses communities but does not yet apply policy based on them, say that directly: "the model now preserves the attribute for later comparison; policy matching is not implemented in this change."

## Lab Example

If the implementation creates or changes a lab, describe what the lab demonstrates:

- Name the lab/topology file and the nodes.
- State the route or traffic scenario the lab exercises.
- Mention the configs or generated artifacts that make the scenario work.
- Say what command proves it, such as `containerlab deploy`, `go run ./cmd/hoyan live-check --topology ...`, or a focused Go test.

Example:

"Added the `bgp-community` lab with `ce1`, `leaf1`, and `spine1`. The lab demonstrates a customer route `10.10.0.0/24` entering on `ce1`, receiving community `65000:100`, and being exported from `leaf1` only through the matching route policy. The relevant SR Linux config is in `configs/leaf1.conf`, and the hoyan expected model is in `...`. I verified the generated model with `...`; I did not run the live containerlab check because `...`."

## Scope Control

Do not invent details. If the actual topology, prefixes, RIB/FIB output, route attributes, or node names are unknown, either use a clearly labeled illustrative example or say what was verified from code/tests only. Do not claim live lab verification unless the relevant commands actually ran.

Separate implemented behavior from likely follow-up behavior. For example, if a change stores BGP communities but does not yet implement import/export policy matching on communities, explain both parts clearly.
