import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiGet,
  apiPatch,
  apiPost,
  causeLabels,
  type CauseCount,
  type ErrorEntry,
  type ErrorStatsResponse,
  type RootCause,
  type TriageItem,
  type TriageResponse,
} from "../api";

const statusFilters = [
  { value: "open", label: "Open" },
  { value: "resolved", label: "Resolved" },
  { value: "all", label: "All" },
] as const;

type StatusFilter = (typeof statusFilters)[number]["value"];

interface Details {
  note: string;
  repair_action: string;
  repair_due: string;
}

const emptyDetails: Details = { note: "", repair_action: "", repair_due: "" };

function todayISO(): string {
  return new Date().toISOString().slice(0, 10);
}

export default function Errors() {
  return (
    <div className="space-y-8">
      <Triage />
      <Diagnosed />
      <Patterns />
    </div>
  );
}

function Triage() {
  const queryClient = useQueryClient();
  const [expandedId, setExpandedId] = useState<number | null>(null);
  const [details, setDetails] = useState<Record<number, Details>>({});

  const triage = useQuery({
    queryKey: ["errors", "triage"],
    queryFn: () => apiGet<TriageResponse>("/api/errors/triage"),
  });

  const classify = useMutation({
    mutationFn: (body: {
      event_id: number;
      root_cause: RootCause;
      note?: string;
      repair_action?: string;
      repair_due?: string;
    }) => apiPost("/api/errors", body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["errors"] });
      queryClient.invalidateQueries({ queryKey: ["today"] });
    },
  });

  if (!triage.data || triage.data.items.length === 0) return null;

  const items = triage.data.items;
  const causes = triage.data.causes;

  function detailsFor(eventId: number): Details {
    return details[eventId] ?? emptyDetails;
  }

  function updateDetails(eventId: number, patch: Partial<Details>) {
    setDetails((prev) => ({ ...prev, [eventId]: { ...detailsFor(eventId), ...patch } }));
  }

  function classifyItem(item: TriageItem, cause: RootCause) {
    const body: {
      event_id: number;
      root_cause: RootCause;
      note?: string;
      repair_action?: string;
      repair_due?: string;
    } = { event_id: item.event_id, root_cause: cause };
    if (expandedId === item.event_id) {
      const d = detailsFor(item.event_id);
      if (d.note.trim()) body.note = d.note.trim();
      if (d.repair_action.trim()) body.repair_action = d.repair_action.trim();
      if (d.repair_due) body.repair_due = d.repair_due;
    }
    classify.mutate(body);
  }

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Triage — classify recent failures ({items.length})
      </h2>
      <ul className="space-y-2">
        {items.map((item) => {
          const expanded = expandedId === item.event_id;
          const d = detailsFor(item.event_id);
          return (
            <li
              key={item.event_id}
              className="rounded-md border border-zinc-200 bg-white px-3 py-2 dark:border-zinc-800 dark:bg-zinc-900"
            >
              <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                <span
                  className="max-w-56 truncate font-medium sm:max-w-xs"
                  title={item.card_front}
                >
                  {item.card_front}
                </span>
                <span className="text-xs text-zinc-400">{item.deck || "—"}</span>
                {item.kind === "card_review" ? (
                  <span className="rounded-full bg-red-100 px-2 py-0.5 text-xs text-red-700 dark:bg-red-900/40 dark:text-red-300">
                    again
                  </span>
                ) : (
                  <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
                    graded incorrect
                  </span>
                )}
                <span className="ml-auto text-xs text-zinc-400">
                  {new Date(item.ts).toLocaleDateString()}
                </span>
              </div>
              {item.answer && (
                <p className="mt-1 text-sm italic text-zinc-500">
                  you answered: {item.answer}
                </p>
              )}
              <div className="mt-2 flex flex-wrap items-center gap-1">
                {causes.map((cause) => (
                  <button
                    key={cause}
                    type="button"
                    disabled={classify.isPending}
                    onClick={() => classifyItem(item, cause)}
                    className="rounded-full border border-zinc-300 px-2 py-0.5 text-xs text-zinc-600 hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                  >
                    {causeLabels[cause]}
                  </button>
                ))}
                <button
                  type="button"
                  onClick={() => setExpandedId(expanded ? null : item.event_id)}
                  className="ml-1 text-xs text-zinc-400 hover:underline"
                >
                  {expanded ? "− details" : "+ details"}
                </button>
              </div>
              {expanded && (
                <div className="mt-2 flex flex-wrap gap-2">
                  <input
                    type="text"
                    value={d.note}
                    onChange={(e) => updateDetails(item.event_id, { note: e.target.value })}
                    placeholder="What actually went wrong?"
                    className="min-w-48 flex-1 rounded-md border border-zinc-300 bg-white px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
                  />
                  <input
                    type="text"
                    value={d.repair_action}
                    onChange={(e) =>
                      updateDetails(item.event_id, { repair_action: e.target.value })
                    }
                    placeholder="e.g. rewrite the derivation from memory"
                    className="min-w-48 flex-1 rounded-md border border-zinc-300 bg-white px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
                  />
                  <input
                    type="date"
                    value={d.repair_due}
                    onChange={(e) =>
                      updateDetails(item.event_id, { repair_due: e.target.value })
                    }
                    className="rounded-md border border-zinc-300 bg-white px-2 py-1 text-sm dark:border-zinc-700 dark:bg-zinc-900"
                  />
                </div>
              )}
            </li>
          );
        })}
      </ul>
      {classify.isError && (
        <p className="mt-2 text-sm text-red-600">{String(classify.error)}</p>
      )}
    </section>
  );
}

