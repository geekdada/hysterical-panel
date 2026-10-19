import type {
  MediaTypeObject,
  OpenAPI3,
  OperationObject,
  ParameterObject,
  ReferenceObject,
  RequestBodyObject,
  ResponseObject,
  SchemaObject,
} from "openapi-typescript";

export type HttpMethod = "get" | "post" | "put" | "patch" | "delete";

export type DocOperation = { method: HttpMethod; path: string; operation: OperationObject };

export type DocGroup = { name: string; description?: string; operations: DocOperation[] };

export type DocField = {
  name: string;
  required: boolean;
  type: string;
  description?: string;
};

export type DocResponse = { status: string; description: string; example?: unknown };

const METHODS: HttpMethod[] = ["get", "post", "put", "patch", "delete"];
const MAX_REF_DEPTH = 16;

function isRef(value: unknown): value is ReferenceObject {
  return typeof value === "object" && value !== null && "$ref" in value;
}

// Follows local "#/components/..." refs. The spec comes from our own backend,
// so an unresolvable ref is a bug there and fails loudly here.
export function resolveRef<T>(doc: OpenAPI3, value: T | ReferenceObject): T {
  let current: unknown = value;
  for (let depth = 0; isRef(current); depth++) {
    const ref = current.$ref;
    if (depth >= MAX_REF_DEPTH) throw new Error(`OpenAPI ref cycle at ${ref}`);
    current = ref
      .replace(/^#\//, "")
      .split("/")
      .reduce<unknown>((node, key) => (node as Record<string, unknown> | undefined)?.[key], doc);
    if (current === undefined) throw new Error(`Unresolved OpenAPI ref ${ref}`);
  }
  return current as T;
}

export function groupOperations(doc: OpenAPI3): DocGroup[] {
  const groups = new Map<string, DocGroup>(
    (doc.tags ?? []).map((tag) => [
      tag.name,
      { name: tag.name, description: tag.description, operations: [] },
    ])
  );
  for (const [path, rawItem] of Object.entries(doc.paths ?? {})) {
    const item = resolveRef(doc, rawItem);
    for (const method of METHODS) {
      const raw = item[method];
      if (!raw) continue;
      const operation = resolveRef(doc, raw);
      const name = operation.tags?.[0] ?? "";
      if (!groups.has(name)) groups.set(name, { name, operations: [] });
      groups.get(name)!.operations.push({ method, path, operation });
    }
  }
  return [...groups.values()].filter((group) => group.operations.length > 0);
}

export function operationParameters(doc: OpenAPI3, entry: DocOperation): ParameterObject[] {
  return (entry.operation.parameters ?? []).map((param) => resolveRef(doc, param));
}

function jsonMedia(
  doc: OpenAPI3,
  content: Record<string, MediaTypeObject | ReferenceObject> | undefined
): MediaTypeObject | undefined {
  const media = content?.["application/json"];
  return media ? resolveRef(doc, media) : undefined;
}

function requestBodyMedia(doc: OpenAPI3, entry: DocOperation): MediaTypeObject | undefined {
  const body = entry.operation.requestBody;
  return body ? jsonMedia(doc, resolveRef<RequestBodyObject>(doc, body).content) : undefined;
}

export function requestBodyExample(doc: OpenAPI3, entry: DocOperation): unknown {
  return requestBodyMedia(doc, entry)?.example;
}

// Request bodies are flat objects, so one level of properties is enough.
export function requestBodyFields(doc: OpenAPI3, entry: DocOperation): DocField[] {
  const media = requestBodyMedia(doc, entry);
  if (!media?.schema) return [];
  const schema = resolveRef<SchemaObject>(doc, media.schema);
  const properties = "properties" in schema ? (schema.properties ?? {}) : {};
  const required = new Set(schema.required ?? []);
  return Object.entries(properties).map(([name, property]) => {
    const resolved = resolveRef<SchemaObject>(doc, property);
    return {
      name,
      required: required.has(name),
      type: schemaTypeLabel(resolved),
      description: resolved.description,
    };
  });
}

export function schemaTypeLabel(schema: SchemaObject | undefined): string {
  if (!schema || !("type" in schema) || !schema.type) return "any";
  const types = Array.isArray(schema.type) ? schema.type : [schema.type];
  const label = types.filter((type) => type !== "null").join(" | ");
  return types.includes("null") || schema.nullable ? `${label} | null` : label;
}

export function operationResponses(doc: OpenAPI3, entry: DocOperation): DocResponse[] {
  return Object.entries(entry.operation.responses ?? {}).map(([status, raw]) => {
    const response = resolveRef<ResponseObject>(doc, raw);
    return {
      status,
      description: response.description,
      example: jsonMedia(doc, response.content)?.example,
    };
  });
}

export function curlCommand(origin: string, entry: DocOperation, doc: OpenAPI3): string {
  const params = operationParameters(doc, entry);
  let path = entry.path;
  for (const param of params.filter((p) => p.in === "path")) {
    path = path.replace(`{${param.name}}`, String(param.example ?? param.name.toUpperCase()));
  }
  const query = params
    .filter((p) => p.in === "query" && p.example !== undefined)
    .map((p) => `${encodeURIComponent(p.name)}=${queryValue(String(p.example))}`)
    .join("&");
  const url = `${origin}${path}${query ? `?${query}` : ""}`;
  const method = entry.method === "get" ? "" : `-X ${entry.method.toUpperCase()} `;
  const lines = [`curl ${method}"${url}"`, `  -H "Authorization: Bearer YOUR_TOKEN"`];
  const body = requestBodyExample(doc, entry);
  if (body !== undefined) {
    lines.push(`  -H "Content-Type: application/json"`, `  -d '${JSON.stringify(body)}'`);
  }
  return lines.join(" \\\n");
}

// "@" is legal in a query string, and email examples read better unescaped.
function queryValue(value: string): string {
  return encodeURIComponent(value).replaceAll("%40", "@");
}

export function exampleText(value: unknown): string {
  return JSON.stringify(value, null, 2);
}
