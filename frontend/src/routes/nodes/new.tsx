import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createFileRoute, Link, useNavigate } from "@tanstack/react-router";
import { useForm } from "@tanstack/react-form";
import {
  Button,
  Description,
  FieldError,
  Input,
  Label,
  NumberField,
  Switch,
  TextField,
} from "@heroui/react";
import { requireAdmin } from "~/api/guards";
import { apiClient } from "~/api/client";
import type { components } from "~/api/schema";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";

type Node = components["schemas"]["Node"];

export const Route = createFileRoute("/nodes/new")({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: AddNodePage,
});

type TestState =
  | { status: "pending" }
  | { status: "ok"; latencyMs: number }
  | { status: "error"; message: string };

function AddNodePage() {
  const navigate = useNavigate();
  const [created, setCreated] = useState<Node | null>(null);
  const [test, setTest] = useState<TestState>({ status: "pending" });
  const [submitError, setSubmitError] = useState("");

  const runTest = useCallback(async (id: string) => {
    setTest({ status: "pending" });
    const { data, error } = await apiClient.POST("/api/panel/nodes/{id}/test", {
      params: { path: { id } },
    });
    if (error || !data) {
      setTest({
        status: "error",
        message: "Couldn't run the connectivity test.",
      });
      return;
    }
    if (data.ok) {
      setTest({ status: "ok", latencyMs: data.latency_ms ?? 0 });
    } else {
      setTest({
        status: "error",
        message: data.error || "Node is unreachable.",
      });
    }
  }, []);

  const form = useForm({
    defaultValues: {
      name: "",
      api_url: "",
      api_secret: "",
      poll_interval: 30,
      enabled: true,
    },
    onSubmit: async ({ value }) => {
      setSubmitError("");
      const { data, error } = await apiClient.POST("/api/panel/nodes", {
        body: {
          name: value.name.trim(),
          api_url: value.api_url.trim(),
          api_secret: value.api_secret,
          poll_interval: value.poll_interval,
          enabled: value.enabled,
        },
      });
      if (error || !data?.id) {
        setSubmitError(errorMessage(error) || "Couldn't create the node.");
        return;
      }
      setCreated(data);
      void runTest(data.id);
    },
  });

  function addAnother() {
    form.reset();
    setCreated(null);
    setTest({ status: "pending" });
    setSubmitError("");
  }

  return (
    <div className="min-h-svh bg-(--background) text-(--foreground)">
      <header className="sticky top-0 z-20 border-b border-(--border) bg-(--surface)">
        <div className="mx-auto flex h-12 max-w-7xl items-center justify-between px-4 sm:px-6">
          <div className="flex items-center gap-2">
            <span className="grid size-5 place-items-center rounded-[5px] bg-(--accent) text-[11px] font-bold text-(--accent-foreground)">
              H
            </span>
            <span className="text-[13px] font-semibold tracking-tight">
              Hysterical Panel
            </span>
          </div>
          <Link
            to="/"
            className="text-xs text-(--muted) transition-colors duration-150 hover:text-(--foreground)"
          >
            ← Dashboard
          </Link>
        </div>
      </header>

      <main className="mx-auto max-w-xl px-4 py-8 sm:px-6">
        <div className="mb-5">
          <h1 className="text-base font-semibold tracking-tight">Add node</h1>
          <p className="mt-0.5 text-[13px] text-(--muted)">
            Register a Hysteria node's API endpoint to start collecting traffic.
          </p>
        </div>

        <div className="rounded-(--radius) border border-(--border) bg-(--surface) p-5">
          {created ? (
            <CreatedView
              node={created}
              test={test}
              onRetry={() => created.id && runTest(created.id)}
              onAddAnother={addAnother}
              onDone={() => navigate({ to: "/" })}
            />
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                e.stopPropagation();
                void form.handleSubmit();
              }}
              className="flex flex-col gap-5"
              noValidate
              autoComplete="off"
            >
              {/* Decoy inputs absorb Chrome's password-manager autofill,
                  which ignores autocomplete="off" on real fields. */}
              <div aria-hidden className="hidden">
                <input
                  type="text"
                  name="username"
                  tabIndex={-1}
                  autoComplete="username"
                />
                <input
                  type="password"
                  name="password"
                  tabIndex={-1}
                  autoComplete="current-password"
                />
              </div>
              <form.Field
                name="name"
                validators={{
                  onChange: ({ value }) =>
                    !value.trim()
                      ? "Name is required"
                      : value.trim().length > 128
                        ? "Keep the name under 128 characters"
                        : undefined,
                }}
              >
                {(field) => (
                  <TextField
                    name="name"
                    value={field.state.value}
                    onChange={field.handleChange}
                    onBlur={field.handleBlur}
                    isInvalid={field.state.meta.errors.length > 0}
                    isRequired
                  >
                    <Label>Name</Label>
                    <Input
                      placeholder="hk-01"
                      autoFocus
                      autoComplete="off"
                      data-1p-ignore
                      data-lpignore="true"
                      data-form-type="other"
                    />
                    <FieldError>
                      {field.state.meta.errors.join(", ")}
                    </FieldError>
                  </TextField>
                )}
              </form.Field>

              <form.Field
                name="api_url"
                validators={{
                  onChange: ({ value }) => validateUrl(value),
                }}
              >
                {(field) => (
                  <TextField
                    name="api_url"
                    value={field.state.value}
                    onChange={field.handleChange}
                    onBlur={field.handleBlur}
                    isInvalid={field.state.meta.errors.length > 0}
                    isRequired
                  >
                    <Label>API URL</Label>
                    <Input
                      type="url"
                      inputMode="url"
                      placeholder="http://203.0.113.10:9999"
                      className="font-mono text-[13px]"
                      autoComplete="url"
                      pattern="https?://.*"
                      data-1p-ignore
                      data-lpignore="true"
                      data-form-type="other"
                    />
                    {field.state.meta.errors.length > 0 ? (
                      <FieldError>
                        {field.state.meta.errors.join(", ")}
                      </FieldError>
                    ) : (
                      <Description>
                        The Traffic Stats API address (the `trafficStats` listen
                        port, not the proxy port).
                      </Description>
                    )}
                  </TextField>
                )}
              </form.Field>

              <form.Field
                name="api_secret"
                validators={{
                  onChange: ({ value }) =>
                    !value ? "API secret is required" : undefined,
                }}
              >
                {(field) => (
                  <TextField
                    name="api_secret"
                    value={field.state.value}
                    onChange={field.handleChange}
                    onBlur={field.handleBlur}
                    isInvalid={field.state.meta.errors.length > 0}
                    isRequired
                  >
                    <div className="flex items-center justify-between gap-2">
                      <Label>API secret</Label>
                      <button
                        type="button"
                        onClick={() => {
                          field.handleChange(generateSecret());
                        }}
                        className="rounded text-xs font-medium text-(--accent) transition-opacity duration-150 hover:opacity-80 focus-visible:underline focus-visible:outline-none"
                      >
                        Generate
                      </button>
                    </div>
                    <div className="relative">
                      <Input
                        type="text"
                        autoComplete="new-password"
                        className="w-full pr-10 font-mono text-[13px]"
                        data-1p-ignore
                        data-lpignore="true"
                        data-form-type="other"
                      />
                      <div className="absolute inset-y-0 right-1.5 flex items-center gap-0.5">
                        <CopyButton
                          value={field.state.value}
                          label="API secret"
                        />
                      </div>
                    </div>
                    {field.state.meta.errors.length > 0 ? (
                      <FieldError>
                        {field.state.meta.errors.join(", ")}
                      </FieldError>
                    ) : (
                      <Description>
                        Generate one or paste the node's secret, then copy it
                        into the server config. Stored encrypted and hidden
                        after saving.
                      </Description>
                    )}
                  </TextField>
                )}
              </form.Field>

              <form.Field
                name="poll_interval"
                validators={{
                  onChange: ({ value }) =>
                    value == null || Number.isNaN(value)
                      ? "Poll interval is required"
                      : !Number.isInteger(value) || value < 1
                        ? "Use a whole number of seconds, at least 1"
                        : undefined,
                }}
              >
                {(field) => (
                  <NumberField
                    name="poll_interval"
                    value={field.state.value}
                    onChange={field.handleChange}
                    onBlur={field.handleBlur}
                    minValue={1}
                    step={1}
                    isInvalid={field.state.meta.errors.length > 0}
                    isRequired
                    className="max-w-48"
                  >
                    <Label>Poll interval</Label>
                    <NumberField.Group>
                      <NumberField.DecrementButton />
                      <NumberField.Input />
                      <NumberField.IncrementButton />
                    </NumberField.Group>
                    {field.state.meta.errors.length > 0 ? (
                      <FieldError>
                        {field.state.meta.errors.join(", ")}
                      </FieldError>
                    ) : (
                      <Description>Seconds between traffic polls.</Description>
                    )}
                  </NumberField>
                )}
              </form.Field>

              <form.Field name="enabled">
                {(field) => (
                  <Switch
                    isSelected={field.state.value}
                    onChange={field.handleChange}
                    className="justify-between"
                  >
                    <Switch.Content>
                      <Label>Enabled</Label>
                      <Description>
                        Poll this node as soon as it's saved.
                      </Description>
                    </Switch.Content>
                    <Switch.Control>
                      <Switch.Thumb />
                    </Switch.Control>
                  </Switch>
                )}
              </form.Field>

              {submitError && (
                <div
                  className="flex items-center gap-2 rounded-(--radius) border border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)"
                  role="alert"
                >
                  <StatusDot tone="error" />
                  <span>{submitError}</span>
                </div>
              )}

              <div className="flex items-center justify-end gap-2 border-t border-(--separator) pt-4">
                <Button variant="ghost" onPress={() => navigate({ to: "/" })}>
                  Cancel
                </Button>
                <form.Subscribe
                  selector={(s) => ({
                    canSubmit: s.canSubmit,
                    isSubmitting: s.isSubmitting,
                  })}
                >
                  {({ canSubmit, isSubmitting }) => (
                    <Button
                      type="submit"
                      variant="primary"
                      isDisabled={!canSubmit}
                    >
                      {isSubmitting ? "Adding…" : "Add node"}
                    </Button>
                  )}
                </form.Subscribe>
              </div>
            </form>
          )}
        </div>

        {!created && (
          <form.Subscribe selector={(s) => s.values.api_secret}>
            {(apiSecret) => <ServerSetup apiSecret={apiSecret} />}
          </form.Subscribe>
        )}
      </main>
    </div>
  );
}

