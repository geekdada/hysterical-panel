import { useEffect, useMemo, useState, type FormEvent } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Input, Label, Modal, TextField, Tooltip } from "@heroui/react";
import { CircleInfo } from "@gravity-ui/icons";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  createUser,
  queryErrorMessage,
  queryKeys,
  userStatsQueryOptions,
  usersListQueryOptions,
  updateUserStatus,
} from "~/api/queries";
import { markResponsePrivate } from "~/api/ssr";
import {
  BrandLink,
  CopyButton,
  Dot,
  ErrorAlert,
  PageShell,
  PanelMessage,
  Section,
  SortableTh,
  TableSkeleton,
  Td,
  Teaching,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { formatBytes, relTimeFromISO } from "~/lib/format";
import { useActiveTimeZone } from "~/lib/use-timezone";
import { useHydratedNow } from "~/lib/use-hydrated-now";
import {
  USER_LIST_PAGE_SIZE_OPTIONS,
  isSortDesc,
  parseUsersListSearch,
  sortColumnId,
  type UsersListSearch,
} from "~/lib/users-list-search";
import * as m from "~/paraglide/messages.js";

type PanelUser = components["schemas"]["PanelUser"];

export const Route = createFileRoute("/users/")({
  validateSearch: parseUsersListSearch,
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  loaderDeps: ({ search }) => search,
  loader: async ({ context, deps }) => {
    markResponsePrivate();
    await Promise.allSettled([
      context.queryClient.ensureQueryData(usersListQueryOptions(deps)),
      context.queryClient.ensureQueryData(userStatsQueryOptions()),
    ]);
  },
  component: UsersPage,
});

function UsersPage() {
  const { auth } = Route.useRouteContext();
  const listSearch = Route.useSearch();
  const now = useHydratedNow(30_000);

  const usersQuery = useQuery({
    ...usersListQueryOptions(listSearch),
    // Keep the prior page's rows mounted while a new search/sort/page loads, so
    // the table (and the focused search input inside it) is never swapped for
    // the loading skeleton mid-keystroke.
    placeholderData: keepPreviousData,
  });
  const statsQuery = useQuery(userStatsQueryOptions());

  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);

  const invalidateUsers = () => {
    void queryClient.invalidateQueries({ queryKey: [...queryKeys.all, "users"] });
    void queryClient.invalidateQueries({ queryKey: queryKeys.userStats() });
  };

  const createMutation = useMutation({ mutationFn: createUser, onSuccess: invalidateUsers });
  const toggleMutation = useMutation({
    mutationFn: ({ id, status }: { id: string; status: "active" | "disabled" }) =>
      updateUserStatus(id, status),
    onSuccess: invalidateUsers,
  });

  function handleToggleStatus(user: PanelUser) {
    if (!user.id) return;
    const next = (user.status ?? "active") === "active" ? "disabled" : "active";
    toggleMutation.mutate({ id: user.id, status: next });
  }

  // Reset the create mutation when the modal closes so a stale success/error
  // card never flashes the next time it opens.
  function handleCreateOpenChange(open: boolean) {
    setCreateOpen(open);
    if (!open) createMutation.reset();
  }

  const toggleError = toggleMutation.error
    ? queryErrorMessage(toggleMutation.error, m.error_user_update_network())
    : "";

  const list = usersQuery.data;
  const users = (list?.items ?? []) as PanelUser[];
  const total = list?.total ?? 0;
  const pageCount = total > 0 ? Math.ceil(total / listSearch.per_page) : 0;
  const listError = usersQuery.error ? queryErrorMessage(usersQuery.error) : "";
  const stats = statsQuery.data;

  const sectionMeta =
    !usersQuery.isPending && !listError
      ? listSearch.search.trim()
        ? m.users_meta_matches({ count: String(total) })
        : stats
          ? m.users_meta_stats({
              total: String(stats.total),
              active: String(stats.active),
            })
          : undefined
      : undefined;

  return (
    <PageShell headerLeft={<BrandLink />} headerRight={auth ? <UserMenu auth={auth} /> : undefined}>
      <ErrorAlert message={toggleError} className="mb-4" />

      <CreateUserModal
        isOpen={createOpen}
        onOpenChange={handleCreateOpenChange}
        pending={createMutation.isPending}
        error={
          createMutation.error
            ? queryErrorMessage(createMutation.error, m.error_user_create_network())
            : ""
        }
        created={createMutation.data ?? null}
        onSubmit={(email) => createMutation.mutate({ email })}
      />

      <Section
        className="mt-0"
        title={m.users_section_all()}
        meta={sectionMeta}
        action={
          <div className="flex sm:justify-end">
            <Button size="sm" variant="secondary" onPress={() => setCreateOpen(true)}>
              {m.users_new_button()}
            </Button>
          </div>
        }
      >
        {usersQuery.isPending ? (
          <TableSkeleton />
        ) : listError ? (
          <PanelMessage>{m.users_load_error()}</PanelMessage>
        ) : users.length > 0 || listSearch.search.trim() ? (
          <UsersTable
            listSearch={listSearch}
            now={now}
            pageCount={pageCount}
            total={total}
            users={users}
            currentUserId={auth?.user.id}
            togglingId={toggleMutation.isPending ? (toggleMutation.variables?.id ?? null) : null}
            onToggleStatus={handleToggleStatus}
          />
        ) : (
          <Teaching title={m.users_empty_title()} hint={m.users_empty_hint()} />
        )}
      </Section>
    </PageShell>
  );
}

