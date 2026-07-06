import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  apiGet,
  type DayCount,
  type ForecastResponse,
  type SessionsResponse,
  type StatsSummary,
  type TimeBucket,
  type TimeResponse,
} from "../api";

// Chart tokens (dataviz reference palette): sequential blue ramp steps per
// mode, chrome inks. Wired as CSS custom properties on .viz-root so light
// and dark swap in one place.
const vizCss = `
.viz-root {
  --surface-1: #fcfcfb;
  --ink-2: #52514e;
  --ink-muted: #898781;
  --grid: #e1e0d9;
  --baseline: #c3c2b7;
  --bar: #2a78d6;
  --heat-0: #f0efec;
  --heat-1: #b7d3f6;
  --heat-2: #86b6ef;
  --heat-3: #3987e5;
  --heat-4: #1c5cab;
}
@media (prefers-color-scheme: dark) {
  .viz-root {
    --surface-1: #1a1a19;
    --ink-2: #c3c2b7;
    --ink-muted: #898781;
    --grid: #2c2c2a;
    --baseline: #383835;
    --bar: #3987e5;
    --heat-0: #2c2c2a;
    --heat-1: #104281;
    --heat-2: #1c5cab;
    --heat-3: #2a78d6;
    --heat-4: #86b6ef;
  }
}`;

