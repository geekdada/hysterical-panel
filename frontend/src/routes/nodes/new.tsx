import { useEffect, useRef, useState, type ReactNode } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { createFileRoute, useNavigate } from "@tanstack/react-router";
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
import { Check, Copy } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import { createNode, queryErrorMessage, queryKeys, testNode } from "~/api/queries";
import { BackLink, ErrorAlert, PageShell } from "~/components/ui";
import { usePanelApiOrigin } from "~/lib/use-panel-api-origin";
import * as m from "~/paraglide/messages.js";

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
  const queryClient = useQueryClient();
  const [created, setCreated] = useState<Node | null>(null);
  const testMutation = useMutation({ mutationFn: testNode });
  const createMutation = useMutation({
    mutationFn: async (body: components["schemas"]["NodeCreateRequest"]) => {
      const node = await createNode(body);
      if (!node.id) {
        throw new Error(m.error_node_create());
      }
      return node as Node & { id: string };
    },
    onSuccess: (node) => {
      setCreated(node);
      void queryClient.invalidateQueries({
        queryKey: queryKeys.dashboardBase(),
      });
      testMutation.mutate(node.id);
    },
  });

  const form = useForm({
    defaultValues: {
      name: "",
      api_url: "",
      api_secret: "",
      poll_interval: 30,
      enabled: true,
    },
    onSubmit: async ({ value }) => {
      createMutation.reset();
      testMutation.reset();
      try {
        await createMutation.mutateAsync({
          name: value.name.trim(),
          api_url: value.api_url.trim(),
          api_secret: value.api_secret,
          poll_interval: value.poll_interval,
          enabled: value.enabled,
        });
      } catch {
        // The mutation owns rendering the submission error.
      }
    },
  });

  function addAnother() {
    form.reset();
    setCreated(null);
    createMutation.reset();
    testMutation.reset();
  }

  const test = toTestState(testMutation);
  const submitError = createMutation.error
    ? queryErrorMessage(createMutation.error, m.error_node_create_network())
    : "";

  return (
    <PageShell width="narrow" headerLeft={<BackLink />}>
      <div className="mb-5">
        <h1 className="text-base font-semibold tracking-tight">{m.nodes_add_title()}</h1>
        <p className="mt-0.5 text-[13px] text-(--muted)">{m.nodes_add_description()}</p>
      </div>

      <div className="rounded-(--radius) border border-(--border) bg-(--surface) p-5">
        {created ? (
          <CreatedView
            node={created}
            test={test}
            onRetry={() => {
              if (created.id) testMutation.mutate(created.id);
            }}
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
              <input type="text" name="username" tabIndex={-1} autoComplete="username" />
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
                    ? m.nodes_add_name_required()
                    : value.trim().length > 128
                      ? m.nodes_add_name_too_long()
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
                  <Label>{m.nodes_add_name_label()}</Label>
                  <Input
                    placeholder={m.nodes_add_name_placeholder()}
                    autoFocus
                    autoComplete="off"
                    data-1p-ignore
                    data-lpignore="true"
                    data-form-type="other"
                  />
                  <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
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
                  <Label>{m.nodes_add_api_url_label()}</Label>
                  <Input
                    type="url"
                    inputMode="url"
                    placeholder={m.nodes_add_api_url_placeholder()}
                    className="font-mono text-[13px]"
                    autoComplete="url"
                    pattern="https?://.*"
                    data-1p-ignore
                    data-lpignore="true"
                    data-form-type="other"
                  />
                  {field.state.meta.errors.length > 0 ? (
                    <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                  ) : (
                    <Description>{m.nodes_add_api_url_description()}</Description>
                  )}
                </TextField>
              )}
            </form.Field>

            <form.Field
              name="api_secret"
              validators={{
                onChange: ({ value }) => (!value ? m.nodes_add_api_secret_required() : undefined),
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
                    <Label>{m.nodes_add_api_secret_label()}</Label>
                    <button
                      type="button"
                      onClick={() => {
                        field.handleChange(generateSecret());
                      }}
                      className="rounded text-xs font-medium text-(--accent) transition-opacity duration-150 hover:opacity-80 focus-visible:underline focus-visible:outline-none"
                    >
                      {m.nodes_add_api_secret_generate()}
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
                      <CopyButton value={field.state.value} label={m.nodes_add_copy_api_secret()} />
                    </div>
                  </div>
                  {field.state.meta.errors.length > 0 ? (
                    <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                  ) : (
                    <Description>{m.nodes_add_api_secret_description()}</Description>
                  )}
                </TextField>
              )}
            </form.Field>

            <form.Field
              name="poll_interval"
              validators={{
                onChange: ({ value }) =>
                  value == null || Number.isNaN(value)
                    ? m.nodes_add_poll_interval_required()
                    : !Number.isInteger(value) || value < 1
                      ? m.nodes_add_poll_interval_invalid()
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
                  <Label>{m.nodes_add_poll_interval_label()}</Label>
                  <NumberField.Group>
                    <NumberField.DecrementButton />
                    <NumberField.Input />
                    <NumberField.IncrementButton />
                  </NumberField.Group>
                  {field.state.meta.errors.length > 0 ? (
                    <FieldError>{field.state.meta.errors.join(", ")}</FieldError>
                  ) : (
                    <Description>{m.nodes_add_poll_interval_description()}</Description>
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
                    <Label>{m.nodes_add_enabled_label()}</Label>
                    <Description>{m.nodes_add_enabled_description()}</Description>
                  </Switch.Content>
                  <Switch.Control>
                    <Switch.Thumb />
                  </Switch.Control>
                </Switch>
              )}
            </form.Field>

            <ErrorAlert message={submitError} icon />

            <div className="flex items-center justify-end gap-2 border-t border-(--separator) pt-4">
              <Button variant="ghost" onPress={() => navigate({ to: "/" })}>
                {m.common_cancel()}
              </Button>
              <form.Subscribe
                selector={(s) => ({
                  canSubmit: s.canSubmit,
                  isSubmitting: s.isSubmitting,
                })}
              >
                {({ canSubmit, isSubmitting }) => (
                  <Button type="submit" variant="primary" isDisabled={!canSubmit}>
                    {isSubmitting ? m.nodes_add_submitting() : m.nodes_add_submit()}
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
    </PageShell>
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
      note: m.nodes_setup_yaml_note_listen(),
    },
    {
      code: `  secret: ${secret}`,
      note: m.nodes_setup_yaml_note_secret(),
    },
  ];
}

const PANEL_URL_PLACEHOLDER = "<panel-base-url>";

function authYaml(panelOrigin: string): { code: string; note?: string }[] {
  return [
    { code: "auth:" },
    { code: "  type: http" },
    { code: "  http:" },
    {
      code: `    url: ${panelOrigin}/api/hysteria/auth`,
      note: m.nodes_setup_yaml_note_panel(),
    },
    {
      code: "    insecure: false",
      note: m.nodes_setup_yaml_note_insecure(),
    },
  ];
}

function CodeBlock({ lines, label }: { lines: { code: string; note?: string }[]; label: string }) {
  const codeText = lines.map((l) => l.code).join("\n");
  return (
    <div className="relative mt-3">
      <pre className="overflow-x-auto rounded-(--radius) border border-(--border) bg-(--surface-secondary) p-3 pr-10 font-mono text-xs leading-relaxed">
        {lines.map((line) => (
          <div key={line.code}>
            <span className="text-(--foreground)">{line.code}</span>
            {line.note && <span className="text-(--muted)">{`  # ${line.note}`}</span>}
          </div>
        ))}
      </pre>
      <div className="absolute right-1 top-1">
        <CopyButton value={codeText} label={label} />
      </div>
    </div>
  );
}

function ServerSetup({ apiSecret }: { apiSecret: string }) {
  const resolvedOrigin = usePanelApiOrigin();
  const panelOrigin = resolvedOrigin || PANEL_URL_PLACEHOLDER;

  return (
    <section className="mt-4 rounded-(--radius) border border-(--border) bg-(--surface) p-5">
      <h2 className="text-[13px] font-semibold tracking-tight">{m.nodes_setup_title()}</h2>
      <p className="mt-1 max-w-prose text-[13px] text-(--muted)">{m.nodes_setup_intro()}</p>

      <CodeBlock lines={setupYaml(apiSecret)} label={m.common_copy_config()} />

      <h3 className="mt-5 text-[13px] font-semibold tracking-tight">
        {m.nodes_setup_auth_title()}
      </h3>
      <p className="mt-1 max-w-prose text-[13px] text-(--muted)">{m.nodes_setup_auth_intro()}</p>

      <CodeBlock lines={authYaml(panelOrigin)} label={m.common_copy_config()} />
      <p className="mt-1.5 text-xs text-(--muted)">{m.nodes_setup_host_note()}</p>

      <dl className="mt-4 flex flex-col gap-1.5 text-[13px]">
        <SetupRow term={m.nodes_setup_term_api_url()}>{m.nodes_setup_api_url_value()}</SetupRow>
        <SetupRow term={m.nodes_setup_term_api_secret()}>
          {m.nodes_setup_api_secret_value()}
        </SetupRow>
        <SetupRow term={m.nodes_setup_term_auth_check()}>
          {m.nodes_setup_auth_check_value()}
        </SetupRow>
        <SetupRow term={m.nodes_setup_term_reachability()}>
          {m.nodes_setup_reachability_value()}
        </SetupRow>
      </dl>
    </section>
  );
}

function SetupRow({ term, children }: { term: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:gap-2">
      <dt className="shrink-0 font-medium text-(--foreground) sm:w-28">{term}</dt>
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
          <p className="text-[13px] font-medium">{m.nodes_created_title()}</p>
          <p className="mt-0.5 text-xs text-(--muted)">
            {m.nodes_created_registered({ name: node.name ?? "" })}
          </p>
        </div>
      </div>

      <div className="rounded-(--radius) border border-(--border) bg-(--surface-secondary) px-3 py-2.5 text-[13px]">
        {test.status === "pending" && (
          <div className="flex items-center gap-2 text-(--muted)">
            <span className="inline-block size-2 shrink-0 animate-pulse rounded-full bg-(--muted)" />
            {m.nodes_created_testing()}
          </div>
        )}
        {test.status === "ok" && (
          <div className="flex items-center gap-2">
            <StatusDot tone="ok" />
            <span>{m.nodes_created_reachable()}</span>
            <span className="ml-auto font-mono tabular-nums text-(--muted)">
              {m.nodes_created_latency_ms({ ms: test.latencyMs })}
            </span>
          </div>
        )}
        {test.status === "error" && (
          <div className="flex flex-col gap-1.5">
            <div className="flex items-center gap-2">
              <StatusDot tone="error" />
              <span className="block truncate text-(--danger)" title={test.message}>
                {test.message}
              </span>
              <button
                type="button"
                onClick={onRetry}
                className="ml-auto shrink-0 text-xs text-(--muted) underline-offset-2 transition-colors duration-150 hover:text-(--foreground) hover:underline"
              >
                {m.common_retry()}
              </button>
            </div>
            <p className="text-xs text-(--muted)">{m.nodes_created_saved_note()}</p>
          </div>
        )}
      </div>

      <div className="flex items-center justify-end gap-2 border-t border-(--separator) pt-4">
        <Button variant="ghost" onPress={onAddAnother}>
          {m.nodes_created_add_another()}
        </Button>
        <Button variant="primary" onPress={onDone}>
          {m.common_back_dashboard()}
        </Button>
      </div>
    </div>
  );
}

/* ── Helpers ───────────────────────────────────────────────────────────── */

function StatusDot({ tone, className = "" }: { tone: "ok" | "error"; className?: string }) {
  const fill = tone === "ok" ? "bg-(--success)" : "bg-(--danger)";
  return <span className={`inline-block size-2 shrink-0 rounded-full ${fill} ${className}`} />;
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
      label={copied ? m.common_copy_copied() : m.common_copy_copy({ label })}
      onClick={copy}
      disabled={!value}
    >
      {copied ? (
        <span className="text-(--success)">
          <Check className="size-3.5" aria-hidden />
        </span>
      ) : (
        <Copy className="size-3.5" aria-hidden />
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
  return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function validateUrl(value: string): string | undefined {
  const v = value.trim();
  if (!v) return m.nodes_add_api_url_required();
  try {
    new URL(v);
  } catch {
    return m.nodes_add_api_url_invalid();
  }
  return undefined;
}

function toTestState(mutation: {
  data?: components["schemas"]["NodeTestResponse"];
  error: unknown;
  isPending: boolean;
}): TestState {
  if (mutation.isPending) {
    return { status: "pending" };
  }
  if (mutation.error) {
    return {
      status: "error",
      message: queryErrorMessage(mutation.error, m.error_node_test_network()),
    };
  }
  if (!mutation.data) {
    return { status: "pending" };
  }
  if (mutation.data.ok) {
    return { status: "ok", latencyMs: mutation.data.latency_ms ?? 0 };
  }
  return {
    status: "error",
    message: mutation.data.error || m.nodes_created_unreachable(),
  };
}
