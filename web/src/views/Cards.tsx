import { useState } from "react";
import { Link } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPatch, type CardInfo, type TimeBucket } from "../api";

const stateLabels = ["new", "learning", "review", "relearning"] as const;

function fmtDue(due: string, state: number): string {
  if (state === 0) return "—";
  const d = new Date(due);
  const days = Math.round((d.getTime() - Date.now()) / 86_400_000);
  if (days < 0) return `${-days}d overdue`;
  if (days === 0) return "today";
  if (days === 1) return "tomorrow";
  return `in ${days}d`;
}

export default function Cards() {
  const [q, setQ] = useState("");
  const [deck, setDeck] = useState("");
  const [status, setStatus] = useState("active");
  const queryClient = useQueryClient();

  const decks = useQuery({
    queryKey: ["decks"],
    queryFn: () => apiGet<TimeBucket[]>("/api/decks"),
  });

  const params = new URLSearchParams();
  if (q) params.set("q", q);
  if (deck) params.set("deck", deck);
  params.set("status", status);
  const cards = useQuery({
    queryKey: ["cards", q, deck, status],
    queryFn: () => apiGet<CardInfo[]>(`/api/cards?${params}`),
  });

  const suspend = useMutation({
    mutationFn: ({ id, suspended }: { id: string; suspended: boolean }) =>
      apiPatch(`/api/cards/${encodeURIComponent(id)}`, { suspended }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["cards"] });
      queryClient.invalidateQueries({ queryKey: ["queue"] });
    },
  });

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center gap-2">
        <input
          type="search"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          placeholder="Filter cards…"
          className="w-56 rounded-md border border-zinc-300 bg-white px-3 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        />
        <select
          value={deck}
          onChange={(e) => setDeck(e.target.value)}
          className="rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        >
          <option value="">All decks</option>
          {decks.data?.map((d) => (
            <option key={d.key} value={d.key === "(root)" ? "" : d.key}>
              {d.key} ({d.count})
            </option>
          ))}
        </select>
        <select
          value={status}
          onChange={(e) => setStatus(e.target.value)}
          className="rounded-md border border-zinc-300 bg-white px-2 py-1.5 text-sm dark:border-zinc-700 dark:bg-zinc-900"
        >
          <option value="active">Active</option>
          <option value="suspended">Suspended</option>
          <option value="orphaned">Orphaned</option>
          <option value="all">All</option>
        </select>
        {cards.data && (
          <span className="text-xs text-zinc-400">
            {cards.data.length} card{cards.data.length === 1 ? "" : "s"}
          </span>
        )}
      </div>

      {cards.isError && <p className="text-sm text-red-600">{String(cards.error)}</p>}

      <div className="overflow-x-auto rounded-lg border border-zinc-200 bg-white dark:border-zinc-800 dark:bg-zinc-900">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-zinc-200 text-left text-xs text-zinc-500 dark:border-zinc-800">
              <th className="px-3 py-2 font-medium">Front</th>
              <th className="px-3 py-2 font-medium">Deck</th>
              <th className="px-3 py-2 font-medium">Due</th>
              <th className="px-3 py-2 font-medium">State</th>
              <th className="px-3 py-2 font-medium">Reps</th>
              <th className="px-3 py-2 font-medium">Lapses</th>
              <th className="px-3 py-2" />
            </tr>
          </thead>
          <tbody>
            {cards.data?.map((c) => (
              <tr
                key={c.id}
                className="border-b border-zinc-100 last:border-0 dark:border-zinc-800/60"
              >
                <td className="max-w-56 truncate px-3 py-2 sm:max-w-xs" title={c.front}>
                  <Link to={`/notes/${c.note_path}`} className="hover:underline">
                    {c.front}
                  </Link>
                  {c.orphaned && (
                    <span className="ml-2 rounded bg-zinc-100 px-1.5 py-0.5 text-xs text-zinc-500 dark:bg-zinc-800">
                      orphaned
                    </span>
                  )}
                </td>
                <td className="px-3 py-2 text-zinc-500">{c.deck || "—"}</td>
                <td className="px-3 py-2 tabular-nums text-zinc-500">
                  {fmtDue(c.due, c.state)}
                </td>
                <td className="px-3 py-2 text-zinc-500">{stateLabels[c.state] ?? c.state}</td>
                <td className="px-3 py-2 tabular-nums text-zinc-500">{c.reps}</td>
                <td className="px-3 py-2 tabular-nums text-zinc-500">{c.lapses}</td>
                <td className="px-3 py-2 text-right">
                  <button
                    type="button"
                    disabled={suspend.isPending}
                    onClick={() =>
                      suspend.mutate({ id: c.id, suspended: !c.suspended })
                    }
                    className="rounded-md border border-zinc-300 px-2 py-1 text-xs text-zinc-600 hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
                  >
                    {c.suspended ? "Restore" : "Suspend"}
                  </button>
                </td>
              </tr>
            ))}
            {cards.data?.length === 0 && (
              <tr>
                <td colSpan={7} className="px-4 py-8 text-center text-sm text-zinc-400">
                  No cards match.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
