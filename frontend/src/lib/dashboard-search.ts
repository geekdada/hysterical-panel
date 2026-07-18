export type DashboardSearch = {
  sort: string;
};

const ALLOWED_SORTS = new Set([
  "name",
  "-name",
  "today",
  "-today",
  "tx_speed",
  "-tx_speed",
  "rx_speed",
  "-rx_speed",
  "status",
  "-status",
]);

export function parseDashboardSearch(search: Record<string, unknown>): DashboardSearch {
  const raw = typeof search.sort === "string" ? search.sort.trim() : "";
  return { sort: raw && ALLOWED_SORTS.has(raw) ? raw : "name" };
}

export function defaultDashboardSearch(): DashboardSearch {
  return parseDashboardSearch({});
}
