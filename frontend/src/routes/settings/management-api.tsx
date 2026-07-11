import { createFileRoute } from "@tanstack/react-router";
import { type ReactNode } from "react";
import { requireAdmin } from "~/api/guards";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";
import { BrandLink, CopyableCode, PageShell } from "~/components/ui";
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

const GET_RESPONSE = `{
  "id": "rec_8f3k2j1d0s9a7gx",
  "email": "user@example.com",
  "role": "user",
  "auth_string": "qZ4yN8tR2vL6wB1k",
  "quota_bytes": 0,
  "used_tx": 1048576,
  "used_rx": 5242880,
  "status": "active",
  "created": "2026-06-01T08:30:00.000Z"
}`;

const POST_RESPONSE = `{
  "id": "rec_8f3k2j1d0s9a7gx",
  "email": "user@example.com",
  "status": "active"
}`;

const ERROR_RESPONSE = `{
  "status": 401,
  "message": "invalid token",
  "data": {}
}`;

function ManagementApiDocsPage() {
  const { auth } = Route.useRouteContext();
  const base = usePanelApiOrigin();

  const getRequest = `curl "${base}/api/mgmt/users?email=user@example.com" \\
  -H "Authorization: Bearer YOUR_TOKEN"`;

  const postRequest = `curl -X POST "${base}/api/mgmt/users" \\
  -H "Authorization: Bearer YOUR_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"email":"user@example.com"}'`;

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

      <Endpoint
        method="GET"
        path="/api/mgmt/users"
        summary={m.mgmt_api_get_summary()}
        errors={m.mgmt_api_get_errors()}
      >
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={getRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
        <CodeBlock
          label={m.mgmt_api_label_response_200()}
          code={GET_RESPONSE}
          copyLabel={m.mgmt_api_copy_response()}
        />
      </Endpoint>

      <Endpoint
        method="POST"
        path="/api/mgmt/users"
        summary={m.mgmt_api_post_summary()}
        errors={m.mgmt_api_post_errors()}
      >
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={postRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
        <CodeBlock
          label={m.mgmt_api_label_response_201()}
          code={POST_RESPONSE}
          copyLabel={m.mgmt_api_copy_response()}
        />
      </Endpoint>

      <DocSection heading={m.mgmt_api_errors_heading()}>
        <p className="text-[13px] text-muted">{m.mgmt_api_errors_shape()}</p>
        <CodeBlock
          label={m.mgmt_api_label_response_4xx()}
          code={ERROR_RESPONSE}
          copyLabel={m.mgmt_api_copy_error()}
        />
      </DocSection>
    </PageShell>
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

function Endpoint({
  method,
  path,
  summary,
  errors,
  children,
}: {
  method: "GET" | "POST";
  path: string;
  summary: string;
  errors: string;
  children: ReactNode;
}) {
  return (
    <section className="mt-8 border-t border-border pt-6">
      <div className="flex items-center gap-2">
        <span className="rounded border bg-surface-secondary px-1.5 py-0.5 font-mono text-[11px] font-semibold tracking-wide text-foreground">
          {method}
        </span>
        <span className="font-mono text-[13px] text-foreground">{path}</span>
      </div>
      <p className="mt-2 text-[13px] text-muted">{summary}</p>
      <div className="mt-4 flex flex-col gap-4">{children}</div>
      <p className="mt-3 text-xs text-muted">
        {m.mgmt_api_errors_prefix()} {errors}
      </p>
    </section>
  );
}

function CodeBlock({ label, code, copyLabel }: { label: string; code: string; copyLabel: string }) {
  return (
    <div>
      <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-muted">{label}</p>
      <CopyableCode value={code} label={copyLabel} />
    </div>
  );
}

// A standalone monospace line (e.g. a header to send), styled like a code block.
function Code({ children }: { children: string }) {
  return <CopyableCode value={children} label={m.mgmt_api_copy_header()} />;
}