function fmtMs(ms: number): string {
  const s = Math.round(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  return `${Math.floor(m / 60)}h ${m % 60}m`;
}

const kindLabels: Record<string, string> = {
  card_review: "Card reviews",
  note_read: "Note reading",
  note_edit: "Note editing",
  free_text_answer: "Free-text answers",
  pdf_read: "PDF reading",
  sync: "Sync",
};

type Tip = { x: number; y: number; text: string } | null;

export default function Stats() {
  const [tip, setTip] = useState<Tip>(null);

  const summary = useQuery({
    queryKey: ["stats", "summary"],
    queryFn: () => apiGet<StatsSummary>("/api/stats/summary"),
  });
  const heatmap = useQuery({
    queryKey: ["stats", "heatmap"],
    queryFn: () => apiGet<DayCount[]>("/api/stats/heatmap?days=182"),
  });
  const forecast = useQuery({
    queryKey: ["stats", "forecast"],
    queryFn: () => apiGet<ForecastResponse>("/api/stats/forecast?days=30"),
  });
  const time = useQuery({
    queryKey: ["stats", "time"],
    queryFn: () => apiGet<TimeResponse>("/api/stats/time?days=30"),
  });
  const sessions = useQuery({
    queryKey: ["sessions"],
    queryFn: () => apiGet<SessionsResponse>("/api/sessions"),
  });

  const show = (e: React.MouseEvent, text: string) =>
    setTip({ x: e.clientX, y: e.clientY, text });

  if (summary.isError)
    return <p className="text-sm text-red-600">{String(summary.error)}</p>;
  if (!summary.data) return <p className="text-sm text-zinc-500">Loading…</p>;
  const s = summary.data;

  return (
    <div className="viz-root space-y-8" onMouseLeave={() => setTip(null)}>
      <style>{vizCss}</style>

      {/* KPI row */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
        <StatTile label="Due now" value={String(s.due_now)} />
        <StatTile
          label="Reviews today"
          value={String(s.reviews_today)}
          sub={`${s.new_remaining} new remaining`}
        />
        <StatTile label="Time today" value={fmtMs(s.time_today_ms)} />
        <StatTile
          label="Retention, 30 days"
          value={s.retention_30 < 0 ? "—" : `${Math.round(s.retention_30 * 100)}%`}
          sub={s.avg_review_ms > 0 ? `${fmtMs(s.avg_review_ms)} per card` : undefined}
        />
      </div>

      {heatmap.data && (
        <Heatmap days={heatmap.data} onHover={show} onLeave={() => setTip(null)} />
      )}

      {forecast.data && (
        <Forecast data={forecast.data} onHover={show} onLeave={() => setTip(null)} />
      )}

      {time.data && (time.data.by_kind.length > 0 || time.data.by_deck.length > 0) && (
        <div className="grid gap-6 sm:grid-cols-2">
          <BarList
            title="Time by activity, 30 days"
            items={time.data.by_kind.map((b) => ({
              ...b,
              key: kindLabels[b.key] ?? b.key,
            }))}
          />
          <BarList title="Review time by deck, 30 days" items={time.data.by_deck} />
        </div>
      )}

      {sessions.data && sessions.data.recent.length > 0 && (
        <SessionsTable sessions={sessions.data} />
      )}

      {tip && (
        <div
          className="pointer-events-none fixed z-50 rounded-md bg-zinc-900 px-2.5 py-1.5 text-xs text-white shadow-lg dark:bg-zinc-100 dark:text-zinc-900"
          style={{ left: tip.x + 12, top: tip.y + 12 }}
        >
          {tip.text}
        </div>
      )}
    </div>
  );
}

function StatTile({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
      <div className="text-xs text-zinc-500">{label}</div>
      <div className="mt-1 text-2xl font-semibold">{value}</div>
      {sub && <div className="mt-0.5 text-xs text-zinc-400">{sub}</div>}
    </div>
  );
}

type HoverFns = {
  onHover: (e: React.MouseEvent, text: string) => void;
  onLeave: () => void;
};

// GitHub-style review heatmap: 26 weeks × 7 days, sequential blue ramp
// (near-zero recedes toward the surface).
function Heatmap({ days, onHover, onLeave }: { days: DayCount[] } & HoverFns) {
  const byDate = new Map(days.map((d) => [d.date, d.count]));
  const today = new Date();
  const weeks = 26;

  // Grid columns = weeks, rows = Mon..Sun. Find the Monday `weeks` ago.
  const start = new Date(today);
  start.setDate(start.getDate() - ((today.getDay() + 6) % 7) - (weeks - 1) * 7);

  const cells: { date: string; count: number; future: boolean }[] = [];
  for (let w = 0; w < weeks; w++) {
    for (let d = 0; d < 7; d++) {
      const day = new Date(start);
      day.setDate(start.getDate() + w * 7 + d);
      const iso = day.toISOString().slice(0, 10);
      cells.push({
        date: iso,
        count: byDate.get(iso) ?? 0,
        future: day > today,
      });
    }
  }

  const bucket = (n: number) =>
    n === 0 ? 0 : n <= 2 ? 1 : n <= 5 ? 2 : n <= 9 ? 3 : 4;

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Review activity
      </h2>
      <div className="overflow-x-auto rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
        <div
          className="grid w-max grid-flow-col gap-0.5"
          style={{ gridTemplateRows: "repeat(7, 12px)" }}
        >
          {cells.map((c) => (
            <div
              key={c.date}
              className="h-3 w-3 rounded-[3px]"
              style={{
                background: c.future ? "transparent" : `var(--heat-${bucket(c.count)})`,
              }}
              onMouseMove={(e) =>
                !c.future &&
                onHover(e, `${c.date}: ${c.count} review${c.count === 1 ? "" : "s"}`)
              }
              onMouseLeave={onLeave}
            />
          ))}
        </div>
        <div className="mt-3 flex items-center gap-1 text-xs" style={{ color: "var(--ink-muted)" }}>
          less
          {[0, 1, 2, 3, 4].map((b) => (
            <span
              key={b}
              className="inline-block h-3 w-3 rounded-[3px]"
              style={{ background: `var(--heat-${b})` }}
            />
          ))}
          more
        </div>
      </div>
    </section>
  );
}

// Due forecast: single-series column chart (no legend needed), thin bars
// with 4px rounded data-ends, hairline gridlines, tooltip per bar.
function Forecast({ data, onHover, onLeave }: { data: ForecastResponse } & HoverFns) {
  const days = 30;
  const byDate = new Map(data.forecast.map((d) => [d.date, d.count]));
  const series: { date: string; count: number }[] = [];
  const today = new Date();
  for (let i = 0; i < days; i++) {
    const day = new Date(today);
    day.setDate(today.getDate() + i);
    const iso = day.toISOString().slice(0, 10);
    series.push({ date: iso, count: byDate.get(iso) ?? 0 });
  }

  const w = 640;
  const h = 160;
  const pad = { top: 8, right: 8, bottom: 20, left: 28 };
  const innerW = w - pad.left - pad.right;
  const innerH = h - pad.top - pad.bottom;
  const maxRaw = Math.max(1, ...series.map((d) => d.count));
  // Clean axis max: next multiple of a nice step.
  const step = maxRaw <= 5 ? 1 : maxRaw <= 10 ? 2 : maxRaw <= 25 ? 5 : 10;
  const yMax = Math.ceil(maxRaw / step) * step;
  const slot = innerW / days;
  const barW = Math.min(24, Math.max(4, slot - 2));

  const ticks: number[] = [];
  for (let v = 0; v <= yMax; v += step) ticks.push(v);

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Due forecast, next 30 days
        {data.overdue > 0 && (
          <span className="ml-2 normal-case tracking-normal text-red-600 dark:text-red-400">
            ⚠ {data.overdue} overdue
          </span>
        )}
      </h2>
      <div className="overflow-x-auto rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
        <svg viewBox={`0 0 ${w} ${h}`} className="min-w-[480px]" role="img"
          aria-label="Cards due per day over the next 30 days">
          {ticks.map((v) => {
            const y = pad.top + innerH - (v / yMax) * innerH;
            return (
              <g key={v}>
                <line
                  x1={pad.left} x2={w - pad.right} y1={y} y2={y}
                  stroke={v === 0 ? "var(--baseline)" : "var(--grid)"}
                  strokeWidth="1"
                />
                <text
                  x={pad.left - 6} y={y + 3} textAnchor="end" fontSize="9"
                  fill="var(--ink-muted)" style={{ fontVariantNumeric: "tabular-nums" }}
                >
                  {v}
                </text>
              </g>
            );
          })}
          {series.map((d, i) => {
            const x = pad.left + i * slot + (slot - barW) / 2;
            const barH = (d.count / yMax) * innerH;
            const y = pad.top + innerH - barH;
            const r = Math.min(4, barW / 2, barH);
            return (
              <g key={d.date}>
                {d.count > 0 && (
                  // 4px rounded data-end, square at the baseline.
                  <path
                    d={`M ${x} ${pad.top + innerH} V ${y + r} Q ${x} ${y} ${x + r} ${y} H ${x + barW - r} Q ${x + barW} ${y} ${x + barW} ${y + r} V ${pad.top + innerH} Z`}
                    fill="var(--bar)"
                  />
                )}
                {/* hover hit target: full slot height, wider than the mark */}
                <rect
                  x={pad.left + i * slot} y={pad.top} width={slot} height={innerH}
                  fill="transparent"
                  onMouseMove={(e) => onHover(e, `${d.date}: ${d.count} due`)}
                  onMouseLeave={onLeave}
                />
                {i % 5 === 0 && (
                  <text
                    x={x + barW / 2} y={h - 6} textAnchor="middle" fontSize="9"
                    fill="var(--ink-muted)"
                  >
                    {d.date.slice(5)}
                  </text>
                )}
              </g>
            );
          })}
        </svg>
      </div>
    </section>
  );
}