/* ── Hysteria server setup guidance ────────────────────────────────────── */

const SECRET_PLACEHOLDER = "<random-string>";

function setupYaml(apiSecret: string): { code: string; note?: string }[] {
  const secret = apiSecret || SECRET_PLACEHOLDER;

  return [
    { code: "trafficStats:" },
    {
      code: "  listen: :9999",
      note: "API address; must be reachable from the panel",
    },
    {
      code: `  secret: ${secret}`,
      note: "sent as the Authorization header",
    },
  ];
}

const PANEL_URL_PLACEHOLDER = "<panel-base-url>";

function authYaml(panelOrigin: string): { code: string; note?: string }[] {
  return [
    { code: "auth:" },
    { code: "  type: http" },
    { code: "  http:" },
    { code: `    url: ${panelOrigin}/api/hysteria/auth`, note: "this panel" },
    {
      code: "    insecure: false",
      note: "true only if the panel uses self-signed TLS",
    },
  ];
}

function ServerSetup({ apiSecret }: { apiSecret: string }) {
  const resolvedOrigin = usePanelApiOrigin();
  const panelOrigin = resolvedOrigin || PANEL_URL_PLACEHOLDER;

  return (
    <section className="mt-4 rounded-(--radius) border border-(--border) bg-(--surface) p-5">
      <h2 className="text-[13px] font-semibold tracking-tight">
        On the Hysteria server
      </h2>
      <p className="mt-1 max-w-prose text-[13px] text-(--muted)">
        The panel only reads stats; it never deploys the node. Enable the
        Traffic Stats API in the node's{" "}
        <span className="font-mono">server.yaml</span>, then restart Hysteria.
      </p>

      <pre className="mt-3 overflow-x-auto rounded-(--radius) border border-(--border) bg-(--surface-secondary) p-3 font-mono text-xs leading-relaxed">
        {setupYaml(apiSecret).map((line) => (
          <div key={line.code}>
            <span className="text-(--foreground)">{line.code}</span>
            {line.note && (
              <span className="text-(--muted)">{`  # ${line.note}`}</span>
            )}
          </div>
        ))}
      </pre>

      <h3 className="mt-5 text-[13px] font-semibold tracking-tight">
        Authenticate clients against this panel
      </h3>
      <p className="mt-1 max-w-prose text-[13px] text-(--muted)">
        Point Hysteria's <span className="font-mono">auth.http.url</span> at the
        panel. Each connect attempt is checked against the{" "}
        <span className="font-mono">auth_string</span> of the matching user;
        disabled accounts are rejected.
      </p>

      <pre className="mt-3 overflow-x-auto rounded-(--radius) border border-(--border) bg-(--surface-secondary) p-3 font-mono text-xs leading-relaxed">
        {authYaml(panelOrigin).map((line) => (
          <div key={line.code}>
            <span className="text-(--foreground)">{line.code}</span>
            {line.note && (
              <span className="text-(--muted)">{`  # ${line.note}`}</span>
            )}
          </div>
        ))}
      </pre>
      <p className="mt-1.5 text-xs text-(--muted)">
        Adjust the host if Hysteria reaches the panel through a different URL
        than your browser does.
      </p>

      <dl className="mt-4 flex flex-col gap-1.5 text-[13px]">
        <SetupRow term="API URL">
          <span className="font-mono">http://&lt;server-ip&gt;:9999</span> — the
          listen address above, no trailing slash.
        </SetupRow>
        <SetupRow term="API secret">
          the <span className="font-mono">secret</span> value above; the panel
          stores it encrypted.
        </SetupRow>
        <SetupRow term="Auth check">
          the client's <span className="font-mono">auth</span> string must match
          a user's <span className="font-mono">auth_string</span>; disabled
          users are rejected.
        </SetupRow>
        <SetupRow term="Reachability">
          open the stats port to the panel only, and keep it behind a firewall
          or TLS. It exposes per-user traffic.
        </SetupRow>
      </dl>
    </section>
  );
}

