import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { Link, createFileRoute, useNavigate } from '@tanstack/react-router'
import { Button } from '@heroui/react'
import { apiClient } from '~/api/client'
import { requireAdmin } from '~/api/guards'
import type { components } from '~/api/schema'
import {
  GranularityToggle,
  RANGE_MS,
  TrafficChart,
  toPbDateTime,
  type Granularity,
} from '~/components/traffic'
import {
  CopyButton,
  Dot,
  PanelMessage,
  Section,
  TableSkeleton,
  Td,
  Teaching,
  Th,
} from '~/components/ui'
import { UserMenu } from '~/components/user-menu'
import {
  formatBytes,
  formatDuration,
  relTime,
  relTimeFromISO,
} from '~/lib/format'

type Node = components['schemas']['Node']
type TrafficSeries = components['schemas']['TrafficSeriesResponse']
type NodeTrafficSummary = components['schemas']['NodeTrafficSummaryResponse']
type NodeLive = components['schemas']['NodeLiveResponse']

const REFRESH_MS = 20_000

export const Route = createFileRoute('/nodes/$nodeId')({
  beforeLoad: ({ context }) => requireAdmin(context.auth),
  component: NodeDetailPage,
})

function NodeDetailPage() {
  const { nodeId } = Route.useParams()
  const { auth } = Route.useRouteContext()
  const navigate = useNavigate()

  const [node, setNode] = useState<Node | null>(null)
  const [summary, setSummary] = useState<NodeTrafficSummary | null>(null)
  const [series, setSeries] = useState<TrafficSeries | null>(null)
  const [granularity, setGranularity] = useState<Granularity>('daily')
  const [loading, setLoading] = useState(true)
  const [notFound, setNotFound] = useState(false)
  const [error, setError] = useState('')
  const [updatedAt, setUpdatedAt] = useState<number | null>(null)
  const [now, setNow] = useState(() => Date.now())

  const fetchData = useCallback(async () => {
    try {
      const to = toPbDateTime(new Date(Date.now()))
      const from = toPbDateTime(new Date(Date.now() - RANGE_MS[granularity]))
      const [nodeRes, summaryRes, seriesRes] = await Promise.all([
        apiClient.GET('/api/panel/nodes/{id}', {
          params: { path: { id: nodeId } },
        }),
        apiClient.GET('/api/panel/nodes/{id}/traffic/summary', {
          params: { path: { id: nodeId } },
        }),
        apiClient.GET('/api/panel/nodes/{id}/traffic/series', {
          params: { path: { id: nodeId }, query: { granularity, from, to } },
        }),
      ])
      if (nodeRes.response.status === 404) {
        setNotFound(true)
        return
      }
      if (nodeRes.error || summaryRes.error || seriesRes.error) {
        setError("Couldn't reach the panel API.")
        return
      }
      setNode(nodeRes.data ?? null)
      setSummary(summaryRes.data ?? null)
      setSeries(seriesRes.data ?? null)
      setError('')
      setUpdatedAt(Date.now())
    } catch {
      setError('Network error. Retrying on the next refresh.')
    } finally {
      setLoading(false)
    }
  }, [nodeId, granularity])

  useEffect(() => {
    fetchData()
    const id = setInterval(fetchData, REFRESH_MS)
    return () => clearInterval(id)
  }, [fetchData])

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 5_000)
    return () => clearInterval(id)
  }, [])

  const health = node?.health ?? 'never'
  const enabled = node?.enabled ?? false
  const tone = !enabled
    ? 'idle'
    : health === 'ok'
      ? 'ok'
      : health === 'error'
        ? 'error'
        : 'idle'

  return (
    <div className="min-h-svh bg-(--background) text-(--foreground)">
      <header className="sticky top-0 z-20 border-b border-(--border) bg-(--surface)">
        <div className="mx-auto flex h-12 max-w-7xl items-center justify-between px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-2.5">
            <Link
              to="/"
              aria-label="Back to dashboard"
              className="grid size-6 shrink-0 place-items-center rounded text-(--muted) transition-colors duration-150 hover:bg-(--surface-secondary) hover:text-(--foreground) focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-(--focus)"
            >
              <BackIcon />
            </Link>
            {loading && !node ? (
              <span className="h-3.5 w-32 animate-pulse rounded bg-(--surface-secondary)" />
            ) : (
              <div className="flex min-w-0 items-center gap-2">
                <Dot tone={tone} title={enabled ? health : 'disabled'} />
                <span className="truncate text-[13px] font-semibold tracking-tight">
                  {node?.name || 'Node'}
                </span>
              </div>
            )}
          </div>
          <div className="flex items-center gap-3 text-xs text-(--muted)">
            {updatedAt !== null && (
              <span
                className="hidden tabular-nums sm:inline"
                title={new Date(updatedAt).toLocaleString()}
              >
                Updated {relTime(updatedAt, now)}
              </span>
            )}
            <span className="hidden h-3.5 w-px bg-(--border) sm:block" />
            {auth && <UserMenu auth={auth} />}
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-5 sm:px-6">
        {error && (
          <div
            className="mb-4 flex items-center gap-2 rounded-(--radius) border border-(--border) bg-(--danger-soft) px-3 py-2 text-[13px] text-(--danger-soft-foreground)"
            role="alert"
          >
            <Dot tone="error" />
            <span>{error}</span>
          </div>
        )}

        {notFound ? (
          <Teaching
            title="Node not found"
            hint="It may have been deleted. Head back to the dashboard to see the current fleet."
            action={
              <Button
                size="sm"
                variant="secondary"
                onPress={() => navigate({ to: '/' })}
              >
                Back to dashboard
              </Button>
            }
          />
        ) : (
          <>
            <DetailRail node={node} loading={loading && !node} now={now} />

            <TrafficSection
              loading={loading && !series}
              granularity={granularity}
              onGranularityChange={setGranularity}
              series={series}
              summary={summary}
            />

            <StreamsSection nodeId={nodeId} />
          </>
        )}
      </main>
    </div>
  )
}

