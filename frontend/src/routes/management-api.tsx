import { Link, createFileRoute } from "@tanstack/react-router";
import { type ReactNode } from "react";
import { requireAdmin } from "~/api/guards";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";
import { BackLink, CopyButton, PageShell } from "~/components/ui";
import { UserMenu } from "~/components/user-menu";

export const Route = createFileRoute("/management-api")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
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
      headerLeft={
        <div className="flex min-w-0 items-center gap-3">
          <BackLink to="/settings" label="Settings" />
          <span className="truncate text-[13px] font-semibold tracking-tight">Management API</span>
        </div>
      }
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <div className="mb-6">
        <h1 className="text-base font-semibold tracking-tight">Management API</h1>
        <p className="mt-0.5 text-[13px] text-(--muted)">
          HTTP endpoints for external services to look up and provision users. Enable the API on the{" "}
          <Link to="/settings" className="text-(--accent) hover:underline">
            Settings
          </Link>{" "}
          page; a token is generated and shown once.
        </p>
      </div>

      <DocSection heading="Authentication">
        <p className="text-[13px] text-(--muted)">
          Send the configured token as a bearer token on every request:
        </p>
        <Code>Authorization: Bearer YOUR_TOKEN</Code>
        <p className="text-[13px] text-(--muted)">
          While the API is turned off, every endpoint returns <Inline>404</Inline>. A missing or
          invalid token returns <Inline>401</Inline>.
        </p>
      </DocSection>

      <Endpoint
        method="GET"
        path="/api/mgmt/users"
        summary="Look up a single user by email or auth_string. Provide exactly one of the two."
        errors={
          <>
            <Inline>400</Inline> if both or neither parameter is sent. <Inline>404</Inline> if no
            user matches.
          </>
        }
      >
        <CodeBlock label="Request" code={getRequest} copyLabel="request" />
        <CodeBlock label="Response · 200" code={GET_RESPONSE} copyLabel="response" />
      </Endpoint>

      <Endpoint
        method="POST"
        path="/api/mgmt/users"
        summary="Create a user from an email. The password and auth_string are generated server-side and never returned; the account is created active and verified with the user role. Send the new user through the password reset flow to grant access."
        errors={
          <>
            <Inline>400</Inline> if the body is invalid, the email is missing, or the email is
            already taken.
          </>
        }
      >
        <CodeBlock label="Request" code={postRequest} copyLabel="request" />
        <CodeBlock label="Response · 201" code={POST_RESPONSE} copyLabel="response" />
      </Endpoint>

      <DocSection heading="Error responses">
        <p className="text-[13px] text-(--muted)">All errors share the same shape:</p>
        <CodeBlock label="Response · 4xx" code={ERROR_RESPONSE} copyLabel="error" />
      </DocSection>
    </PageShell>
  );
}

function DocSection({ heading, children }: { heading: string; children: ReactNode }) {
  return (
    <section className="mt-8 border-t border-(--border) pt-6 first:mt-0 first:border-t-0 first:pt-0">
      <h2 className="text-[13px] font-semibold text-(--foreground)">{heading}</h2>
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
  errors: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="mt-8 border-t border-(--border) pt-6">
      <div className="flex items-center gap-2">
        <span className="rounded border border-(--border) bg-(--surface-secondary) px-1.5 py-0.5 font-mono text-[11px] font-semibold tracking-wide text-(--foreground)">
          {method}
        </span>
        <span className="font-mono text-[13px] text-(--foreground)">{path}</span>
      </div>
      <p className="mt-2 text-[13px] text-(--muted)">{summary}</p>
      <div className="mt-4 flex flex-col gap-4">{children}</div>
      <p className="mt-3 text-xs text-(--muted)">Errors: {errors}</p>
    </section>
  );
}

// A monospace block with a copy button tucked into the corner.
function CopyablePre({ value, copyLabel }: { value: string; copyLabel: string }) {
  return (
    <div className="group/key relative">
      <pre className="overflow-x-auto rounded-(--radius) border border-(--border) bg-(--surface-secondary) p-3 pr-9 font-mono text-xs leading-relaxed text-(--foreground)">
        {value}
      </pre>
      <div className="absolute right-2 top-2">
        <CopyButton value={value} label={copyLabel} />
      </div>
    </div>
  );
}

function CodeBlock({ label, code, copyLabel }: { label: string; code: string; copyLabel: string }) {
  return (
    <div>
      <p className="mb-1.5 text-[11px] font-medium uppercase tracking-wider text-(--muted)">
        {label}
      </p>
      <CopyablePre value={code} copyLabel={copyLabel} />
    </div>
  );
}

// A standalone monospace line (e.g. a header to send), styled like a code block.
function Code({ children }: { children: string }) {
  return <CopyablePre value={children} copyLabel="header" />;
}

// Inline monospace token used within prose (status codes, field names).
function Inline({ children }: { children: ReactNode }) {
  return (
    <code className="rounded bg-(--surface-secondary) px-1 py-0.5 font-mono text-[12px] text-(--foreground)">
      {children}
    </code>
  );
}
