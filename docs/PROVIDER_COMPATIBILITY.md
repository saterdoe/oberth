# Provider compatibility

Oberth treats “OpenAI-compatible” as a transport description, not as proof that
every model supports streaming, tools, structured actions, cancellation, token
usage, or a particular context window.

| Provider family | Chat | Streaming | Typed tools | Token usage | Notes |
| --- | --- | --- | --- | --- | --- |
| Ollama | Supported | Supported | Model-dependent | Supported or estimated | Context and tool support must be resolved for the selected model. |
| LM Studio / OpenAI-compatible local endpoint | Supported | Endpoint-dependent | Model and template-dependent | Endpoint-dependent | Verify the actual endpoint/model pair. |
| OpenAI-compatible remote endpoint | Supported | Endpoint-dependent | Endpoint-dependent | Endpoint-dependent | Compatibility does not imply identical limits or error behavior. |
| OpenAI | Supported | Supported | Model-dependent | Supported | Use published model capabilities and limits. |
| Anthropic | Supported | Supported | Model-dependent | Supported | Uses the native Anthropic adapter. |

The deterministic conformance harness in `internal/providercompat` exercises
chat completion, typed tool requests, streaming termination, cancellation,
deadlines, malformed responses and token-accounting behavior without cloud
credentials. A `partial` result means the transport responded but the requested
capability is not certified; Oberth must select a bounded fallback rather than
silently dropping tools or evidence.

For an installed provider, use `oberth provider verify <provider-id>` to verify
reachability and model discovery. That command is a connectivity check, not a
capability certification. Provider/model capability evidence must be captured
when a workflow stage starts and revalidated before fallback.
