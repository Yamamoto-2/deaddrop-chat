import { bytesToB64, b64ToBytes } from "./crypto";
import type { FilePayload } from "./proto";

// Client-side cap on the raw file size. After base64 + JSON + encrypt + base64
// the wire frame is ~1.8x this, so it must stay under the server's
// MAX_FRAME_BYTES (10MB default).
export const MAX_FILE_BYTES = 5 * 1024 * 1024;

export async function readFilePayload(file: File): Promise<FilePayload> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  return {
    name: file.name,
    mime: file.type || "application/octet-stream",
    size: file.size,
    data: bytesToB64(bytes),
  };
}

// Decode a received file payload into an object URL (caller revokes it).
export function fileBlobUrl(f: FilePayload): string {
  const part = b64ToBytes(f.data) as unknown as BlobPart;
  return URL.createObjectURL(new Blob([part], { type: f.mime }));
}

export function humanSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}
