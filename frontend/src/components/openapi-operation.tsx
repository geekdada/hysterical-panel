import type { OpenAPI3 } from "openapi-typescript";
import type { ReactNode } from "react";
import { CopyableCode, Td, Th } from "~/components/ui";
import {
  curlCommand,
  exampleText,
  operationParameters,
  operationResponses,
  requestBodyFields,
  schemaTypeLabel,
  type DocOperation,
} from "~/lib/openapi-doc";
import * as m from "~/paraglide/messages.js";

// 401 is shared by every operation and explained once in the auth section.
const SHARED_ERROR_STATUS = "401";

export function OpenApiOperation({
  doc,
  entry,
  origin,
}: {
  doc: OpenAPI3;
  entry: DocOperation;
  origin: string;
}) {
  const { operation } = entry;
  const responses = operationResponses(doc, entry);
  const successes = responses.filter((r) => r.status.startsWith("2") && r.example !== undefined);
  const errors = responses
    .filter((r) => !r.status.startsWith("2") && r.status !== SHARED_ERROR_STATUS)
    .map((r) => `${r.status} ${r.description}`)
    .join(". ");

  return (
    <section className="mt-8 border-t border-border pt-6">
      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded border bg-surface-secondary px-1.5 py-0.5 font-mono text-[11px] tracking-wide text-foreground">
          {entry.method.toUpperCase()}
        </span>
        <span className="break-all font-mono text-[13px] text-foreground">{entry.path}</span>
      </div>
      {operation.summary && <p className="mt-2 text-[13px] text-foreground">{operation.summary}</p>}
      {operation.description && (
        <p className="mt-1 whitespace-pre-line text-[13px] text-muted">{operation.description}</p>
      )}
      <div className="mt-4 flex flex-col gap-4">
        <ParameterTable doc={doc} entry={entry} />
        <BodyTable doc={doc} entry={entry} />
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={curlCommand(origin, entry, doc)}
          copyLabel={m.mgmt_api_copy_request()}
        />
        {successes.map((response) => (
          <CodeBlock
            key={response.status}
            label={m.mgmt_api_label_response({ status: response.status })}
            code={exampleText(response.example)}
            copyLabel={m.mgmt_api_copy_response()}
          />
        ))}
      </div>
      {errors && (
        <p className="mt-3 text-xs text-muted">
          {m.mgmt_api_errors_prefix()} {errors}
        </p>
      )}
    </section>
  );
}

function ParameterTable({ doc, entry }: { doc: OpenAPI3; entry: DocOperation }) {
  const params = operationParameters(doc, entry);
  if (params.length === 0) return null;
  return (
    <FieldTable label={m.mgmt_api_label_parameters()} withLocation>
      {params.map((param) => (
        <FieldRow
          key={`${param.in}:${param.name}`}
          name={param.name}
          location={param.in}
          required={param.required ?? false}
          type={schemaTypeLabel(param.schema)}
          description={param.description}
        />
      ))}
    </FieldTable>
  );
}

function BodyTable({ doc, entry }: { doc: OpenAPI3; entry: DocOperation }) {
  const fields = requestBodyFields(doc, entry);
  if (fields.length === 0) return null;
  return (
    <FieldTable label={m.mgmt_api_label_body()}>
      {fields.map((field) => (
        <FieldRow key={field.name} {...field} />
      ))}
    </FieldTable>
  );
}

function FieldTable({
  label,
  withLocation = false,
  children,
}: {
  label: string;
  withLocation?: boolean;
  children: ReactNode;
}) {
  return (
    <div>
      <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">{label}</p>
      <div className="overflow-x-auto rounded border border-border">
        <table className="w-full text-left text-[13px]">
          <thead className="border-b border-border bg-surface-secondary">
            <tr>
              <Th>{m.mgmt_api_col_name()}</Th>
              {withLocation && <Th>{m.mgmt_api_col_location()}</Th>}
              <Th>{m.mgmt_api_col_required()}</Th>
              <Th>{m.mgmt_api_col_type()}</Th>
              <Th>{m.mgmt_api_col_description()}</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-separator">{children}</tbody>
        </table>
      </div>
    </div>
  );
}

function FieldRow({
  name,
  location,
  required,
  type,
  description,
}: {
  name: string;
  location?: string;
  required: boolean;
  type: string;
  description?: string;
}) {
  return (
    <tr>
      <Td className="font-mono text-foreground">{name}</Td>
      {location !== undefined && <Td className="text-muted">{location}</Td>}
      <Td className="text-muted">{required ? m.mgmt_api_required_yes() : ""}</Td>
      <Td className="font-mono text-muted">{type}</Td>
      <Td className="text-muted">{description}</Td>
    </tr>
  );
}

export function CodeBlock({
  label,
  code,
  copyLabel,
}: {
  label: string;
  code: string;
  copyLabel: string;
}) {
  return (
    <div>
      <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">{label}</p>
      <CopyableCode value={code} label={copyLabel} />
    </div>
  );
}
