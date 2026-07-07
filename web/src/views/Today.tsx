import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { apiGet, causeLabels, type TodayResponse } from "../api";
import { stageColors } from "./Notes";

function fmtMinutes(ms: number): string {
  const min = Math.round(ms / 60000);
  if (min < 60) return `${min} min`;
  return `${Math.floor(min / 60)}h ${min % 60}m`;
}

// Start-of-day dashboard: what is due, what is stuck, what needs repair.
export default function Today() {
  const today = useQuery({
    queryKey: ["today"],
    queryFn: () => apiGet<TodayResponse>("/api/today"),
  });

  if (today.isPending) return <p className="text-sm text-zinc-500">Loading…</p>;
  if (today.isError)
    return <p className="text-sm text-red-600">{String(today.error)}</p>;

  const t = today.data;
  const s = t.summary;
  const workLeft = s.due_now + s.new_remaining;

  return (
    <div className="space-y-6">
      <section className="rounded-lg border border-zinc-200 bg-white p-6 dark:border-zinc-800 dark:bg-zinc-900">
        {workLeft > 0 ? (
          <>
            <h1 className="text-xl font-semibold">
              {s.due_now} due · {s.new_remaining} new available
            </h1>
            <p className="mt-1 text-sm text-zinc-500">
              {s.reviews_today > 0
                ? `${s.reviews_today} reviews done today (${fmtMinutes(s.time_today_ms)} total activity).`
                : "Nothing reviewed yet today."}
            </p>
            <Link
              to="/review"
              className="mt-4 inline-block rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-700 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
            >
              Start reviewing →
            </Link>
          </>
        ) : (
          <>
            <h1 className="text-xl font-semibold">Queue clear 🎉</h1>
            <p className="mt-1 text-sm text-zinc-500">
              {s.reviews_today} reviews done today
              {s.time_today_ms > 0 && <> · {fmtMinutes(s.time_today_ms)} total activity</>}
              . Good moment for deep reading or folding notes.
            </p>
          </>
        )}
      </section>

      {t.repairs_due.length > 0 && (
        <section>
          <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            Repair tasks due
          </h2>
          <ul className="divide-y divide-zinc-200 rounded-lg border border-red-200 bg-white dark:divide-zinc-800 dark:border-red-900/50 dark:bg-zinc-900">
            {t.repairs_due.map((r) => (
              <li key={r.id}>
                <Link
                  to="/errors"
                  className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-800/60"
                >
                  <span className="font-medium">
                    {r.repair_action || `revisit: ${r.card_front}`}
                  </span>
                  <span className="rounded bg-violet-100 px-1.5 py-0.5 text-xs text-violet-700 dark:bg-violet-900/40 dark:text-violet-300">
                    {causeLabels[r.root_cause] ?? r.root_cause}
                  </span>
                  <span className="ml-auto text-xs text-zinc-400">
                    due {r.repair_due}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}

      {(t.leeches > 0 || t.open_questions > 0 || t.error_triage > 0) && (
        <section className="flex flex-wrap gap-3 text-sm">
          {t.error_triage > 0 && (
            <Link
              to="/errors"
              className="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-red-800 hover:bg-red-100 dark:border-red-700/60 dark:bg-red-900/20 dark:text-red-300 dark:hover:bg-red-900/40"
            >
              <span className="font-semibold">{t.error_triage}</span> failure
              {t.error_triage === 1 ? "" : "s"} to classify — what went wrong?
            </Link>
          )}
          {t.leeches > 0 && (
            <Link
              to="/cards"
              className="rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-amber-800 hover:bg-amber-100 dark:border-amber-700/60 dark:bg-amber-900/20 dark:text-amber-300 dark:hover:bg-amber-900/40"
            >
              <span className="font-semibold">{t.leeches}</span> leech
              {t.leeches === 1 ? "" : "es"} — repeatedly forgotten cards worth
              rewriting
            </Link>
          )}
          {t.open_questions > 0 && (
            <Link
              to="/notes"
              className="rounded-lg border border-sky-300 bg-sky-50 px-4 py-3 text-sky-800 hover:bg-sky-100 dark:border-sky-700/60 dark:bg-sky-900/20 dark:text-sky-300 dark:hover:bg-sky-900/40"
            >
              <span className="font-semibold">{t.open_questions}</span> open
              question{t.open_questions === 1 ? "" : "s"} waiting
              {t.oldest_questions.length > 0 && (
                <span className="block text-xs opacity-80">
                  oldest: “{t.oldest_questions[0].text}”
                </span>
              )}
            </Link>
          )}
        </section>
      )}

      {t.stale_notes.length > 0 && (
        <section>
          <h2 className="mb-2 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            Stuck notes — untouched 2+ weeks
          </h2>
          <ul className="divide-y divide-zinc-200 rounded-lg border border-zinc-200 bg-white dark:divide-zinc-800 dark:border-zinc-800 dark:bg-zinc-900">
            {t.stale_notes.map((n) => (
              <li key={n.path}>
                <Link
                  to={`/notes/${n.path}`}
                  className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3 hover:bg-zinc-50 dark:hover:bg-zinc-800/60"
                >
                  <span className="font-medium">{n.title}</span>
                  <span
                    className={`rounded-full px-2 py-0.5 text-xs ${stageColors[n.stage] ?? ""}`}
                  >
                    {n.stage}
                  </span>
                  <span className="ml-auto text-xs text-zinc-400">
                    {n.idle_days}d idle —{" "}
                    {n.stage === "skim" ? "deepen or drop" : "ready to fold?"}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
