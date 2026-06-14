import { useEffect, useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Link, createFileRoute, useNavigate } from "@tanstack/react-router";
import { Button, Input, Label, TextField } from "@heroui/react";
import { requireAdmin } from "~/api/guards";
import type { components } from "~/api/schema";
import {
  canQueryPanelApi,
  fetchUserStats,
  fetchUsersList,
  queryErrorMessage,
  queryKeys,
  REFRESH_MS,
} from "~/api/queries";
import {
  BackLink,
  CopyButton,
  Dot,
  PageShell,
  PanelMessage,
  Section,
  ServerSortableTh,
  TableSkeleton,
  Td,
  Teaching,
  Th,
} from "~/components/ui";
import { UserMenu } from "~/components/user-menu";
import { formatBytes, plural, relTimeFromISO } from "~/lib/format";
import {
  USER_LIST_PAGE_SIZE_OPTIONS,
  parseUsersListSearch,
  toggleUsersListSort,
  type UsersListSearch,
} from "~/lib/users-list-search";

type PanelUser = components["schemas"]["PanelUser"];

export const Route = createFileRoute("/users/")({
  validateSearch: parseUsersListSearch,
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: UsersPage,
});

function UsersPage() {
  const { auth } = Route.useRouteContext();
  const listSearch = Route.useSearch();
  const navigate = useNavigate({ from: Route.fullPath });
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 30_000);
    return () => window.clearInterval(id);
  }, []);

  const usersQuery = useQuery({
    queryKey: queryKeys.usersList(listSearch),
    queryFn: () => fetchUsersList(listSearch),
    enabled: canQueryPanelApi(),
    refetchInterval: REFRESH_MS,
    // Keep the prior page's rows mounted while a new search/sort/page loads, so
    // the table (and the focused search input inside it) is never swapped for
    // the loading skeleton mid-keystroke.
    placeholderData: keepPreviousData,
  });
  const statsQuery = useQuery({
    queryKey: queryKeys.userStats(),
    queryFn: fetchUserStats,
    enabled: canQueryPanelApi(),
    refetchInterval: REFRESH_MS,
  });

  const list = usersQuery.data;
  const users = (list?.items ?? []) as PanelUser[];
  const total = list?.total ?? 0;
  const pageCount = total > 0 ? Math.ceil(total / listSearch.per_page) : 0;
  const listError = usersQuery.error ? queryErrorMessage(usersQuery.error) : "";
  const stats = statsQuery.data;

  const sectionMeta =
    !usersQuery.isPending && !listError
      ? listSearch.search.trim()
        ? `${total} ${plural(Number(total), "match")}`
        : stats
          ? `${stats.total} ${plural(Number(stats.total), "user")} · ${stats.active} active`
          : undefined
      : undefined;

  return (
    <PageShell
      headerLeft={
        <div className="flex min-w-0 items-center gap-3">
          <BackLink />
          <span className="truncate text-[13px] font-semibold tracking-tight">Users</span>
        </div>
      }
      headerRight={auth ? <UserMenu auth={auth} /> : undefined}
    >
      <Section className="mt-0" title="All users" meta={sectionMeta}>
        {usersQuery.isPending ? (
          <TableSkeleton />
        ) : listError ? (
          <PanelMessage>Couldn't load users.</PanelMessage>
        ) : users.length > 0 || listSearch.search.trim() ? (
          <UsersTable
            listSearch={listSearch}
            now={now}
            pageCount={pageCount}
            total={total}
            users={users}
            onSort={(columnId) =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  sort: toggleUsersListSort(prev.sort, columnId),
                  page: 1,
                }),
              })
            }
          />
        ) : (
          <Teaching
            title="No users yet"
            hint="Create a user to issue a Hysteria auth key and track its traffic."
          />
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
  onSort,
}: {
  listSearch: UsersListSearch;
  now: number;
  pageCount: number;
  total: number;
  users: PanelUser[];
  onSort: (columnId: string) => void;
}) {
  const navigate = useNavigate({ from: Route.fullPath });
  const [searchDraft, setSearchDraft] = useState(listSearch.search);

  useEffect(() => {
    setSearchDraft(listSearch.search);
  }, [listSearch.search]);

  useEffect(() => {
    if (searchDraft === listSearch.search) return;
    const id = window.setTimeout(() => {
      navigate({
        search: (prev) => ({
          ...prev,
          search: searchDraft,
          page: 1,
        }),
      });
    }, 300);
    return () => window.clearTimeout(id);
  }, [searchDraft, listSearch.search, navigate]);

  return (
    <div>
      <div className="border-b border-(--border) p-3">
        <TextField
          aria-label="Search users"
          className="max-w-sm"
          value={searchDraft}
          onChange={setSearchDraft}
        >
          <Label className="sr-only">Search users</Label>
          <Input placeholder="Search email, role, status, or full auth key…" />
        </TextField>
      </div>

      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
              <ServerSortableTh columnId="email" sort={listSearch.sort} onSort={onSort}>
                Email
              </ServerSortableTh>
              <Th>Auth key</Th>
              <ServerSortableTh columnId="role" sort={listSearch.sort} onSort={onSort}>
                Role
              </ServerSortableTh>
              <ServerSortableTh
                columnId="used_tx"
                sort={listSearch.sort}
                onSort={onSort}
                align="right"
                className="text-right"
              >
                TX
              </ServerSortableTh>
              <ServerSortableTh
                columnId="used_rx"
                sort={listSearch.sort}
                onSort={onSort}
                align="right"
                className="text-right"
              >
                RX
              </ServerSortableTh>
              <ServerSortableTh
                columnId="status"
                sort={listSearch.sort}
                onSort={onSort}
                align="right"
                className="text-right"
              >
                Status
              </ServerSortableTh>
              <ServerSortableTh
                columnId="created"
                sort={listSearch.sort}
                onSort={onSort}
                align="right"
                className="text-right"
              >
                Created
              </ServerSortableTh>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--separator)">
            {users.length > 0 ? (
              users.map((user) => {
                const active = (user.status ?? "active") === "active";
                return (
                  <tr
                    key={user.id}
                    className={`transition-colors duration-150 hover:bg-(--surface-secondary) ${active ? "" : "opacity-60"}`}
                  >
                    <Td>
                      <div className="flex items-center gap-2.5">
                        <Dot tone={active ? "ok" : "idle"} title={active ? "active" : "disabled"} />
                        <Link
                          to="/users/$userId"
                          params={{ userId: user.id ?? "" }}
                          search={{ from: "users" }}
                          className="block max-w-[200px] truncate rounded-sm font-medium underline-offset-2 hover:text-(--accent) hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
                        >
                          {user.email || "—"}
                        </Link>
                      </div>
                    </Td>
                    <Td>
                      <div className="group/key flex items-center gap-1.5">
                        <span className="block max-w-[200px] truncate font-mono text-xs text-(--muted)">
                          {user.auth_string || "—"}
                        </span>
                        {user.auth_string && (
                          <CopyButton value={user.auth_string} label="auth key" />
                        )}
                      </div>
                    </Td>
                    <Td>
                      <span className="text-xs capitalize text-(--muted)">
                        {user.role ?? "user"}
                      </span>
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      <span className="text-(--muted)">↑</span> {formatBytes(user.used_tx ?? 0)}
                    </Td>
                    <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                      <span className="text-(--muted)">↓</span> {formatBytes(user.used_rx ?? 0)}
                    </Td>
                    <Td className="text-right">
                      <span className="text-xs text-(--muted)">
                        {active ? "Active" : "Disabled"}
                      </span>
                    </Td>
                    <Td className="whitespace-nowrap text-right text-xs text-(--muted)">
                      {user.created ? relTimeFromISO(user.created, now) : "—"}
                    </Td>
                  </tr>
                );
              })
            ) : (
              <tr>
                <td colSpan={7} className="px-3 py-8 text-center text-xs text-(--muted)">
                  No users match your search.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3 border-t border-(--border) px-3 py-2.5">
        <span className="text-xs text-(--muted)">
          {listSearch.search.trim()
            ? `${total} ${plural(Number(total), "match")}`
            : `${total} ${plural(Number(total), "user")}`}
        </span>
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-1.5 text-xs text-(--muted)">
            Rows
            <select
              value={listSearch.per_page}
              onChange={(e) =>
                navigate({
                  search: (prev) => ({
                    ...prev,
                    per_page: Number(e.target.value) as UsersListSearch["per_page"],
                    page: 1,
                  }),
                })
              }
              className="rounded-(--radius) border border-(--border) bg-(--surface) px-1.5 py-0.5 text-[13px] text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
            >
              {USER_LIST_PAGE_SIZE_OPTIONS.map((size) => (
                <option key={size} value={size}>
                  {size}
                </option>
              ))}
            </select>
          </label>
          <span className="text-xs tabular-nums text-(--muted)">
            Page {total === 0 ? 0 : listSearch.page} of {total === 0 ? 0 : pageCount}
          </span>
          <Button
            size="sm"
            variant="secondary"
            isDisabled={listSearch.page <= 1}
            onPress={() =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  page: Math.max(1, prev.page - 1),
                }),
              })
            }
          >
            Previous
          </Button>
          <Button
            size="sm"
            variant="secondary"
            isDisabled={listSearch.page >= pageCount || total === 0}
            onPress={() =>
              navigate({
                search: (prev) => ({
                  ...prev,
                  page: prev.page + 1,
                }),
              })
            }
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  );
}