function SetupRow({ term, children }: { term: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:gap-2">
      <dt className="shrink-0 font-medium text-(--foreground) sm:w-28">
        {term}
      </dt>
      <dd className="text-(--muted)">{children}</dd>
    </div>
  );
}

/* ── Post-create verification view ─────────────────────────────────────── */

function CreatedView({
  node,
  test,
  onRetry,
  onAddAnother,
  onDone,
}: {
  node: Node;
  test: TestState;
  onRetry: () => void;
  onAddAnother: () => void;
  onDone: () => void;
}) {
  return (
    <div className="flex flex-col gap-5">
      <div className="flex items-start gap-2.5">
        <StatusDot tone="ok" className="mt-1.5" />
        <div>
          <p className="text-[13px] font-medium">Node created</p>
          <p className="mt-0.5 text-xs text-(--muted)">
            <span className="font-mono">{node.name}</span> is registered.
          </p>
        </div>
      </div>

      <div className="rounded-(--radius) border border-(--border) bg-(--surface-secondary) px-3 py-2.5 text-[13px]">
        {test.status === "pending" && (
          <div className="flex items-center gap-2 text-(--muted)">
            <span className="inline-block size-2 shrink-0 animate-pulse rounded-full bg-(--muted)" />
            Testing connection…
          </div>
        )}
        {test.status === "ok" && (
          <div className="flex items-center gap-2">
            <StatusDot tone="ok" />
            <span>Reachable</span>
            <span className="ml-auto font-mono tabular-nums text-(--muted)">
              {test.latencyMs} ms
            </span>
          </div>
        )}
        {test.status === "error" && (
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center gap-2">
              <StatusDot tone="error" />
              <span
                className="block truncate text-(--danger)"
                title={test.message}
              >
                {test.message}
              </span>
              <button
                type="button"
                onClick={onRetry}
                className="ml-auto shrink-0 text-xs text-(--muted) underline-offset-2 transition-colors duration-150 hover:text-(--foreground) hover:underline"
              >
                Retry
              </button>
            </div>
            <p className="text-xs text-(--muted)">
              The node was saved and will still be polled. You can retest it
              from the dashboard.
            </p>
          </div>
        )}
      </div>

      <div className="flex items-center justify-end gap-2 border-t border-(--separator) pt-4">
        <Button variant="ghost" onPress={onAddAnother}>
          Add another
        </Button>
        <Button variant="primary" onPress={onDone}>
          Back to dashboard
        </Button>
      </div>
    </div>
  );
}

