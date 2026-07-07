import { useCallback, useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  apiGet,
  apiPost,
  type QueueCard,
  type QueueResponse,
  type TimeBucket,
} from "../api";
import Markdown from "../Markdown";

const ratings = [
  { value: 1, label: "Again", key: "1", color: "bg-red-600 hover:bg-red-500" },
  { value: 2, label: "Hard", key: "2", color: "bg-amber-600 hover:bg-amber-500" },
  { value: 3, label: "Good", key: "3", color: "bg-emerald-600 hover:bg-emerald-500" },
  { value: 4, label: "Easy", key: "4", color: "bg-sky-600 hover:bg-sky-500" },
] as const;

function assetBaseFor(notePath: string): string {
  const i = notePath.lastIndexOf("/");
  return i === -1 ? "" : notePath.slice(0, i + 1);
}

export default function Review() {
  const queryClient = useQueryClient();
  const [deck, setDeck] = useState("");
  const [cram, setCram] = useState(false);

  const decks = useQuery({
    queryKey: ["decks"],
    queryFn: () => apiGet<TimeBucket[]>("/api/decks"),
  });

  const params = new URLSearchParams();
  if (deck) params.set("deck", deck);
  if (cram && deck) params.set("cram", "1");
  const queue = useQuery({
    queryKey: ["queue", deck, cram],
    queryFn: () => apiGet<QueueResponse>(`/api/queue?${params}`),
    staleTime: Infinity,
    refetchOnMount: "always",
  });

  const [position, setPosition] = useState(0);
  const [revealed, setRevealed] = useState(false);
  const [done, setDone] = useState(0);
  const shownAt = useRef(Date.now());
  // Positions of past ratings, so undo can step back to the right card.
  const undoStack = useRef<number[]>([]);

  useEffect(() => {
    setPosition(0);
    setRevealed(false);
    setDone(0);
    undoStack.current = [];
  }, [deck, cram]);

  const cards: QueueCard[] = queue.data ? [...queue.data.due, ...queue.data.new] : [];
  const card = cards[position];
  const isCram = queue.data?.cram === true;

  const rate = useCallback(
    async (rating: number) => {
      if (!card || !revealed) return;
      const elapsed = Date.now() - shownAt.current;
      undoStack.current.push(position);
      setRevealed(false);
      setPosition((p) => p + 1);
      setDone((d) => d + 1);
      shownAt.current = Date.now();
      try {
        await apiPost("/api/reviews", {
          card_id: card.id,
          rating,
          elapsed_ms: elapsed,
          cram: isCram || undefined,
        });
      } finally {
        if (position + 1 >= cards.length) {
          queryClient.invalidateQueries({ queryKey: ["queue"] });
        }
      }
    },
    [card, revealed, position, cards.length, isCram, queryClient],
  );

  const undo = useCallback(async () => {
    const prev = undoStack.current.pop();
    if (prev === undefined) return;
    try {
      await apiPost("/api/reviews/undo");
      setPosition(prev);
      setRevealed(false);
      setDone((d) => Math.max(0, d - 1));
      shownAt.current = Date.now();
    } catch {
      undoStack.current.push(prev); // nothing was undone server-side
    }
  }, []);

  const bury = useCallback(async () => {
    if (!card) return;
    setRevealed(false);
    setPosition((p) => p + 1);
    shownAt.current = Date.now();
    await apiPost(`/api/cards/${encodeURIComponent(card.id)}/bury`);
  }, [card]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      if (e.code === "Space") {
        e.preventDefault();
        setRevealed(true);
      } else if (revealed && e.key >= "1" && e.key <= "4") {
        void rate(Number(e.key));
      } else if (e.key === "u") {
        void undo();
      } else if (e.key === "b") {
        void bury();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [rate, revealed, undo, bury]);

  useEffect(() => {
    shownAt.current = Date.now();
  }, [card?.id]);

  if (queue.isPending) return <p className="text-sm text-zinc-500">Loading queue…</p>;
  if (queue.isError) return <p className="text-sm text-red-600">{String(queue.error)}</p>;

  const scopeBar = (
    <div className="mx-auto flex max-w-2xl flex-wrap items-center gap-2 text-xs text-zinc-500">
      <select
        value={deck}
        onChange={(e) => {
          setDeck(e.target.value);
          if (!e.target.value) setCram(false);
        }}
        className="rounded-md border border-zinc-300 bg-white px-2 py-1 text-xs dark:border-zinc-700 dark:bg-zinc-900"
      >
        <option value="">All decks</option>
        {decks.data?.map((d) => (
          <option key={d.key} value={d.key === "(root)" ? "" : d.key}>
            {d.key}
          </option>
        ))}
      </select>
      {deck && (
        <label
          className="flex cursor-pointer items-center gap-1.5"
          title="Exam prep: every card in the deck, weakest first, ignoring due dates"
        >
          <input
            type="checkbox"
            checked={cram}
            onChange={(e) => setCram(e.target.checked)}
          />
          Cram
        </label>
      )}
      {isCram && (
        <span className="rounded bg-amber-100 px-1.5 py-0.5 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300">
          cram — schedule still updates, due dates ignored
        </span>
      )}
      <div className="flex-1" />
      {done > 0 && (
        <button
          type="button"
          onClick={() => void undo()}
          className="rounded-md border border-zinc-300 px-2 py-1 text-xs text-zinc-600 hover:bg-zinc-100 dark:border-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-800"
        >
          Undo <span className="opacity-60">u</span>
        </button>
      )}
    </div>
  );

  if (!card) {
    return (
      <div className="space-y-4">
        {scopeBar}
        <div className="rounded-lg border border-dashed border-zinc-300 p-10 text-center dark:border-zinc-700">
          <h1 className="text-xl font-semibold">
            {done > 0 ? `Done — ${done} card${done === 1 ? "" : "s"} reviewed 🎉` : "Nothing due"}
          </h1>
          <p className="mt-2 text-sm text-zinc-500">
            {done > 0
              ? "Queue cleared for now."
              : deck
                ? "No cards due in this deck today. Try Cram for exam prep."
                : "No cards due and no new cards remaining today."}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-2xl space-y-4">
      {scopeBar}
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
          <Markdown assetBase={assetBaseFor(card.note_path)}>{card.front}</Markdown>
        </div>
        {revealed && card.back && (
          <>
            <hr className="my-4 border-zinc-200 dark:border-zinc-700" />
            <div className="prose prose-zinc max-w-none dark:prose-invert">
              <Markdown assetBase={assetBaseFor(card.note_path)}>{card.back}</Markdown>
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

      <p className="text-center text-xs text-zinc-400">
        <button type="button" onClick={() => void bury()} className="hover:underline">
          Bury until tomorrow <span className="opacity-60">b</span>
        </button>
      </p>
    </div>
  );
}
