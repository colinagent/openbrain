import type { Content, JsonContent, Message, Meta, TextContent } from "./types.js";

export function textContent(text = "", annotations?: Record<string, unknown>): TextContent {
  return annotations ? { type: "text", text, annotations } : { type: "text", text };
}

export function jsonContent(payload: unknown): JsonContent {
  return { type: "json", payload };
}

export function userMessage(content: string): Message {
  return { role: "user", content };
}

export function assistantMessage(content: string): Message {
  return { role: "assistant", content };
}

export function cloneMeta(meta: Meta | undefined): Meta {
  return meta ? { ...meta } : {};
}

export function mergeMeta(base: Meta | undefined, extra: Meta | undefined): Meta {
  return { ...cloneMeta(base), ...cloneMeta(extra) };
}

export function metaString(meta: Meta | undefined, key: string): string {
  const value = meta?.[key];
  if (value === undefined || value === null) {
    return "";
  }
  return String(value).trim();
}

export function contentText(content: Content | undefined): string {
  if (!content) {
    return "";
  }
  switch (content.type) {
    case "text":
      return content.text;
    case "json":
      return jsonPayloadText(content.payload);
    default:
      return JSON.stringify(content);
  }
}

function jsonPayloadText(value: unknown): string {
  if (typeof value === "string") {
    return value;
  }
  if (Array.isArray(value)) {
    return value.map(jsonPayloadText).filter(Boolean).join("\n");
  }
  if (value && typeof value === "object") {
    const obj = value as Record<string, unknown>;
    for (const key of ["prompt", "message", "text", "input", "query", "instruction"]) {
      if (typeof obj[key] === "string" && obj[key].trim()) {
        return obj[key];
      }
    }
    if ("content" in obj) {
      return jsonPayloadText(obj.content);
    }
    if ("messages" in obj) {
      return jsonPayloadText(obj.messages);
    }
  }
  return JSON.stringify(value, null, 2);
}