function Diagnosed() {
  const queryClient = useQueryClient();
  const [status, setStatus] = useState<StatusFilter>("open");
  const [cause, setCause] = useState<RootCause | "">("");

  const params = new URLSearchParams();
  params.set("status", status);
  if (cause) params.set("cause", cause);

  const errors = useQuery({
    queryKey: ["errors", status, cause],
    queryFn: () => apiGet<ErrorEntry[]>(`/api/errors?${params}`),
  });

  const setResolved = useMutation({
    mutationFn: ({ id, resolved }: { id: number; resolved: boolean }) =>
      apiPatch(`/api/errors/${id}`, { resolved }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["errors"] });
      queryClient.invalidateQueries({ queryKey: ["today"] });
    },
  });

  const today = todayISO();

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Diagnosed
      </h2>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <select
          value={status}
          onChange={(e) => setStatus(e.target.value as StatusFilter)}
          className="rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        >
          {statusFilters.map((f) => (
            <option key={f.value} value={f.value}>
              {f.label}
            </option>
          ))}
        </select>
        <select
          value={cause}
          onChange={(e) => setCause(e.target.value as RootCause | "")}
          className="rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        >
          <option value="">All causes</option>
          {(Object.keys(causeLabels) as RootCause[]).map((c) => (
            <option key={c} value={c}>
              {causeLabels[c]}
            </option>
          ))}
        </select>
      </div>

      {errors.isPending && <p className="text-sm text-zinc-500">Loading…</p>}
      {errors.isError && <p className="text-sm text-red-600">{String(errors.error)}</p>}

      {errors.data && errors.data.length === 0 && (
        <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center text-sm text-zinc-500 dark:border-zinc-700">
          No diagnosed errors yet. Classify failures above, or fail something first 🙂
        </div>
      )}

      {errors.data && errors.data.length > 0 && (
        <ul className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 bg-white dark:divide-zinc-800 dark:border-zinc-800 dark:bg-zinc-900">
          {errors.data.map((entry) => {
            const overdue =
              entry.repair_due !== "" && entry.repair_due <= today && entry.resolved_at === "";
            return (
              <li
                key={entry.id}
                className="flex flex-wrap items-start gap-x-3 gap-y-1 px-4 py-3"
              >
                <span className="rounded-full bg-violet-100 px-2 py-0.5 text-xs text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
                  {causeLabels[entry.root_cause]}
                </span>
                <div className="min-w-0 flex-1">
                  <div>
                    {entry.note_path ? (
                      <Link to={`/notes/${entry.note_path}`} className="font-medium hover:underline">
                        {entry.card_front}
                      </Link>
                    ) : (
                      <span className="font-medium">{entry.card_front}</span>
                    )}
                    <span className="ml-2 text-xs text-zinc-400">{entry.deck || "—"}</span>
                  </div>
                  {entry.note && <p className="text-sm text-zinc-500">{entry.note}</p>}
                  {entry.repair_action && (
                    <p className="mt-1 flex flex-wrap items-center gap-2 text-sm text-zinc-500">
                      <span>Repair: {entry.repair_action}</span>
                      {entry.repair_note_path && (
                        <Link
                          to={`/notes/${entry.repair_note_path}`}
                          className="text-xs text-zinc-400 hover:underline"
                        >
                          ↗ note
                        </Link>
                      )}
                      {entry.repair_due && (
                        <span
                          className={`rounded px-1.5 py-0.5 text-xs ${
                            overdue
                              ? "bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300"
                              : "bg-zinc-100 text-zinc-500 dark:bg-zinc-800 dark:text-zinc-400"
                          }`}
                        >
                          {entry.repair_due}
                        </span>
                      )}
                    </p>
                  )}
                </div>
                <div className="flex items-center gap-2">
                  {entry.resolved_at ? (
                    <>
                      <span className="text-xs text-zinc-400">
                        resolved {new Date(entry.resolved_at).toLocaleDateString()}
                      </span>
                      <button
                        type="button"
                        disabled={setResolved.isPending}
                        onClick={() => setResolved.mutate({ id: entry.id, resolved: false })}
                        className="rounded-md border border-zinc-300 px-2 py-1 text-xs text-zinc-600 hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                      >
                        Reopen
                      </button>
                    </>
                  ) : (
                    <button
                      type="button"
                      disabled={setResolved.isPending}
                      onClick={() => setResolved.mutate({ id: entry.id, resolved: true })}
                      className="rounded-md border border-zinc-300 px-2 py-1 text-xs text-zinc-600 hover:bg-zinc-100 disabled:opacity-50 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                    >
                      Resolve
                    </button>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

function statTable(title: string, rows: CauseCount[], showDeck: boolean) {
  return (
    <div className="overflow-x-auto rounded-lg border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-zinc-200 text-left text-xs text-zinc-500 dark:border-zinc-800">
            <th className="px-3 py-2 font-medium">{title}</th>
            {showDeck && <th className="px-3 py-2 font-medium">Cause</th>}
            <th className="px-3 py-2 font-medium">Open</th>
            <th className="px-3 py-2 font-medium">Total</th>
          </tr>
        </thead>
        <tbody>
          {rows.map((r, i) => (
            <tr
              key={i}
              className="border-b border-zinc-100 last:border-0 dark:border-zinc-800/60"
            >
              <td className="px-3 py-2">{showDeck ? r.deck || "—" : causeLabels[r.cause]}</td>
              {showDeck && <td className="px-3 py-2 text-zinc-500">{causeLabels[r.cause]}</td>}
              <td className="px-3 py-2 tabular-nums text-zinc-500">{r.open}</td>
              <td className="px-3 py-2 tabular-nums text-zinc-500">{r.total}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function Patterns() {
  const stats = useQuery({
    queryKey: ["errors", "stats"],
    queryFn: () => apiGet<ErrorStatsResponse>("/api/errors/stats"),
  });

  if (!stats.data || stats.data.by_cause.length === 0) return null;

  return (
    <section>
      <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Patterns
      </h2>
      <div className="grid gap-4 sm:grid-cols-2">
        {statTable("By cause", stats.data.by_cause, false)}
        {statTable("By deck", stats.data.by_deck.slice(0, 12), true)}
      </div>
    </section>
  );
}