/* ── Detail rail (config + health) ─────────────────────────────────────── */

function DetailRail({
  node,
  loading,
  now,
}: {
  node: Node | null
  loading: boolean
  now: number
}) {
  if (loading) {
    return (
      <div className="flex flex-col divide-y divide-(--border) rounded-(--radius) border border-(--border) bg-(--surface) sm:flex-row sm:divide-x sm:divide-y-0">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="flex-1 px-4 py-3">
            <div className="h-3 w-16 animate-pulse rounded bg-(--surface-secondary)" />
            <div className="mt-2 h-4 w-28 animate-pulse rounded bg-(--surface-secondary)" />
          </div>
        ))}
      </div>
    )
  }

  const enabled = node?.enabled ?? false
  const health = node?.health ?? 'never'
  const stateLabel = !enabled
    ? 'Disabled'
    : health === 'error'
      ? node?.last_error || 'Error'
      : health === 'ok'
        ? 'Healthy'
        : 'Never polled'
  const stateTone = !enabled ? 'muted' : health === 'error' ? 'danger' : 'muted'

  return (
    <div className="flex flex-col divide-y divide-(--border) rounded-(--radius) border border-(--border) bg-(--surface) sm:flex-row sm:divide-x sm:divide-y-0">
      <RailItem label="Endpoint" className="sm:flex-[2]">
        <div className="group/key flex items-center gap-1.5">
          <span className="block truncate font-mono text-[13px] text-(--foreground)">
            {node?.api_url || '—'}
          </span>
          {node?.api_url && (
            <CopyButton value={node.api_url} label="endpoint" />
          )}
        </div>
      </RailItem>
      <RailItem label="Poll interval">
        <span className="font-mono text-[13px] tabular-nums">
          {node?.poll_interval ? `${node.poll_interval}s` : '—'}
        </span>
      </RailItem>
      <RailItem label="Last poll">
        <span className="font-mono text-[13px] tabular-nums">
          {node?.last_polled_at
            ? relTimeFromISO(node.last_polled_at, now)
            : '—'}
        </span>
      </RailItem>
      <RailItem label="State">
        <span
          className={`block truncate text-[13px] ${stateTone === 'danger' ? 'text-(--danger)' : 'text-(--foreground)'}`}
          title={stateLabel}
        >
          {stateLabel}
        </span>
      </RailItem>
    </div>
  )
}