/* ── Helpers ───────────────────────────────────────────────────────────── */

function StatusDot({
  tone,
  className = "",
}: {
  tone: "ok" | "error";
  className?: string;
}) {
  const fill = tone === "ok" ? "bg-(--success)" : "bg-(--danger)";
  return (
    <span
      className={`inline-block size-2 shrink-0 rounded-full ${fill} ${className}`}
    />
  );
}

function IconAction({
  label,
  onClick,
  disabled = false,
  children,
}: {
  label: string;
  onClick: () => void;
  disabled?: boolean;
  children: ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      title={label}
      aria-label={label}
      className="inline-grid size-7 place-items-center rounded text-(--muted) transition-colors duration-150 hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus) disabled:pointer-events-none disabled:opacity-40"
    >
      {children}
    </button>
  );
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    []
  );

  async function copy() {
    if (!value) return;
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => setCopied(false), 1200);
    } catch {
      // Clipboard unavailable (insecure context); nothing actionable to do.
    }
  }

  return (
    <IconAction
      label={copied ? "Copied" : `Copy ${label}`}
      onClick={copy}
      disabled={!value}
    >
      {copied ? (
        <span className="text-(--success)">
          <CheckIcon />
        </span>
      ) : (
        <CopyIcon />
      )}
    </IconAction>
  );
}

