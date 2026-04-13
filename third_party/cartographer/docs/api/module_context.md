# Project Cartographer API Documentation

## Module Context API: `get_module_context`

The `get_module_context` API endpoint provides a lightweight, semantically rich representation of a specific module's public API surface and its dependencies. This API is designed for efficient consumption by AI agents, drastically reducing token count while maintaining essential structural information.

### Endpoint
`/api/v1/module-context` (Example - actual endpoint path may vary based on implementation)

### Method
`GET` (or `POST` if complex query parameters are needed)

### Parameters
| Parameter | Type   | Description                                                                                             | Required |
| :-------- | :----- | :------------------------------------------------------------------------------------------------------ | :------- |
| `moduleId`  | String | **Unique identifier for the module to retrieve context for (e.g., file path or module name).**            | Yes      |
| `depth`     | Integer | Optional. Controls the depth of transitive dependencies to include. `0` for module only, `1` for direct dependencies, etc. | No       |
| `include`   | Array  | Optional. List of specific elements to include (e.g., `"imports"`, `"exports"`, `"types"`). Defaults to all public API surface.  | No       |
| `format`    | String | Optional. Desired output format (e.g., `"compressed-ai-lang"`, `"json"`). Defaults to `compressed-ai-lang` for token efficiency. | No       |

### Response
The API returns a compressed representation of the module's public API surface. This includes function signatures, class/interface definitions, type declarations, and optionally import/export statements, and transitive dependencies based on the `depth` parameter.

**Example (conceptual compressed-ai-lang format):**
```
(module: UserAuth)
 (imports: [express, bcrypt])
 (exports:
  (func: login (params: email, password))
  (func: register (params: username, email, password))
  (class: User (props: id, email, hashedPassword)))
```

### Token Savings
The `compressed-ai-lang` format is highly optimized to minimize token usage for LLMs, achieving up to a 96% reduction compared to raw source code.

## Project Graph JSON (`project_graph.json`)

While `get_module_context` provides on-demand module details, the `project_graph.json` file offers a static, comprehensive map of the entire codebase. This file is generated and maintained by the `cartographer_service.py` background worker and is primarily consumed by systems requiring a global view, such as Hop AI. It contains metadata about files/modules, their exported signatures, and their interdependencies. Its format is also optimized for size, removing whitespace and normalizing formatting.