function RailItem({
  label,
  children,
  className = '',
}: {
  label: string
  children: ReactNode
  className?: string
}) {
  return (
    <div className={`min-w-0 flex-1 px-4 py-3 ${className}`}>
      <div className="text-[11px] font-medium uppercase tracking-wider text-(--muted)">
        {label}
      </div>
      <div className="mt-1">{children}</div>
    </div>
  )
}

/* ── Traffic details ───────────────────────────────────────────────────── */

function TrafficSection({
  loading,
  granularity,
  onGranularityChange,
  series,
  summary,
}: {
  loading: boolean
  granularity: Granularity
  onGranularityChange: (g: Granularity) => void
  series: TrafficSeries | null
  summary: NodeTrafficSummary | null
}) {
  const points = series?.points ?? []
  const totalTx = summary?.total?.tx ?? 0
  const totalRx = summary?.total?.rx ?? 0
  const byUser = (summary?.by_user ?? []).slice(0, 8)

  return (
    <Section
      title="Traffic"
      meta={
        !loading ? (
          <span className="font-mono tabular-nums">
            ↑ {formatBytes(totalTx)} · ↓ {formatBytes(totalRx)}
          </span>
        ) : undefined
      }
      action={
        <GranularityToggle value={granularity} onChange={onGranularityChange} />
      }
    >
      <div className="p-3 sm:p-4">
        {loading ? (
          <div className="h-[220px] animate-pulse rounded bg-(--surface-secondary)" />
        ) : points.length === 0 ? (
          <div className="grid h-[220px] place-items-center text-[13px] text-(--muted)">
            No traffic recorded in this window.
          </div>
        ) : (
          <TrafficChart
            points={points}
            granularity={granularity}
            idPrefix="node-traffic"
          />
        )}
      </div>

      {!loading && byUser.length > 0 && (
        <div className="overflow-x-auto border-t border-(--border)">
          <table className="w-full border-collapse text-[13px]">
            <thead>
              <tr className="border-b border-(--border) bg-(--surface-secondary) text-left">
                <Th>Top users</Th>
                <Th className="text-right">TX</Th>
                <Th className="text-right">RX</Th>
                <Th className="text-right">Total</Th>
              </tr>
            </thead>
            <tbody className="divide-y divide-(--separator)">
              {byUser.map((u, i) => (
                <tr
                  key={u.user?.id || i}
                  className="transition-colors duration-150 hover:bg-(--surface-secondary)"
                >
                  <Td>
                    <span className="block max-w-[280px] truncate font-medium">
                      {u.user?.email || 'unknown'}
                    </span>
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-(--muted)">↑</span>{' '}
                    {formatBytes(u.tx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    <span className="text-(--muted)">↓</span>{' '}
                    {formatBytes(u.rx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                    {formatBytes((u.tx ?? 0) + (u.rx ?? 0))}
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </Section>
  )
}

/* ── Streams dump (on demand) ──────────────────────────────────────────── */

function StreamsSection({ nodeId }: { nodeId: string }) {
  const [live, setLive] = useState<NodeLive | null>(null)
  const [loading, setLoading] = useState(false)
  const [fetchedAt, setFetchedAt] = useState<number | null>(null)
  const [now, setNow] = useState(() => Date.now())
  const [reqError, setReqError] = useState('')

  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 5_000)
    return () => clearInterval(id)
  }, [])

  const fetchStreams = useCallback(async () => {
    setLoading(true)
    setReqError('')
    try {
      const { data, error } = await apiClient.GET(
        '/api/panel/nodes/{id}/live',
        {
          params: { path: { id: nodeId } },
        }
      )
      if (error) {
        setReqError("Couldn't reach the panel API.")
        return
      }
      setLive(data ?? null)
      setFetchedAt(Date.now())
    } catch {
      setReqError('Network error while fetching streams.')
    } finally {
      setLoading(false)
    }
  }, [nodeId])

  const byUser = live?.by_user ?? []
  const topDomains = (live?.top_domains ?? []).slice(0, 12)
  const byConnection = live?.by_connection ?? []

  return (
    <Section
      title="Live streams"
      meta={
        live && !live.error ? (
          <span className="font-mono tabular-nums">
            {live.online_devices ?? 0} devices online ·{' '}
            {live.active_streams ?? 0} streams
          </span>
        ) : undefined
      }
      action={
        <div className="flex items-center gap-2.5">
          {fetchedAt !== null && (
            <span
              className="hidden text-xs tabular-nums text-(--muted) sm:inline"
              title={new Date(fetchedAt).toLocaleString()}
            >
              {relTime(fetchedAt, now)}
            </span>
          )}
          <Button
            size="sm"
            variant="secondary"
            onPress={fetchStreams}
            isPending={loading}
          >
            {fetchedAt === null ? 'Fetch streams' : 'Refresh'}
          </Button>
        </div>
      }
    >
      {reqError ? (
        <PanelMessage>{reqError}</PanelMessage>
      ) : loading && !live ? (
        <TableSkeleton />
      ) : live?.error ? (
        <div
          className="px-4 py-3 text-[13px] text-(--danger)"
          title={live.error}
        >
          {live.error}
        </div>
      ) : !live ? (
        <Teaching
          title="No snapshot yet"
          hint="Fetch a live snapshot to see every active stream on this node, grouped by user."
        />
      ) : byUser.length === 0 ? (
        <Teaching
          title="No active streams"
          hint="Nobody is routing traffic through this node right now."
        />
      ) : (
        <div className="flex flex-col">
          {byUser.map((u, i) => (
            <UserStreams
              key={u.user?.id || `unknown-${i}`}
              group={u}
              now={now}
            />
          ))}

          {topDomains.length > 0 && <TopDomainsTable rows={topDomains} />}
          {byConnection.length > 0 && (
            <ByConnectionTable rows={byConnection} />
          )}
        </div>
      )}
    </Section>
  )
}

function UserStreams({
  group,
  now,
}: {
  group: NonNullable<NodeLive['by_user']>[number]
  now: number
}) {
  const streams = group.streams ?? []
  return (
    <div className="border-t border-(--border) first:border-t-0">
      <div className="flex items-center justify-between gap-3 bg-(--surface-secondary) px-3 py-1.5">
        <div className="flex min-w-0 items-center gap-2">
          <Dot tone="ok" />
          <span className="truncate text-xs font-medium">
            {group.user?.email || 'unknown'}
          </span>
        </div>
        <span className="shrink-0 font-mono text-xs tabular-nums text-(--muted)">
          {group.online_devices ?? 0} devices online · {streams.length} streams
        </span>
      </div>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-b border-(--separator) text-left">
              <Th>Target</Th>
              <Th>State</Th>
              <Th className="text-right">TX</Th>
              <Th className="text-right">RX</Th>
              <Th className="text-right">Lifetime</Th>
              <Th className="text-right">Idle</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--separator)">
            {streams.map((s, i) => {
              const target = s.hooked_req_addr || s.req_addr || '—'
              return (
                <tr
                  key={`${s.connection}-${s.stream}-${i}`}
                  className="transition-colors duration-150 hover:bg-(--surface-secondary)"
                >
                  <Td>
                    <span
                      className="block max-w-[320px] truncate font-mono text-xs text-(--foreground)"
                      title={target}
                    >
                      {target}
                    </span>
                  </Td>
                  <Td className="whitespace-nowrap font-mono text-xs text-(--muted)">
                    {s.state || '—'}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    {formatBytes(s.tx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    {formatBytes(s.rx ?? 0)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                    {formatDuration(s.lifetime_sec ?? -1)}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums text-(--muted)">
                    {formatDuration(s.idle_sec ?? -1)}
                  </Td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function TopDomainsTable({
  rows,
}: {
  rows: NonNullable<NodeLive['top_domains']>
}) {
  return (
    <div className="bg-(--surface)">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-y border-(--border) bg-(--surface-secondary) text-left">
              <Th>Top domains</Th>
              <Th>ASN</Th>
              <Th>Country</Th>
              <Th className="text-right">Streams</Th>
              <Th className="text-right">Total</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--separator)">
            {rows.map((d, i) => {
              const domain = d.domain || '—'
              const meta = d.ip_meta
              const countryTitle =
                meta?.country_name || meta?.country_code || ''
              return (
                <tr
                  key={(d.domain || '') + i}
                  className="hover:bg-(--surface-secondary)"
                >
                  <Td>
                    {meta?.ipinfo_url ? (
                      <a
                        href={meta.ipinfo_url}
                        target="_blank"
                        rel="noreferrer"
                        className="block max-w-[260px] truncate font-mono text-xs text-(--foreground) underline decoration-(--border) underline-offset-2 transition-colors duration-150 hover:text-(--accent)"
                        title={`Open ${meta.ip || domain} on ipinfo.io`}
                      >
                        {domain}
                      </a>
                    ) : (
                      <span
                        className="block max-w-[260px] truncate font-mono text-xs"
                        title={domain}
                      >
                        {domain}
                      </span>
                    )}
                  </Td>
                  <Td className="whitespace-nowrap font-mono text-xs text-(--muted)">
                    {meta?.asn || '—'}
                  </Td>
                  <Td className="whitespace-nowrap text-xs text-(--muted)">
                    {meta?.country_code ? (
                      <span title={countryTitle}>
                        <span className="font-mono text-(--foreground)">
                          {meta.country_code}
                        </span>
                        {meta.country_name && (
                          <span className="ml-1 hidden max-w-[140px] truncate align-bottom sm:inline-block">
                            {meta.country_name}
                          </span>
                        )}
                      </span>
                    ) : (
                      '—'
                    )}
                  </Td>
                  <Td className="text-right font-mono text-xs tabular-nums text-(--muted)">
                    {d.streams ?? 0}
                  </Td>
                  <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                    {formatBytes((d.tx ?? 0) + (d.rx ?? 0))}
                  </Td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function ByConnectionTable({
  rows,
}: {
  rows: NonNullable<NodeLive['by_connection']>
}) {
  return (
    <div className="bg-(--surface)">
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-[13px]">
          <thead>
            <tr className="border-y border-(--border) bg-(--surface-secondary) text-left">
              <Th>Device</Th>
              <Th>Top domain</Th>
              <Th className="text-right">Streams</Th>
              <Th className="text-right">Total</Th>
            </tr>
          </thead>
          <tbody className="divide-y divide-(--separator)">
            {rows.map((c, i) => (
              <tr
                key={`${c.connection}-${i}`}
                className="hover:bg-(--surface-secondary)"
              >
                <Td className="whitespace-nowrap font-mono text-xs tabular-nums text-(--muted)">
                  #{c.connection ?? 0}
                </Td>
                <Td>
                  <span
                    className="block max-w-[200px] truncate font-mono text-xs"
                    title={c.top_domain || ''}
                  >
                    {c.top_domain || '—'}
                  </span>
                </Td>
                <Td className="text-right font-mono text-xs tabular-nums text-(--muted)">
                  {c.stream_count ?? 0}
                </Td>
                <Td className="whitespace-nowrap text-right font-mono text-xs tabular-nums">
                  {formatBytes((c.tx ?? 0) + (c.rx ?? 0))}
                </Td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function BackIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-4"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d="m15 18-6-6 6-6" />
    </svg>
  )
}