// Horizontal bar list: one measure across categories → single hue; value
// labeled at the bar tip in ink (never in the series color).
function BarList({ title, items }: { title: string; items: TimeBucket[] }) {
  const max = Math.max(1, ...items.map((b) => b.ms));
  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        {title}
      </h2>
      <div className="space-y-2 rounded-lg border border-zinc-200 bg-white p-4 dark:border-zinc-800 dark:bg-zinc-900">
        {items.length === 0 && <p className="text-xs text-zinc-400">No timed activity yet.</p>}
        {items.map((b) => (
          <div key={b.key} className="flex items-center gap-2 text-sm">
            <span className="w-36 shrink-0 truncate text-xs" style={{ color: "var(--ink-2)" }}>
              {b.key}
            </span>
            <span
              className="h-2.5 shrink-0 rounded-r-[4px]"
              style={{ width: `${Math.max(2, (b.ms / max) * 100 * 0.6)}%`, background: "var(--bar)" }}
            />
            <span className="text-xs" style={{ color: "var(--ink-muted)" }}>
              {fmtMs(b.ms)}
            </span>
          </div>
        ))}
      </div>
    </section>
  );
}

function SessionsTable({ sessions }: { sessions: SessionsResponse }) {
  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Recent sessions
      </h2>
      <div className="overflow-x-auto rounded-lg border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-zinc-200 text-left text-xs text-zinc-500 dark:border-zinc-800">
              <th className="px-4 py-2 font-medium">Kind</th>
              <th className="px-4 py-2 font-medium">Started</th>
              <th className="px-4 py-2 font-medium">Duration</th>
              <th className="px-4 py-2 font-medium">Note</th>
            </tr>
          </thead>
          <tbody className="tabular-nums">
            {sessions.recent.slice(0, 10).map((sess) => {
              const start = new Date(sess.started_at);
              const end = sess.ended_at ? new Date(sess.ended_at) : new Date();
              return (
                <tr key={sess.id} className="border-b border-zinc-100 last:border-0 dark:border-zinc-800/60">
                  <td className="px-4 py-2 capitalize">
                    {sess.kind}
                    {!sess.ended_at && (
                      <span className="ml-2 text-xs text-emerald-600 dark:text-emerald-400">active</span>
                    )}
                  </td>
                  <td className="px-4 py-2 text-zinc-500">{start.toLocaleString()}</td>
                  <td className="px-4 py-2">{fmtMs(end.getTime() - start.getTime())}</td>
                  <td className="px-4 py-2 text-zinc-500">{sess.note}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
    </section>
  );
}
