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
  "subscription_required": true,
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

const NODE_RESPONSE = `{
  "id": "k2m9x7c4v1b8n3q",
  "name": "Tokyo 1",
  "api_url": "https://tokyo-1.example.com:25413",
  "poll_interval": 30,
  "enabled": true,
  "last_polled_at": "2026-06-01 08:30:00.000Z",
  "last_error": "",
  "health": "ok",
  "current_tx_speed": 131072,
  "current_rx_speed": 524288,
  "online_devices": 12
}`;

const NODE_LIST_RESPONSE = `[
${NODE_RESPONSE.replace(/^/gm, "  ")}
]`;

const NODE_DISABLED_RESPONSE = `{
  "id": "k2m9x7c4v1b8n3q",
  "name": "Tokyo 1",
  "api_url": "https://tokyo-1.example.com:25413",
  "poll_interval": 30,
  "enabled": false,
  "last_polled_at": "2026-06-01 08:30:00.000Z",
  "last_error": "",
  "health": "ok",
  "current_tx_speed": 0,
  "current_rx_speed": 0,
  "online_devices": 0
}`;

const NODE_DELETE_RESPONSE = `{
  "deleted": true
}`;

const NODE_TEST_RESPONSE = `{
  "ok": true,
  "latency_ms": 42
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

  const nodesUrl = `${base}/api/mgmt/nodes`;
  const nodeUrl = `${nodesUrl}/NODE_ID`;
  const nodeListRequest = `curl "${nodesUrl}" \\
  -H "Authorization: Bearer YOUR_TOKEN"`;
  const nodeGetRequest = `curl "${nodeUrl}" \\
  -H "Authorization: Bearer YOUR_TOKEN"`;
  const nodeCreateRequest = `curl -X POST "${nodesUrl}" \\
  -H "Authorization: Bearer YOUR_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"name":"Tokyo 1","api_url":"https://tokyo-1.example.com:25413","api_secret":"NODE_API_SECRET"}'`;
  const nodeDisableRequest = `curl -X PATCH "${nodeUrl}" \\
  -H "Authorization: Bearer YOUR_TOKEN" \\
  -H "Content-Type: application/json" \\
  -d '{"enabled":false}'`;
  const nodeDeleteRequest = `curl -X DELETE "${nodeUrl}" \\
  -H "Authorization: Bearer YOUR_TOKEN"`;
  const nodeTestRequest = `curl -X POST "${nodeUrl}/test" \\
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

      <GroupHeading>{m.mgmt_api_users_heading()}</GroupHeading>

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

      <GroupHeading>{m.mgmt_api_nodes_heading()}</GroupHeading>
      <p className="mt-1 text-[13px] text-muted">{m.mgmt_api_nodes_intro()}</p>

      <Endpoint method="GET" path="/api/mgmt/nodes" summary={m.mgmt_api_nodes_list_summary()}>
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={nodeListRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
        <CodeBlock
          label={m.mgmt_api_label_response_200()}
          code={NODE_LIST_RESPONSE}
          copyLabel={m.mgmt_api_copy_response()}
        />
      </Endpoint>

      <Endpoint
        method="GET"
        path="/api/mgmt/nodes/{id}"
        summary={m.mgmt_api_nodes_get_summary()}
        errors={m.mgmt_api_nodes_not_found_errors()}
      >
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={nodeGetRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
        <CodeBlock
          label={m.mgmt_api_label_response_200()}
          code={NODE_RESPONSE}
          copyLabel={m.mgmt_api_copy_response()}
        />
      </Endpoint>

      <Endpoint
        method="POST"
        path="/api/mgmt/nodes"
        summary={m.mgmt_api_nodes_create_summary()}
        errors={m.mgmt_api_nodes_create_errors()}
      >
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={nodeCreateRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
        <CodeBlock
          label={m.mgmt_api_label_response_201()}
          code={NODE_RESPONSE}
          copyLabel={m.mgmt_api_copy_response()}
        />
      </Endpoint>

      <Endpoint
        method="PATCH"
        path="/api/mgmt/nodes/{id}"
        summary={m.mgmt_api_nodes_update_summary()}
        errors={m.mgmt_api_nodes_update_errors()}
      >
        <p className="text-[13px] text-muted">{m.mgmt_api_nodes_update_enabled()}</p>
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={nodeDisableRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
        <CodeBlock
          label={m.mgmt_api_label_response_200()}
          code={NODE_DISABLED_RESPONSE}
          copyLabel={m.mgmt_api_copy_response()}
        />
      </Endpoint>

      <Endpoint
        method="DELETE"
        path="/api/mgmt/nodes/{id}"
        summary={m.mgmt_api_nodes_delete_summary()}
        errors={m.mgmt_api_nodes_not_found_errors()}
      >
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={nodeDeleteRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
        <CodeBlock
          label={m.mgmt_api_label_response_200()}
          code={NODE_DELETE_RESPONSE}
          copyLabel={m.mgmt_api_copy_response()}
        />
      </Endpoint>

      <Endpoint
        method="POST"
        path="/api/mgmt/nodes/{id}/test"
        summary={m.mgmt_api_nodes_test_summary()}
        errors={m.mgmt_api_nodes_not_found_errors()}
      >
        <CodeBlock
          label={m.mgmt_api_label_request()}
          code={nodeTestRequest}
          copyLabel={m.mgmt_api_copy_request()}
        />
        <CodeBlock
          label={m.mgmt_api_label_response_200()}
          code={NODE_TEST_RESPONSE}
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

function GroupHeading({ children }: { children: string }) {
  return <h2 className="mt-12 text-sm font-semibold text-foreground">{children}</h2>;
}

function Endpoint({
  method,
  path,
  summary,
  errors,
  children,
}: {
  method: "GET" | "POST" | "PATCH" | "DELETE";
  path: string;
  summary: string;
  errors?: string;
  children: ReactNode;
}) {
  return (
    <section className="mt-8 border-t border-border pt-6">
      <div className="flex items-center gap-2">
        <span className="rounded border bg-surface-secondary px-1.5 py-0.5 font-mono text-[11px] tracking-wide text-foreground">
          {method}
        </span>
        <span className="font-mono text-[13px] text-foreground">{path}</span>
      </div>
      <p className="mt-2 text-[13px] text-muted">{summary}</p>
      <div className="mt-4 flex flex-col gap-4">{children}</div>
      {errors && (
        <p className="mt-3 text-xs text-muted">
          {m.mgmt_api_errors_prefix()} {errors}
        </p>
      )}
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
