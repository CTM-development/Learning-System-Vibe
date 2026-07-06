// Thin fetch wrapper for the Go backend. All endpoints live under /api.

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function handle<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let message = await res.text();
    try {
      message = (JSON.parse(message) as { error?: string }).error ?? message;
    } catch {
      // plain-text error body
    }
    throw new ApiError(res.status, message);
  }
  return res.json() as Promise<T>;
}

export async function apiGet<T>(path: string): Promise<T> {
  return handle<T>(await fetch(path));
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return handle<T>(
    await fetch(path, {
      method: "POST",
      headers: body !== undefined ? { "Content-Type": "application/json" } : {},
      body: body !== undefined ? JSON.stringify(body) : undefined,
    }),
  );
}

export interface Health {
  status: string;
  version: string;
}

export type Stage = "" | "skim" | "deep" | "synthesis";

export interface NoteSummary {
  path: string;
  title: string;
  stage: Stage;
  status: string;
  tags: string[];
  sources: string[];
  mtime: number;
  card_count: number;
}

export interface NoteDetail extends NoteSummary {
  content: string;
}

export interface OpenQuestion {
  id: number;
  note_path: string;
  text: string;
  status: "open" | "carded" | "folded" | "dropped";
  created_at: string;
}

export interface QueueCard {
  id: string;
  note_path: string;
  type: string;
  front: string;
  back: string;
  deck: string;
  due: string;
  state: number;
}

export interface QueueResponse {
  due: QueueCard[];
  new: QueueCard[];
  new_remaining: number;
}

export interface Session {
  id: number;
  kind: "productivity" | "learning";
  started_at: string;
  ended_at?: string;
  note: string;
}

export interface SessionsResponse {
  active: Session | null;
  recent: Session[];
  serverTime: string;
}

export interface SyncResult {
  notes: number;
  cards_created: number;
  cards_updated: number;
  cards_orphaned: number;
  anchors_written: number;
  open_questions: number;
}