// 32 random bytes as URL-safe base64, generated client-side via Web Crypto.
function generateSecret(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
}

function validateUrl(value: string): string | undefined {
  const v = value.trim();
  if (!v) return "API URL is required";
  try {
    new URL(v);
  } catch {
    return "Enter a valid URL, e.g. https://node.example.com:8443";
  }
  return undefined;
}

function errorMessage(error: unknown): string {
  if (
    error &&
    typeof error === "object" &&
    "message" in error &&
    typeof (error as { message: unknown }).message === "string"
  ) {
    return (error as { message: string }).message;
  }
  return "";
}

/* ── Icons ─────────────────────────────────────────────────────────────── */

function iconProps() {
  return {
    viewBox: "0 0 24 24",
    className: "size-3.5",
    fill: "none",
    stroke: "currentColor",
    strokeWidth: 1.75,
    strokeLinecap: "round" as const,
    strokeLinejoin: "round" as const,
    "aria-hidden": true,
  };
}

function CopyIcon() {
  return (
    <svg {...iconProps()}>
      <rect x="9" y="9" width="11" height="11" rx="2" />
      <path d="M5 15V5a2 2 0 0 1 2-2h10" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg {...iconProps()} strokeWidth={2}>
      <path d="M20 6 9 17l-5-5" />
    </svg>
  );
}
