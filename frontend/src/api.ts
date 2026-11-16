// Typed wrapper over the generated Wails bindings. The generated classes carry
// helper methods, so we use plain interfaces here and cast at the boundary.
import * as Go from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';

export type Status = 'queued' | 'connecting' | 'downloading' | 'paused' | 'processing' | 'completed' | 'error';
export type Kind = 'file' | 'stream' | 'media';

export interface Segment { start: number; end: number; pos: number; active?: boolean }

export interface Download {
  id: string; url: string; fileName: string; dir: string; category: string; queueId: string;
  kind: Kind; status: Status; error?: string; size: number; downloaded: number; resumable: boolean;
  connections: number; segments?: Segment[]; referer?: string; userAgent?: string; cookies?: string;
  headers?: Record<string, string>; username?: string; password?: string; description?: string;
  speedLimit: number; postProcess?: string; added: string; lastTry: string; finished: string;
  speed: number; eta: number; activeConns: number;
}

export interface AddRequest {
  url: string; fileName?: string; dir?: string; category?: string; queueId?: string; kind?: Kind | '';
  referer?: string; userAgent?: string; cookies?: string; headers?: Record<string, string>;
  username?: string; password?: string; description?: string; connections?: number;
  speedLimit?: number; postProcess?: string; startNow?: boolean; size?: number;
}

export interface ProbeResult {
  url: string; finalUrl: string; fileName: string; size: number; resumable: boolean;
  contentType: string; kind: Kind; category: string; dir: string;
}

export interface Category { name: string; extensions: string[]; dir: string }
export interface SMBCredential { host: string; port: number; user: string; password: string; domain: string }
export interface Schedule { enabled: boolean; startTime: string; stopTime: string; days: number[]; onComplete: string }
export interface Queue { id: string; name: string; maxConcurrent: number; schedule: Schedule }

export interface Settings {
  defaultDir: string; maxConnections: number; globalSpeedLimit: number; maxRetries: number;
  retryDelaySec: number; timeoutSec: number; proxy: string; userAgent: string; bridgePort: number;
  bridgeToken: string; showAddDialog: boolean; showCompleteDialog: boolean; clipboardMonitor: boolean;
  browserIntercept: boolean; interceptTypes: string[]; skipSites: string[]; ffmpegPath: string;
  ytDlpPath: string; antivirusCmd: string; theme: 'system' | 'light' | 'dark';
  categories: Category[]; queues: Queue[]; smb: SMBCredential[];
}

export interface BridgeItem {
  url: string; fileName?: string; referer?: string; cookies?: string; userAgent?: string;
  headers?: Record<string, string>; kind?: Kind | ''; size?: number; mime?: string;
}
export interface BatchRequest { items: BridgeItem[]; batch: boolean; silent: boolean; page: string }

export interface GrabOptions {
  startUrl: string; mode: 'files' | 'mirror'; depth: number; sameHost: boolean; stayBelow: boolean;
  include: string[]; exclude: string[]; maxPages: number; concurrency: number;
  cookies: string; userAgent: string; username: string; password: string;
}
export interface GrabItem { url: string; saveAs: string; referer: string; type: string }
export interface GrabProgress { pages: number; found: number; current: string }

export interface Info {
  version: string; initError: string; bridgeError: string; ffmpeg: string; ytdlp: string;
  configDir: string; os: string;
}

const g = Go as any;

export const api = {
  info: (): Promise<Info> => g.GetInfo(),
  settings: (): Promise<Settings> => g.GetSettings(),
  saveSettings: (s: Settings): Promise<void> => g.SaveSettings(s),
  regenerateToken: (): Promise<string> => g.RegenerateToken(),
  list: (): Promise<Download[]> => g.ListDownloads(),
  probe: (r: AddRequest): Promise<ProbeResult> => g.ProbeURL(r),
  add: (r: AddRequest): Promise<Download> => g.AddDownload(r),
  addBatch: (r: AddRequest[]): Promise<number> => g.AddBatch(r),
  resume: (ids: string[]): Promise<void> => g.Resume(ids),
  pause: (ids: string[]): Promise<void> => g.Pause(ids),
  resumeAll: (): Promise<void> => g.ResumeAll(),
  pauseAll: (): Promise<void> => g.PauseAll(),
  remove: (ids: string[], files: boolean): Promise<void> => g.Remove(ids, files),
  removeCompleted: (): Promise<void> => g.RemoveCompleted(),
  redownload: (id: string): Promise<void> => g.Redownload(id),
  update: (id: string, r: AddRequest): Promise<void> => g.UpdateDownload(id, r),
  setSpeedLimit: (id: string, kbps: number): Promise<void> => g.SetSpeedLimit(id, kbps),
  startQueue: (id: string): Promise<void> => g.StartQueue(id),
  stopQueue: (id: string): Promise<void> => g.StopQueue(id),
  queueStates: (): Promise<Record<string, boolean>> => g.QueueStates(),
  moveToQueue: (ids: string[], q: string): Promise<void> => g.MoveToQueue(ids, q),
  openFile: (id: string): Promise<void> => g.OpenFile(id),
  openFolder: (id: string): Promise<void> => g.OpenFolder(id),
  chooseDir: (cur: string): Promise<string> => g.ChooseDirectory(cur),
  chooseFile: (title: string): Promise<string> => g.ChooseFile(title),
  testSMB: (dir: string): Promise<void> => g.TestSMB(dir),
  copy: (s: string): Promise<void> => g.CopyText(s),
  grabScan: (o: GrabOptions): Promise<GrabItem[]> => g.GrabberScan(o),
  grabCancel: (): Promise<void> => g.GrabberCancel(),
  grabAdd: (r: { items: GrabItem[]; dir: string; queueId: string; startNow: boolean; cookies: string; username: string; password: string }): Promise<number> => g.GrabberAdd(r),
};

export function on<T>(event: string, fn: (data: T) => void): () => void {
  return EventsOn(event, fn as (...d: any[]) => void);
}

export function errText(e: unknown): string {
  if (typeof e === 'string') return e;
  if (e instanceof Error) return e.message;
  return JSON.stringify(e);
}
