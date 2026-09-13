export async function api<T = any>(
  path: string,
  method = "GET",
  body?: unknown,
  headers?: Record<string, string>,
): Promise<T> {
  const res = await fetch("/api" + path, {
    method,
    credentials: "same-origin",
    headers: {
      ...(body instanceof FormData
        ? {}
        : { "Content-Type": "application/json" }),
      ...headers,
    },
    body:
      body === undefined
        ? undefined
        : body instanceof FormData
          ? body
          : JSON.stringify(body),
  });
  const value = await res.json();
  if (!res.ok) throw new Error(value.error || "请求失败");
  return value;
}
export const money = (n: number = 0) => "¥" + (n / 1e6).toFixed(2);
export const assetURL = (id: string) => "/api/assets/" + id;
export interface User {
  id: string;
  username: string;
  admin: boolean;
}
export interface Page {
  title: string;
  text: string;
  assetId: string;
  backgroundId: string;
  x: number;
  y: number;
  scale: number;
  cropX: number;
  cropY: number;
}
export interface Body {
  name: string;
  title: string;
  caption: string;
  tags: string;
  selling: string;
  audience: string;
  price: string;
  note: string;
  assetId: string;
  logoId: string;
  color: string;
  tone: string;
  template: string;
  background: string;
  pages: Page[];
  brand?: Body;
  product?: Body;
  productSourceId?: string;
  confirmed?: boolean;
  appliedJobs?: string[];
}
export interface Doc {
  id: string;
  kind: "brand" | "product" | "project";
  body: Body;
  revision: number;
  updated_at: string;
  expires_at?: string;
}
export interface Job {
  id: string;
  project_id: string;
  kind: string;
  state: string;
  error: string;
  reserve: number;
  charged: number;
  result: {
    draft?: {
      title: string;
      caption: string;
      tags: string;
      pages: { title: string; text: string }[];
    };
    assetId?: string;
    mode?: string;
  };
  input: { page: number };
  created_at: string;
}
export function emptyBody(): Body {
  return {
    name: "",
    title: "",
    caption: "",
    tags: "",
    selling: "",
    audience: "",
    price: "",
    note: "",
    assetId: "",
    logoId: "",
    color: "#ff2442",
    tone: "自然、真实",
    template: "simple",
    background: "#f5eee5",
    pages: [],
  };
}
export function pagesFor(assetId: string, name: string): Page[] {
  return [
    "发现日常的小美好",
    "值得关注的细节",
    "适合你的日常",
    "把喜欢带回家",
  ].map((text, i) => ({
    title: i === 0 ? name : text,
    text: i === 0 ? text : "补充真实商品信息",
    assetId,
    backgroundId: "",
    x: 50,
    y: 53,
    scale: 66,
    cropX: 50,
    cropY: 50,
  }));
}
