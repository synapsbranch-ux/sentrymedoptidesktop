/**
 * D5: a consultation recording is written to IndexedDB one chunk at a time while
 * it runs, so audio already captured survives the tab being closed, the browser
 * crashing or the connection dropping. Nothing here talks to the network; the
 * chunks are uploaded by the recorder when it stops, or offered for recovery the
 * next time that consultation is opened.
 */

const databaseName = "sentrymed-recordings";
const storeName = "chunks";

export interface RecoveredRecording {
  encounterId: string;
  chunks: Blob[];
  mimeType: string;
  durationSeconds: number;
  startedAt: string;
}

function openDatabase(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    if (typeof indexedDB === "undefined") {
      reject(new Error("This browser does not provide local storage for recordings."));
      return;
    }
    const request = indexedDB.open(databaseName, 1);
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains(storeName)) request.result.createObjectStore(storeName, { keyPath: "encounterId" });
    };
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("Could not open local recording storage."));
  });
}

function run<T>(mode: IDBTransactionMode, action: (store: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  return openDatabase().then((database) => new Promise<T>((resolve, reject) => {
    const transaction = database.transaction(storeName, mode);
    const request = action(transaction.objectStore(storeName));
    request.onsuccess = () => resolve(request.result);
    request.onerror = () => reject(request.error ?? new Error("Local recording storage failed."));
    transaction.oncomplete = () => database.close();
  }));
}

/** Appends one MediaRecorder chunk. Failures are swallowed: losing the safety
 *  net must never stop the recording that is in progress. */
export async function persistChunk(encounterId: string, chunk: Blob, mimeType: string, durationSeconds: number) {
  try {
    const existing = await run<RecoveredRecording | undefined>("readonly", (store) => store.get(encounterId));
    const record: RecoveredRecording = {
      encounterId,
      chunks: [...(existing?.chunks ?? []), chunk],
      mimeType,
      durationSeconds,
      startedAt: existing?.startedAt ?? new Date().toISOString(),
    };
    await run("readwrite", (store) => store.put(record));
  } catch {
    // Recording continues in memory; only the crash-recovery copy is lost.
  }
}

export async function loadRecovered(encounterId: string): Promise<RecoveredRecording | null> {
  try {
    const record = await run<RecoveredRecording | undefined>("readonly", (store) => store.get(encounterId));
    return record && record.chunks.length > 0 ? record : null;
  } catch {
    return null;
  }
}

export async function clearRecovered(encounterId: string) {
  try {
    await run("readwrite", (store) => store.delete(encounterId));
  } catch {
    // Nothing to do: a stale copy is offered for recovery again, never lost.
  }
}
