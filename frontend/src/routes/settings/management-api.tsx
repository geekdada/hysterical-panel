import { useQuery } from "@tanstack/react-query";
import { createFileRoute } from "@tanstack/react-router";
import type { OpenAPI3, SchemaObject } from "openapi-typescript";
import { type ReactNode } from "react";
import { requireAdmin } from "~/api/guards";
import { managementApiSpecQueryOptions } from "~/api/queries";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";
import { exampleText, groupOperations, resolveRef } from "~/lib/openapi-doc";
import { CodeBlock, OpenApiOperation } from "~/components/openapi-operation";
import { BrandLink, CopyableCode, ErrorAlert, PageShell, TableSkeleton } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { breadcrumbStaticData } from "~/lib/breadcrumb-meta";
import * as m from "~/paraglide/messages.js";

export const Route = createFileRoute("/settings/management-api")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  staticData: breadcrumbStaticData({
    label: () => m.mgmt_api_title(),
  }),
  component: ManagementApiDocsPage,
});

function ManagementApiDocsPage() {
  const { auth } = Route.useRouteContext();
  const base = usePanelApiOrigin();
  const specQuery = useQuery(managementApiSpecQueryOptions());
  const doc = specQuery.data;

  const specRequest = `curl "${base}/api/mgmt/openapi.json" \\
  -H "Authorization: Bearer YOUR_TOKEN"`;

  return (
    <PageShell
      width="narrow"
      headerLeft={<BrandLink />}
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="mb-6">
        <h1 className="text-base font-semibold tracking-tight">{m.mgmt_api_title()}</h1>
        <p className="mt-0.5 text-[13px] text-muted">{m.mgmt_api_intro()}</p>
      </div>

      <DocSection heading={m.mgmt_api_auth_heading()}>
        <p className="text-[13px] text-muted">{m.mgmt_api_auth_bearer_intro()}</p>
        <Code>{m.mgmt_api_auth_header_example()}</Code>
        <p className="text-[13px] text-muted">{m.mgmt_api_auth_disabled()}</p>
      </DocSection>

      <DocSection heading={m.mgmt_api_spec_heading()}>
        <p className="text-[13px] text-muted">{m.mgmt_api_spec_intro()}</p>
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={specRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
      </DocSection>

      {specQuery.isPending && (
        <div className="mt-8">
          <TableSkeleton />
        </div>
      )}
      <ErrorAlert message={specQuery.error?.message} className="mt-8" />
      {doc && <SpecOperations doc={doc} origin={base} />}
    </PageShell>
  );
}

// The spec's own endpoint is already shown above with a copyable request.
const SPEC_TAG = "Specification";

function SpecOperations({ doc, origin }: { doc: OpenAPI3; origin: string }) {
  const groups = groupOperations(doc).filter((group) => group.name !== SPEC_TAG);
  const errorSchema = doc.components?.schemas?.ErrorResponse;
  const errorExample = errorSchema ? resolveRef<SchemaObject>(doc, errorSchema).example : undefined;

  return (
    <>
      {groups.map((group) => (
        <div key={group.name}>
          <GroupHeading>{group.name}</GroupHeading>
          {group.description && <p className="mt-1 text-[13px] text-muted">{group.description}</p>}
          {group.operations.map((entry) => (
            <OpenApiOperation
              key={`${entry.method} ${entry.path}`}
              doc={doc}
              entry={entry}
              origin={origin}
            />
          ))}
        </div>
      ))}

      {errorExample !== undefined && (
        <DocSection heading={m.mgmt_api_errors_heading()}>
          <p className="text-[13px] text-muted">{m.mgmt_api_errors_shape()}</p>
          <CodeBlock
            label={m.mgmt_api_label_response_4xx()}
            code={exampleText(errorExample)}
            copyLabel={m.mgmt_api_copy_error()}
          />
        </DocSection>
      )}
    </>
  );
}

function DocSection({ heading, children }: { heading: string; children: ReactNode }) {
  return (
    <section className="mt-8 border-t border-border pt-6 first:mt-0 first:border-t-0 first:pt-0">
      <h2 className="text-[13px] font-semibold text-foreground">{heading}</h2>
      <div className="mt-3 flex flex-col gap-3">{children}</div>
    </section>
  );
}

function GroupHeading({ children }: { children: string }) {
  return <h2 className="mt-12 text-sm font-semibold text-foreground">{children}</h2>;
}

// A standalone monospace line (e.g. a header to send), styled like a code block.
function Code({ children }: { children: string }) {
  return <CopyableCode value={children} label={m.mgmt_api_copy_header()} />;
}