function UsersTable({
  listSearch,
  now,
  pageCount,
  total,
  users,
  currentUserId,
  togglingId,
  onToggleStatus,
}: {
  listSearch: UsersListSearch;
  now: number | null;
  pageCount: number;
  total: number;
  users: PanelUser[];
  currentUserId?: string;
  togglingId: string | null;
  onToggleStatus: (user: PanelUser) => void;
}) {
  const routerNavigate = useNavigate({ from: Route.fullPath });
  const updateSearch: typeof routerNavigate = (options) =>
    routerNavigate({ ...options, replace: true });
  const tz = useActiveTimeZone();
  const [searchDraft, setSearchDraft] = useState(listSearch.search);
  const sorting = useMemo<SortingState>(
    () => [{ id: sortColumnId(listSearch.sort), desc: isSortDesc(listSearch.sort) }],
    [listSearch.sort]
  );
  const columns = useMemo<ColumnDef<PanelUser>[]>(
    () => [
      { accessorFn: (user) => user.email ?? "", id: "email", sortDescFirst: false },
      { accessorFn: (user) => user.role ?? "user", id: "role", sortDescFirst: false },
      { accessorFn: (user) => user.used_tx ?? 0, id: "used_tx", sortDescFirst: false },
      { accessorFn: (user) => user.used_rx ?? 0, id: "used_rx", sortDescFirst: false },
      {
        accessorFn: (user) => user.last_connected_at ?? "",
        id: "last_connected_at",
        sortDescFirst: false,
      },
      { accessorFn: (user) => user.created ?? "", id: "created", sortDescFirst: false },
      { id: "actions", enableSorting: false },
    ],
    []
  );
  const table = useReactTable({
    columns,
    data: users,
    enableMultiSort: false,
    enableSortingRemoval: false,
    manualSorting: true,
    getCoreRowModel: getCoreRowModel(),
    onSortingChange: (updater) => {
      const next = typeof updater === "function" ? updater(sorting) : updater;
      const first = next[0];
      if (!first) return;
      updateSearch({
        search: (prev) => ({
          ...prev,
          sort: first.desc ? `-${first.id}` : first.id,
          page: 1,
        }),
      });
    },
    state: { sorting },
  });
  const tableRows = table.getRowModel().rows;

  useEffect(() => {
    setSearchDraft(listSearch.search);
  }, [listSearch.search]);

  useEffect(() => {
    if (searchDraft === listSearch.search) return;
    const id = window.setTimeout(() => {
      updateSearch({
        search: (prev) => ({
          ...prev,
          search: searchDraft,
          page: 1,
        }),
      });
    }, 300);
    return () => window.clearTimeout(id);
  }, [searchDraft, listSearch.search, updateSearch]);

  return (
    <div>
      <div className="flex items-center gap-2 border-b border-border p-3">
        <TextField
          aria-label={m.users_search_aria()}
          className="max-w-sm"
          value={searchDraft}
          onChange={setSearchDraft}
        >
          <Label className="sr-only">{m.users_search_aria()}</Label>
          <Input placeholder={m.users_search_placeholder()} />
        </TextField>
        <Tooltip delay={0}>
          <Button isIconOnly variant="tertiary" aria-label={m.users_search_hint()}>
            <CircleInfo className="size-4" aria-hidden />
          </Button>
          <Tooltip.Content className="max-w-xs">
            <p>{m.users_search_hint()}</p>
          </Tooltip.Content>
        </Tooltip>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-b border-border bg-surface-secondary text-left">
              <SortableTh column={table.getColumn("email")!}>{m.common_email()}</SortableTh>
              <SortableTh column={table.getColumn("role")!}>{m.common_role()}</SortableTh>
              <SortableTh column={table.getColumn("used_tx")!} align="right" className="text-right">
                {m.common_th_tx()}
              </SortableTh>
              <SortableTh column={table.getColumn("used_rx")!} align="right" className="text-right">
                {m.common_th_rx()}
              </SortableTh>
              <SortableTh
                column={table.getColumn("last_connected_at")!}
                align="right"
                className="text-right"
              >
                {m.users_th_last_connect()}
              </SortableTh>
              <SortableTh column={table.getColumn("created")!} align="right" className="text-right">
                {m.users_th_created()}
              </SortableTh>
              <Th>{m.common_actions()}</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-separator">
            {tableRows.length > 0 ? (
              tableRows.map((row) => {
                const user = row.original;
                const active = (user.status ?? "active") === "active";
                const isSelf = Boolean(currentUserId && user.id === currentUserId);
                return (
                  <tr
                    key={user.id}
                    className="transition-colors duration-150 hover:bg-surface-secondary"
                  >
                    <Td>
                      <div className="flex items-center gap-2.5">
                        <Dot
                          tone={active ? "ok" : "idle"}
                          title={active ? m.common_status_active() : m.common_status_disabled()}
                        />
                        <Link
                          to="/users/$userId"
                          params={{ userId: user.id ?? "" }}
                          className="block max-w-[200px] truncate rounded-sm font-medium underline-offset-2 hover:text-accent hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
                        >
                          {user.email || m.common_em_dash()}
                        </Link>
                      </div>
                    </Td>
                    <Td>
                      <span className="text-xs capitalize text-muted">{user.role ?? "user"}</span>
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      <span className="text-muted">↑</span> {formatBytes(user.used_tx ?? 0)}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      <span className="text-muted">↓</span> {formatBytes(user.used_rx ?? 0)}
                    </Td>
                    <Td className="whitespace-nowrap text-right text-xs text-muted">
                      <span
                        title={
                          user.last_connected_at
                            ? new Date(user.last_connected_at).toLocaleString(undefined, {
                                timeZone: tz,
                              })
                            : undefined
                        }
                      >
                        {user.last_connected_at
                          ? relTimeFromISO(user.last_connected_at, now)
                          : m.common_never()}
                      </span>
                    </Td>
                    <Td className="whitespace-nowrap text-right text-xs text-muted">
                      {user.created ? relTimeFromISO(user.created, now) : m.common_em_dash()}
                    </Td>
                    <Td>
                      <Button
                        size="sm"
                        variant={active ? "danger-soft" : "primary"}
                        isPending={togglingId === user.id}
                        isDisabled={isSelf}
                        onPress={() => onToggleStatus(user)}
                      >
                        {active ? m.users_deactivate() : m.users_activate()}
                      </Button>
                    </Td>
                  </tr>
                );
              })
            ) : (
              <tr>
                <td colSpan={columns.length} className="px-3 py-8 text-center text-xs text-muted">
                  {m.users_no_search_results()}
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-border px-3 py-2.5">
        <span className="text-xs text-muted">
          {listSearch.search.trim()
            ? m.users_footer_count_matches({ count: String(total) })
            : m.users_footer_count_users({ count: String(total) })}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-1.5 text-xs text-muted">
            {m.users_pagination_rows()}
            <select
              value={listSearch.per_page}
              onChange={(e) =>
                updateSearch({
                  search: (prev) => ({
                    ...prev,
                    per_page: Number(e.target.value) as UsersListSearch["per_page"],
                    page: 1,
                  }),
                })
              }
              className="rounded-lg border bg-surface px-1.5 py-0.5 text-[13px] text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-focus"
            >
              {USER_LIST_PAGE_SIZE_OPTIONS.map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
          </label>
          <span className="text-xs tabular-nums text-muted">
            {m.users_pagination_page({
              page: String(total === 0 ? 0 : listSearch.page),
              pageCount: String(total === 0 ? 0 : pageCount),
            })}
          </span>
          <Button
            size="sm"
            variant="secondary"
            isDisabled={listSearch.page <= 1}
            onPress={() =>
              updateSearch({
                search: (prev) => ({
                  ...prev,
                  page: Math.max(1, prev.page - 1),
                }),
              })
            }
          >
            {m.users_pagination_previous()}
          </Button>
          <Button
            size="sm"
            variant="secondary"
            isDisabled={listSearch.page >= pageCount || total === 0}
            onPress={() =>
              updateSearch({
                search: (prev) => ({
                  ...prev,
                  page: prev.page + 1,
                }),
              })
            }
          >
            {m.users_pagination_next()}
          </Button>
        </div>
      </div>
    </div>
  );
}

function CreateUserModal({
  isOpen,
  onOpenChange,
  pending,
  error,
  created,
  onSubmit,
}: {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  error: string;
  created: PanelUser | null;
  onSubmit: (email: string) => void;
}) {
  const [email, setEmail] = useState("");

  // Start each open with an empty field; the parent resets the mutation on close.
  useEffect(() => {
    if (!isOpen) setEmail("");
  }, [isOpen]);

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmed = email.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
    setEmail("");
  }

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm" placement="auto">
        <Modal.Dialog>
          <Modal.CloseTrigger />
          <form onSubmit={handleSubmit}>
            <Modal.Header>
              <Modal.Heading>{m.users_create_title()}</Modal.Heading>
              <p className="mt-1.5 text-sm leading-5 text-muted">{m.users_create_description()}</p>
            </Modal.Header>
            <Modal.Body>
              <TextField value={email} onChange={setEmail} autoFocus>
                <Label>{m.users_create_email_label()}</Label>
                <Input
                  type="email"
                  autoComplete="off"
                  placeholder={m.users_create_email_placeholder()}
                />
              </TextField>

              {error && (
                <p className="mt-3 text-[13px] text-danger" role="alert">
                  {error}
                </p>
              )}

              {created && (
                <div className="mt-4 rounded-lg border bg-surface-secondary p-3">
                  <div className="flex items-center gap-2 text-[13px]">
                    <Dot tone="ok" />
                    <span className="font-medium">{m.users_created()}</span>
                    <span className="min-w-0 truncate text-xs text-muted">{created.email}</span>
                  </div>
                  <p className="mt-1 text-xs text-muted">{m.users_created_auth_key_hint()}</p>
                  {created.auth_string && (
                    <div className="group/key mt-2 flex items-center gap-1.5">
                      <span className="min-w-0 truncate font-mono text-[12px] text-muted">
                        {created.auth_string}
                      </span>
                      <CopyButton value={created.auth_string} label={m.common_copy_auth_key()} />
                    </div>
                  )}
                </div>
              )}
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {m.users_create_cancel()}
              </Button>
              <Button
                type="submit"
                variant="primary"
                isPending={pending}
                isDisabled={pending || email.trim().length === 0}
              >
                {pending ? m.users_create_submitting() : m.users_create_submit()}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  );
}
