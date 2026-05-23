"""Spike 1a: Create a Kubernaut investigation agent in Oracle Agent Spec format,
serialize to YAML, and verify round-trip."""

from pyagentspec import Agent, AgentSpecSerializer
from pyagentspec.llms.openaicompatibleconfig import OpenAiCompatibleConfig
from pyagentspec.tools.servertool import ServerTool
from pyagentspec.property import Property


def make_tool(name: str, description: str, inputs: list[Property]) -> ServerTool:
    return ServerTool(name=name, description=description, inputs=inputs)


agent = Agent(
    name="kubernaut-rca-investigator",
    description="Investigates Kubernetes alert signals and produces structured RCA",
    system_prompt=(
        "You are a Kubernetes incident investigator for the Kubernaut platform.\n"
        "A signal has fired. Use the available tools to inspect the affected "
        "resource, check events, query metrics, and determine the root cause.\n\n"
        "When you have identified the root cause, call submit_result with a "
        "structured JSON object containing:\n"
        "- root_cause: string describing the root cause\n"
        "- confidence: number 0-1\n"
        "- affected_resources: array of resource identifiers\n"
        "- remediation_suggested: boolean\n"
    ),
    llm_config=OpenAiCompatibleConfig(
        name="vertex-anthropic",
        description="Anthropic Claude via Vertex AI OpenAI-compatible endpoint",
        url="https://us-east5-aiplatform.googleapis.com/v1/projects/PROJECT/locations/us-east5/endpoints/openapi",  # pre-commit:allow-sensitive
        model_id="claude-sonnet-4-20250514",
    ),
    tools=[
        make_tool(
            "kubectl_get",
            "Get a Kubernetes resource by kind, name, and namespace",
            inputs=[
                Property(title="kind", type="string", description="Resource kind"),
                Property(title="name", type="string", description="Resource name"),
                Property(title="namespace", type="string", description="Namespace"),
            ],
        ),
        make_tool(
            "kubectl_list_events",
            "List Kubernetes events for a resource in a namespace",
            inputs=[
                Property(title="namespace", type="string", description="Namespace"),
                Property(title="resource_name", type="string", description="Resource name"),
            ],
        ),
        make_tool(
            "prometheus_query",
            "Execute a PromQL query and return metric results",
            inputs=[
                Property(title="query", type="string", description="PromQL expression"),
                Property(title="time_range", type="string", description="Time range, e.g. 1h"),
            ],
        ),
        make_tool(
            "submit_result",
            "Submit the structured RCA investigation result",
            inputs=[
                Property(
                    title="result",
                    type="object",
                    description="Structured RCA result",
                    json_schema={
                        "type": "object",
                        "properties": {
                            "root_cause": {"type": "string"},
                            "confidence": {"type": "number", "minimum": 0, "maximum": 1},
                            "affected_resources": {"type": "array", "items": {"type": "string"}},
                            "remediation_suggested": {"type": "boolean"},
                        },
                        "required": ["root_cause", "confidence"],
                    },
                ),
            ],
        ),
    ],
)

serializer = AgentSpecSerializer()
yaml_output = serializer.to_yaml(agent)

with open("kubernaut-rca-investigator.yaml", "w") as f:
    f.write(yaml_output)

print("[PASS] OAS YAML spec created and serialized")
print(f"       File: kubernaut-rca-investigator.yaml ({len(yaml_output)} bytes)")
print(f"       Tools: {len(agent.tools)}")
print(f"       Agent name: {agent.name}")
