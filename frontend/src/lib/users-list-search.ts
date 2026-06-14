export const USER_LIST_PAGE_SIZE_OPTIONS = [25, 50, 100] as const;

export type UsersListSearch = {
  page: number;
  per_page: number;
  search: string;
  sort: string;
};

export type UsersListContextSearch = {
  listPage?: number;
  listPerPage?: number;
  listSearch?: string;
  listSort?: string;
};

const ALLOWED_SORTS = new Set([
  "created",
  "-created",
  "email",
  "-email",
  "role",
  "-role",
  "status",
  "-status",
  "used_tx",
  "-used_tx",
  "used_rx",
  "-used_rx",
]);

export function clampUsersListPage(raw: unknown): number {
  const n = typeof raw === "number" ? raw : Number(raw);
  if (!Number.isFinite(n) || n < 1) return 1;
  return Math.floor(n);
}

export function clampUsersListPerPage(raw: unknown): number {
  const n = typeof raw === "number" ? raw : Number(raw);
  if (!Number.isFinite(n)) return USER_LIST_PAGE_SIZE_OPTIONS[0];
  let best: (typeof USER_LIST_PAGE_SIZE_OPTIONS)[number] = USER_LIST_PAGE_SIZE_OPTIONS[0];
  let bestDist = Math.abs(n - best);
  for (const allowed of USER_LIST_PAGE_SIZE_OPTIONS) {
    const dist = Math.abs(n - allowed);
    if (dist < bestDist) {
      best = allowed;
      bestDist = dist;
    }
  }
  return best;
}

export function normalizeUsersListSort(raw: unknown): string {
  const sort = typeof raw === "string" ? raw.trim() : "";
  if (sort && ALLOWED_SORTS.has(sort)) return sort;
  return "created";
}

export function parseUsersListSearch(search: Record<string, unknown>): UsersListSearch {
  return {
    page: clampUsersListPage(search.page),
    per_page: clampUsersListPerPage(search.per_page),
    search: typeof search.search === "string" ? search.search : "",
    sort: normalizeUsersListSort(search.sort),
  };
}

export function parseUsersListContextSearch(
  search: Record<string, unknown>
): UsersListContextSearch {
  const ctx: UsersListContextSearch = {};
  if (search.listPage !== undefined) {
    ctx.listPage = clampUsersListPage(search.listPage);
  }
  if (search.listPerPage !== undefined) {
    ctx.listPerPage = clampUsersListPerPage(search.listPerPage);
  }
  if (typeof search.listSearch === "string") {
    ctx.listSearch = search.listSearch;
  }
  if (search.listSort !== undefined) {
    ctx.listSort = normalizeUsersListSort(search.listSort);
  }
  return ctx;
}

export function hasUsersListContext(search: UsersListContextSearch): boolean {
  return (
    search.listPage !== undefined ||
    search.listPerPage !== undefined ||
    search.listSearch !== undefined ||
    search.listSort !== undefined
  );
}

export function usersListSearchFromContext(ctx: UsersListContextSearch): UsersListSearch {
  return {
    page: ctx.listPage ?? 1,
    per_page: ctx.listPerPage ?? USER_LIST_PAGE_SIZE_OPTIONS[0],
    search: ctx.listSearch ?? "",
    sort: ctx.listSort ?? "created",
  };
}

export function defaultUsersListSearch(): UsersListSearch {
  return parseUsersListSearch({});
}

export function usersListContextFromSearch(search: UsersListSearch): UsersListContextSearch {
  return {
    listPage: search.page,
    listPerPage: search.per_page,
    listSearch: search.search,
    listSort: search.sort,
  };
}

export function sortColumnId(sort: string): string {
  return sort.startsWith("-") ? sort.slice(1) : sort;
}

export function isSortDesc(sort: string): boolean {
  return sort.startsWith("-");
}

export function toggleUsersListSort(current: string, columnId: string): string {
  const id = sortColumnId(current);
  if (id !== columnId) return columnId;
  return isSortDesc(current) ? columnId : `-${columnId}`;
}
