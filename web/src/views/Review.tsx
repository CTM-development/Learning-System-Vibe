import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { apiGet, apiPost, type QueueCard, type QueueResponse } from "../api";
import Markdown from "../Markdown";

const ratings = [
  { value: 1, label: "Again", key: "1", color: "bg-red-600 hover:bg-red-500" },
  { value: 2, label: "Hard", key: "2", color: "bg-amber-600 hover:bg-amber-500" },
  { value: 3, label: "Good", key: "3", color: "bg-emerald-600 hover:bg-emerald-500" },
  { value: 4, label: "Easy", key: "4", color: "bg-sky-600 hover:bg-sky-500" },
] as const;

export default function Review() {
  const queryClient = useQueryClient();
  const queue = useQuery({
    queryKey: ["queue"],
    queryFn: () => apiGet<QueueResponse>("/api/queue"),
    staleTime: Infinity,
    refetchOnMount: "always",
  });

  const [position, setPosition] = useState(0);
  const [revealed, setRevealed] = useState(false);
  const [done, setDone] = useState(0);
  const shownAt = useRef(Date.now());

  const cards: QueueCard[] = queue.data ? [...queue.data.due, ...queue.data.new] : [];
  const card = cards[position];

  const rate = useCallback(
    async (rating: number) => {
      if (!card || !revealed) return;
      const elapsed = Date.now() - shownAt.current;
      setRevealed(false);
      setPosition((p) => p + 1);
      setDone((d) => d + 1);
      shownAt.current = Date.now();
      try {
        await apiPost("/api/reviews", {
          card_id: card.id,
          rating,
          elapsed_ms: elapsed,
        });
      } finally {
        if (position + 1 >= cards.length) {
          queryClient.invalidateQueries({ queryKey: ["queue"] });
        }
      }
    },
    [card, revealed, position, cards.length, queryClient],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.code === "Space") {
        e.preventDefault();
        setRevealed(true);
      } else if (revealed && e.key >= "1" && e.key <= "4") {
        void rate(Number(e.key));
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [rate, revealed]);

  useEffect(() => {
    shownAt.current = Date.now();
  }, [card?.id]);

  if (queue.isPending) return <p className="text-sm text-zinc-500">Loading queue…</p>;
  if (queue.isError) return <p className="text-sm text-red-600">{String(queue.error)}</p>;

  if (!card) {
    return (
      <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center dark:border-zinc-700">
        <h1 className="text-xl font-semibold">
          {done > 0 ? `Done — ${done} card${done === 1 ? "" : "s"} reviewed 🎉` : "Nothing due"}
        </h1>
        <p className="mt-2 text-sm text-zinc-500">
          {done > 0
            ? "Queue cleared for now."
            : "No cards due and no new cards remaining today."}
        </p>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl space-y-4">
      <div className="flex items-center justify-between text-xs text-zinc-500">
        <span>
          {position + 1} / {cards.length}
          {card.state === 0 && (
            <span className="ml-2 rounded bg-sky-100 px-1.5 py-0.5 text-sky-700 dark:bg-sky-900/40 dark:text-sky-300">
              new
            </span>
          )}
        </span>
        <span>{card.deck || card.note_path}</span>
      </div>

      <div
        className="min-h-48 cursor-pointer rounded-lg border border-zinc-200 bg-white p-6 dark:border-zinc-800 dark:bg-zinc-900"
        onClick={() => setRevealed(true)}
      >
        <div className="prose prose-zinc max-w-none dark:prose-invert">
          <Markdown>{card.front}</Markdown>
        </div>
        {revealed && card.back && (
          <>
            <hr className="my-4 border-zinc-200 dark:border-zinc-700" />
            <div className="prose prose-zinc max-w-none dark:prose-invert">
              <Markdown>{card.back}</Markdown>
            </div>
          </>
        )}
      </div>

      {revealed ? (
        <div className="grid grid-cols-4 gap-2">
          {ratings.map((r) => (
            <button
              key={r.value}
              type="button"
              onClick={() => void rate(r.value)}
              className={`rounded-md px-3 py-3 text-sm font-medium text-white transition-colors ${r.color}`}
            >
              {r.label}
              <span className="ml-1 hidden text-white/60 sm:inline">{r.key}</span>
            </button>
          ))}
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setRevealed(true)}
          className="w-full rounded-md bg-zinc-900 px-3 py-3 text-sm font-medium text-white hover:bg-zinc-700 dark:bg-zinc-100 dark:text-zinc-900 dark:hover:bg-zinc-300"
        >
          Reveal <span className="ml-1 hidden opacity-60 sm:inline">Space</span>
        </button>
      )}
    </div>
  );
}
