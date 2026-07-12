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

export type NoteType = "reading" | "thought";

export interface NoteSummary {
  path: string;
  title: string;
  stage: Stage;
  status: string;
  type: NoteType;
  tags: string[];
  sources: string[];
  mtime: number;
  card_count: number;
}

export interface NoteLink {
  target: string;
  to_path: string; // "" = red link (no matching note)
}

export interface NoteRef {
  path: string;
  title: string;
}

export interface NoteDetail extends NoteSummary {
  content: string;
  links: NoteLink[];
  backlinks: NoteRef[];
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
  cram?: boolean;
  project?: number;
  deadline?: string;
  days_left?: number;
  target_new_today?: number;
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

export async function apiPatch<T>(path: string, body: unknown): Promise<T> {
  return handle<T>(
    await fetch(path, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    }),
  );
}

export async function apiDelete<T>(path: string): Promise<T> {
  return handle<T>(await fetch(path, { method: "DELETE" }));
}

export interface StatsSummary {
  total_cards: number;
  suspended_cards: number;
  orphaned_cards: number;
  due_now: number;
  new_remaining: number;
  reviews_today: number;
  time_today_ms: number;
  retention_30: number; // -1 = no data
  avg_review_ms: number;
}

export interface DayCount {
  date: string; // YYYY-MM-DD
  count: number;
}

export interface ForecastResponse {
  forecast: DayCount[];
  overdue: number;
}

export interface TimeBucket {
  key: string;
  ms: number;
  count: number;
}

export interface TimeResponse {
  by_kind: TimeBucket[];
  by_deck: TimeBucket[];
}

export interface CardInfo {
  id: string;
  note_path: string;
  type: string;
  front: string;
  back: string;
  deck: string;
  tags: string[];
  suspended: boolean;
  orphaned: boolean;
  due: string;
  state: number;
  reps: number;
  lapses: number;
}

export interface SearchHit {
  path: string;
  title: string;
  snippet: string;
}

export interface SourceSearchHit {
  source_id: number;
  title: string;
  snippet: string;
}

export interface SearchResponse {
  notes: SearchHit[];
  sources: SourceSearchHit[];
}

export interface Source {
  id: number;
  kind: "pdf" | "url" | "book" | "scan";
  key: string;
  path: string;
  title: string;
  meta: string;
  added_at: string;
}

// scanPageCount reads a scan source's page count from its meta JSON.
// Page n (1-based) is served at /api/sources/{id}/page/{n}.
export function scanPageCount(src: Source): number {
  try {
    const meta = JSON.parse(src.meta) as { pages?: string[] };
    return meta.pages?.length ?? 0;
  } catch {
    return 0;
  }
}

// POST /api/notes — the workbench save: a transcribed reading note or a
// born-digital Thought (server places thoughts in thoughts/YYYY-MM-DD-slug.md).
export interface CreateNoteRequest {
  title: string;
  type?: NoteType; // default "reading"
  stage?: Stage; // reading notes only
  folder?: string; // reading notes only; relative, slugified server-side
  tags?: string[];
  sources?: string[]; // citation keys; must exist
  body?: string;
  transcribed_by?: string; // model id when the draft came from AI
  elapsed_ms?: number; // workbench editing time → note_edit event
}

// POST /api/llm/transcribe {source_id, model?} — AI transcript draft of a
// scan's pages; nothing is saved until the human submits the note.
export interface TranscribeResponse {
  text: string;
  model: string;
  usage: GenerateResponse["usage"];
  source_key: string;
}

export interface LLMStatus {
  configured: boolean;
  model: string;
  daily_tokens: number;
  tokens_today: number;
  cost_today: number;
}

export interface LLMModel {
  id: string;
  name: string;
  pricing: { prompt: string; completion: string };
}

export interface ProposedCard {
  front: string;
  back: string;
}

export interface GenerateResponse {
  cards: ProposedCard[];
  model: string;
  usage: {
    prompt_tokens: number;
    completion_tokens: number;
    cost: number;
  };
}

export interface AcceptResponse {
  added: number;
  card_ids: string[];
}

export interface GradeResult {
  verdict: "correct" | "partial" | "incorrect";
  feedback: string;
  missing?: string;
  suggested_rating: number;
}

export interface GradeResponse {
  grade: GradeResult;
  model: string;
  usage: GenerateResponse["usage"];
  event_id: number;
}

export interface ChatMessage {
  role: "user" | "assistant";
  content: string;
}

export interface TutorResponse {
  reply: string;
  model: string;
  usage: GenerateResponse["usage"];
}

export interface WikiGenerateResponse {
  path: string;
  model?: string;
  usage?: GenerateResponse["usage"];
  existing: boolean;
}

export type RootCause =
  | "memory"
  | "concept"
  | "manipulation"
  | "classification"
  | "prerequisite"
  | "overconfidence"
  | "careless"
  | "source";

// Human labels for the fixed root-cause taxonomy, in display order.
export const causeLabels: Record<RootCause, string> = {
  memory: "memory lapse",
  concept: "conceptual confusion",
  manipulation: "algebra / manipulation",
  classification: "wrong method choice",
  prerequisite: "missing prerequisite",
  overconfidence: "overconfidence",
  careless: "careless slip",
  source: "misread source",
};

export interface TriageItem {
  event_id: number;
  ts: string;
  kind: "card_review" | "free_text_answer";
  card_id: string;
  card_front: string;
  card_back: string;
  deck: string;
  answer?: string;
  verdict?: string;
}

export interface TriageResponse {
  items: TriageItem[];
  causes: RootCause[];
}

export interface ErrorEntry {
  id: number;
  event_id: number;
  card_id: string;
  card_front: string;
  deck: string;
  note_path: string;
  root_cause: RootCause;
  note: string;
  repair_action: string;
  repair_note_path: string;
  repair_due: string;
  created_at: string;
  resolved_at: string;
}

export interface CauseCount {
  cause: RootCause;
  deck?: string;
  open: number;
  total: number;
}

export interface ErrorStatsResponse {
  by_cause: CauseCount[];
  by_deck: CauseCount[];
}

export interface ReviewResponse {
  schedule: unknown;
  event_id: number;
}

export interface StaleNote {
  path: string;
  title: string;
  stage: string;
  idle_days: number;
}

export interface TodayResponse {
  summary: StatsSummary;
  stale_notes: StaleNote[];
  open_questions: number;
  oldest_questions: OpenQuestion[];
  leeches: number;
  error_triage: number;
  open_errors: number;
  repairs_due: ErrorEntry[];
}

export interface SyncResult {
  notes: number;
  cards_created: number;
  cards_updated: number;
  cards_orphaned: number;
  anchors_written: number;
  open_questions: number;
}

export interface Project {
  id: number;
  name: string;
  deadline: string; // "" = none
  dirs: string[];
  created_at: string;
}

export interface ProjectInfo extends Project {
  total_cards: number;
  new_cards: number;
  due_now: number;
  days_left: number | null; // null when no deadline; <=0 means passed
}
