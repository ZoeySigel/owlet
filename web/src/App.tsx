import { useEffect, useRef, useState } from "react";
import {
  Plus,
  Grid2X2,
  Package,
  Palette,
  Clock,
  Shield,
  LogOut,
  ArrowLeft,
  Download,
  Sparkles,
  Check,
  ChevronRight,
  ImagePlus,
  Trash2,
  Copy,
  History,
  Search,
  UserRound,
} from "lucide-react";
import JSZip from "jszip";
import {
  api,
  money,
  assetURL,
  emptyBody,
  pagesFor,
  type Body,
  type Doc,
  type Job,
  type Page,
  type User,
} from "./api";
import { draw, png, download } from "./render";

function Preview({
  body,
  page,
  index = 0,
}: {
  body: Body;
  page: Page;
  index?: number;
}) {
  const ref = useRef<HTMLCanvasElement>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let valid = true;
    const c = document.createElement("canvas");
    draw(c, body, page, index)
      .then((over) => {
        if (valid && ref.current) {
          ref.current.width = c.width;
          ref.current.height = c.height;
          ref.current.getContext("2d")!.drawImage(c, 0, 0);
          setError(over ? "文字溢出，请缩短内容" : "");
        }
      })
      .catch(() => valid && setError("图片加载失败，请检查素材"));
    return () => {
      valid = false;
    };
  }, [body, page, index]);
  return (
    <div className="preview">
      <canvas ref={ref} />
      {error && <span className="preview-warning">{error}</span>}
    </div>
  );
}
function Field({
  label,
  value,
  onChange,
  multiline = false,
  type = "text",
}: {
  label: string;
  value: string;
  onChange: (s: string) => void;
  multiline?: boolean;
  type?: string;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {multiline ? (
        <textarea
          value={value}
          onChange={(e) => onChange(e.target.value)}
          rows={4}
        />
      ) : (
        <input
          type={type}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
    </label>
  );
}
const stateName: Record<string, string> = {
  queued: "排队中",
  running: "生成中",
  succeeded: "已完成",
  failed: "失败",
  cancelled: "已取消",
  outcome_unknown: "待核对",
};
export default function App() {
  const [me, setMe] = useState<User | null>(null),
    [loaded, setLoaded] = useState(false),
    [docs, setDocs] = useState<Doc[]>([]),
    [jobs, setJobs] = useState<Job[]>([]),
    [usage, setUsage] = useState<any>(null),
    [tab, setTab] = useState("projects"),
    [active, setActive] = useState<Doc | null>(null),
    [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false),
    [search, setSearch] = useState("");
  async function refresh() {
    const [d, j, u] = await Promise.all([
      api<Doc[]>("/documents"),
      api<Job[]>("/jobs"),
      api("/usage"),
    ]);
    setDocs(d);
    setJobs(j);
    setUsage(u);
  }
  useEffect(() => {
    api<User>("/me")
      .then(setMe)
      .catch(() => {})
      .finally(() => setLoaded(true));
  }, []);
  useEffect(() => {
    if (!me) return;
    refresh().catch((e) => setMessage(e.message));
    const timer = setInterval(() => {
      api<Job[]>("/jobs")
        .then(setJobs)
        .catch(() => {});
      api("/usage")
        .then(setUsage)
        .catch(() => {});
    }, 3000);
    return () => clearInterval(timer);
  }, [me]);
  async function action(fn: () => Promise<void>) {
    setBusy(true);
    setMessage("");
    try {
      await fn();
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function save(d: Doc) {
    const saved = await api<Doc>(
      "/documents" + (d.id ? "/" + d.id : ""),
      d.id ? "PUT" : "POST",
      { kind: d.kind, body: d.body, revision: d.revision },
    );
    setDocs((old) => [saved, ...old.filter((x) => x.id !== saved.id)]);
    return saved;
  }
  async function upload(file: File) {
    const f = new FormData();
    f.append("file", file);
    return (await api<{ id: string }>("/assets", "POST", f)).id;
  }
  async function create(product?: Doc) {
    await action(async () => {
      const p = product?.body;
      const brand = docs.find((d) => d.kind === "brand")?.body;
      const name = p?.name || "未命名创作";
      const b = {
        ...emptyBody(),
        name,
        title: name,
        assetId: p?.assetId || "",
        selling: p?.selling || "",
        audience: p?.audience || "",
        price: p?.price || "",
        note: p?.note || "",
        brand,
        product: p,
        productSourceId: product?.id,
        color: brand?.color || "#ff2442",
        pages: pagesFor(p?.assetId || "", name),
      };
      const d = await save({
        id: "",
        kind: "project",
        body: b,
        revision: 0,
        updated_at: "",
      });
      setActive(d);
    });
  }
  if (!loaded) return <div className="loading">Owlet 正在准备你的工作台…</div>;
  if (!me) return <Login onLogin={setMe} />;
  return (
    <div className="shell">
      <aside className="sidebar">
        <a
          className="brand"
          href="#"
          onClick={() => {
            setActive(null);
            setTab("projects");
          }}
        >
          owlet<span>创作工作台</span>
        </a>
        <nav>
          {[
            ["projects", "我的创作", Grid2X2],
            ["products", "商品资料", Package],
            ["brand", "品牌风格", Palette],
            ["tasks", "生成记录", Clock],
            ...(me.admin ? [["admin", "管理后台", Shield]] : []),
          ].map(([key, title, Icon]: any) => (
            <button
              key={key}
              className={!active && tab === key ? "nav active" : "nav"}
              onClick={() => {
                setActive(null);
                setTab(key);
              }}
            >
              <Icon size={22} />
              {title}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="quota-mini">
            <span>今日可用</span>
            <strong>{money(usage?.daily?.remaining)}</strong>
            <small>全站本月剩余 {money(usage?.monthly?.remaining)}</small>
          </div>
          <button
            className="profile"
            onClick={() => {
              setActive(null);
              setTab("account");
            }}
          >
            <span className="avatar">{me.username[0].toUpperCase()}</span>
            <span>
              {me.username}
              <small>{me.admin ? "管理员" : "创作者"}</small>
            </span>
            <ChevronRight size={15} />
          </button>
        </div>
      </aside>
      <main>
        <header className="topbar">
          <span className="breadcrumb">
            创作空间 <span>/</span>{" "}
            {active
              ? "编辑图文"
              : (
                  {
                    projects: "我的创作",
                    products: "商品资料",
                    brand: "品牌风格",
                    tasks: "生成记录",
                    admin: "管理后台",
                    account: "个人中心",
                  } as any
                )[tab]}
          </span>
          <div className="mode">
            {usage?.mode === "mock"
              ? "模拟体验 · 不产生 API 费用"
              : "BigModel 已接入"}
          </div>
        </header>
        {message && (
          <div className="alert" role="alert">
            {message}
            <button onClick={() => setMessage("")}>关闭</button>
          </div>
        )}
        {active ? (
          <Editor
            key={active.id}
            initial={active}
            sources={docs}
            save={save}
            upload={upload}
            jobs={jobs.filter((j) => j.project_id === active.id)}
            notify={setMessage}
            back={() => {
              setActive(null);
              refresh();
            }}
          />
        ) : (
          <>
            {tab === "projects" && (
              <section className="content">
                <div className="section-title">
                  <div>
                    <p className="eyebrow">让每一份好物，都有好故事</p>
                    <h1>
                      我的创作
                      <span>
                        {docs.filter((d) => d.kind === "project").length}
                      </span>
                    </h1>
                  </div>
                  <button
                    className="primary"
                    disabled={busy}
                    onClick={() => create()}
                  >
                    <Plus size={18} />
                    新建图文
                  </button>
                </div>
                <div className="filterbar">
                  <span className="pill">全部作品</span>
                  <label className="search">
                    <Search size={17} />
                    <input
                      placeholder="搜索作品名称"
                      value={search}
                      onChange={(e) => setSearch(e.target.value)}
                    />
                  </label>
                </div>
                <div className="project-grid">
                  <button
                    className="new-card"
                    disabled={busy}
                    onClick={() => create()}
                  >
                    <span className="plus-circle">
                      <Plus size={32} />
                    </span>
                    <strong>从一个想法开始</strong>
                    <small>添加商品，写下你的第一篇图文</small>
                  </button>
                  {docs
                    .filter(
                      (d) =>
                        d.kind === "project" && d.body.name.includes(search),
                    )
                    .map((d) => (
                      <article className="project-card" key={d.id}>
                        <button
                          className="cover-button"
                          onClick={() => setActive(d)}
                        >
                          {d.body.pages[0] && (
                            <Preview body={d.body} page={d.body.pages[0]} />
                          )}
                        </button>
                        <div className="card-caption">
                          <strong>{d.body.name}</strong>
                          <div>
                            <span>
                              {new Date(d.updated_at).toLocaleDateString()} ·{" "}
                              {d.body.pages.length} 页
                            </span>
                            <button
                              aria-label="删除项目"
                              onClick={() => {
                                if (
                                  confirm(
                                    "删除该项目及历史版本？费用不会退回。",
                                  )
                                )
                                  action(async () => {
                                    await api("/documents/" + d.id, "DELETE");
                                    await refresh();
                                  });
                              }}
                            >
                              <Trash2 size={15} />
                            </button>
                          </div>
                          {d.expires_at &&
                            new Date(d.expires_at).getTime() - Date.now() <
                              3 * 86400000 && (
                              <small className="danger">
                                即将到期，请编辑续期或下载
                              </small>
                            )}
                        </div>
                      </article>
                    ))}
                </div>
              </section>
            )}
            {tab === "products" && (
              <Catalog
                kind="product"
                docs={docs}
                save={save}
                upload={upload}
                notify={setMessage}
                create={create}
              />
            )}
            {tab === "brand" && (
              <Catalog
                kind="brand"
                docs={docs}
                save={save}
                upload={upload}
                notify={setMessage}
                create={create}
              />
            )}
            {tab === "tasks" && (
              <section className="content">
                <h1>生成记录</h1>
                <p className="muted">
                  成功结果随时可用。结果不明的调用保留额度，等待管理员核对。
                </p>
                <JobList jobs={jobs} notify={setMessage} />
              </section>
            )}
            {tab === "admin" && <Admin notify={setMessage} />}
            {tab === "account" && (
              <section className="content">
                <h1>个人中心</h1>
                <div className="panel account">
                  <UserRound size={36} />
                  <h2>{me.username}</h2>
                  <p>每日限额 ¥5.00 · 全站每月限额 ¥50.00</p>
                  {["daily", "monthly"].map((k) => (
                    <div key={k} className="usage-row">
                      <strong>{k === "daily" ? "个人今日" : "全站本月"}</strong>
                      <span>已用 {money(usage?.[k]?.spent)}</span>
                      <span>预留 {money(usage?.[k]?.reserved)}</span>
                      <span>剩余 {money(usage?.[k]?.remaining)}</span>
                    </div>
                  ))}
                  <p className="muted">
                    按北京时间重置。已计费的生成即使不满意也计入额度。
                  </p>
                  <button
                    onClick={() =>
                      action(async () => {
                        await api("/auth/logout", "POST", {});
                        setMe(null);
                      })
                    }
                  >
                    <LogOut size={16} />
                    退出登录
                  </button>
                </div>
              </section>
            )}
          </>
        )}
      </main>
    </div>
  );
}
function Login({ onLogin }: { onLogin: (u: User) => void }) {
  const [register, setRegister] = useState(false),
    [username, setUsername] = useState(""),
    [password, setPassword] = useState(""),
    [invite, setInvite] = useState(""),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(false);
  return (
    <div className="login-page">
      <div className="login-art">
        <span className="brand">owlet</span>
        <div className="sample-note">
          <div className="sample-orb" />
          <small>MAKE ROOM FOR GOOD THINGS</small>
          <h1>
            好物值得
            <br />
            被看见。
          </h1>
          <p>
            从商品的真实细节，
            <br />
            到让人愿意收藏的图文。
          </p>
          <span>文案 · 配图 · 排版</span>
        </div>
      </div>
      <form
        className="login-form"
        onSubmit={async (e) => {
          e.preventDefault();
          setBusy(true);
          setError("");
          try {
            onLogin(
              await api("/auth/" + (register ? "register" : "login"), "POST", {
                username,
                password,
                invite,
              }),
            );
          } catch (e) {
            setError((e as Error).message);
          } finally {
            setBusy(false);
          }
        }}
      >
        <p className="eyebrow">你的下一篇好内容，从这里开始</p>
        <h1>{register ? "加入创作空间" : "欢迎回来"}</h1>
        <p className="muted">
          {register
            ? "使用管理员提供的邀请码创建账号"
            : "登录 Owlet，继续你的图文创作"}
        </p>
        <Field label="用户名" value={username} onChange={setUsername} />
        <Field
          label="密码（12–72 字节）"
          value={password}
          onChange={setPassword}
          type="password"
        />
        {register && (
          <Field label="一次性邀请码" value={invite} onChange={setInvite} />
        )}
        <button className="primary wide" disabled={busy}>
          {busy ? "正在处理…" : register ? "注册并登录" : "登录"}
        </button>
        {error && (
          <p className="danger" role="alert">
            {error}
          </p>
        )}
        <button
          type="button"
          className="text-button"
          onClick={() => setRegister(!register)}
        >
          {register ? "已有账号？去登录" : "有邀请码？创建账号"}
        </button>
        <small className="muted">忘记密码？请联系管理员重置。</small>
      </form>
    </div>
  );
}
function Catalog({
  kind,
  docs,
  save,
  upload,
  notify,
  create,
}: {
  kind: "brand" | "product";
  docs: Doc[];
  save: (d: Doc) => Promise<Doc>;
  upload: (f: File) => Promise<string>;
  notify: (s: string) => void;
  create: (d: Doc) => void;
}) {
  const existing = docs.filter((d) => d.kind === kind);
  const [edit, setEdit] = useState<Doc>(
      existing[0] || {
        id: "",
        kind,
        body: emptyBody(),
        revision: 0,
        updated_at: "",
      },
    ),
    [busy, setBusy] = useState(false);
  function set(k: keyof Body, v: any) {
    setEdit({ ...edit, body: { ...edit.body, [k]: v } });
  }
  return (
    <section className="content">
      <div className="section-title">
        <h1>{kind === "brand" ? "品牌风格" : "商品资料"}</h1>
        {kind === "product" && (
          <button
            onClick={() =>
              setEdit({
                id: "",
                kind,
                body: emptyBody(),
                revision: 0,
                updated_at: "",
              })
            }
          >
            <Plus size={17} />
            添加商品
          </button>
        )}
      </div>
      <div className="catalog-layout">
        <div className="catalog-list">
          {existing.map((d) => (
            <button
              key={d.id}
              className={
                d.id === edit.id ? "catalog-item selected" : "catalog-item"
              }
              onClick={() => setEdit(d)}
            >
              {d.body.assetId ? (
                <img src={assetURL(d.body.assetId)} />
              ) : (
                <Package size={26} />
              )}
              <span>{d.body.name || "品牌资料"}</span>
            </button>
          ))}
        </div>
        <div className="panel form-panel">
          <Field
            label={kind === "brand" ? "品牌名称" : "商品名称 *"}
            value={edit.body.name}
            onChange={(v) => set("name", v)}
          />
          <label className="upload-zone">
            {edit.body.assetId ? (
              <img src={assetURL(edit.body.assetId)} />
            ) : (
              <>
                <ImagePlus size={30} />
                <span>
                  {kind === "brand" ? "上传品牌 Logo" : "上传商品原图 *"}
                </span>
              </>
            )}
            <input
              type="file"
              accept="image/png,image/jpeg,image/webp"
              onChange={async (e) => {
                const file = e.target.files?.[0];
                if (file)
                  try {
                    const aid = await upload(file);
                    setEdit({
                      ...edit,
                      body: {
                        ...edit.body,
                        assetId: aid,
                        ...(kind === "brand" ? { logoId: aid } : {}),
                      },
                    });
                  } catch (e) {
                    notify((e as Error).message);
                  }
              }}
            />
          </label>
          {kind === "brand" ? (
            <>
              <Field
                label="品牌主色"
                type="color"
                value={edit.body.color}
                onChange={(v) => set("color", v)}
              />
              <Field
                label="文案语气"
                value={edit.body.tone}
                onChange={(v) => set("tone", v)}
              />
            </>
          ) : (
            <>
              <Field
                label="核心卖点 *"
                multiline
                value={edit.body.selling}
                onChange={(v) => set("selling", v)}
              />
              <Field
                label="目标受众"
                value={edit.body.audience}
                onChange={(v) => set("audience", v)}
              />
              <Field
                label="活动价格（选填）"
                value={edit.body.price}
                onChange={(v) => set("price", v)}
              />
              <Field
                label="补充说明"
                multiline
                value={edit.body.note}
                onChange={(v) => set("note", v)}
              />
            </>
          )}
          <div className="actions">
            <button
              className="primary"
              disabled={busy}
              onClick={async () => {
                if (
                  !edit.body.name ||
                  (kind === "product" &&
                    (!edit.body.assetId || !edit.body.selling))
                ) {
                  notify("请填写名称、商品图片和核心卖点");
                  return;
                }
                setBusy(true);
                try {
                  setEdit(await save(edit));
                  notify("资料已保存");
                } catch (e) {
                  notify((e as Error).message);
                } finally {
                  setBusy(false);
                }
              }}
            >
              保存资料
            </button>
            {kind === "product" && edit.id && (
              <button onClick={() => create(edit)}>
                用这个商品创作
                <ChevronRight size={16} />
              </button>
            )}
          </div>
          <p className="muted">
            公共资料修改不会改变旧作品。每次新建创作都会保存独立快照。
          </p>
        </div>
      </div>
    </section>
  );
}
function Editor({
  initial,
  sources,
  save,
  upload,
  jobs,
  notify,
  back,
}: {
  initial: Doc;
  sources: Doc[];
  save: (d: Doc) => Promise<Doc>;
  upload: (f: File) => Promise<string>;
  jobs: Job[];
  notify: (s: string) => void;
  back: () => void;
}) {
  const [doc, setDoc] = useState(initial),
    [index, setIndex] = useState(0),
    [status, setStatus] = useState("已保存"),
    [busy, setBusy] = useState(false),
    [prompt, setPrompt] = useState(""),
    [live, setLive] = useState(""),
    [versions, setVersions] = useState<any[] | null>(null);
  const dirty = useRef(false),
    saving = useRef(false),
    latest = useRef(doc);
  latest.current = doc;
  function update(body: Body) {
    dirty.current = true;
    setStatus("待保存");
    setDoc((d) => ({ ...d, body }));
  }
  async function persist() {
    if (saving.current) throw new Error("正在自动保存，请稍后重试");
    saving.current = true;
    const snap = latest.current;
    try {
      const s = await save(snap);
      setDoc((current) => {
        if (current === snap) {
          dirty.current = false;
          setStatus("已保存");
          return s;
        }
        return { ...current, revision: s.revision };
      });
      return s;
    } finally {
      saving.current = false;
    }
  }
  useEffect(() => {
    const t = setInterval(() => {
      if (dirty.current && !saving.current)
        persist().catch((e) => {
          setStatus("保存失败");
          notify(e.message);
        });
    }, 2000);
    return () => clearInterval(t);
  }, []);
  useEffect(() => {
    const warn = (e: BeforeUnloadEvent) => {
      if (dirty.current) {
        e.preventDefault();
      }
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, []);
  const running = jobs.find(
    (j) => j.state === "running" || j.state === "queued",
  );
  useEffect(() => {
    if (!running) return;
    setLive("");
    const es = new EventSource("/api/jobs/" + running.id + "/events");
    es.onmessage = (e) => {
      const data = JSON.parse(e.data);
      if (data.delta) setLive((s) => (s + data.delta).slice(-3000));
    };
    es.addEventListener("done", () => es.close());
    return () => es.close();
  }, [running?.id]);
  const b = doc.body,
    page = b.pages[index] || pagesFor("", b.name)[0];
  const set = (k: keyof Body, v: any) => update({ ...b, [k]: v });
  const setPage = (k: keyof Page, v: any) =>
    update({
      ...b,
      confirmed: k === "title" || k === "text" ? false : b.confirmed,
      pages: b.pages.map((p, i) => (i === index ? { ...p, [k]: v } : p)),
    });
  async function generate(kind: string, target: number) {
    setBusy(true);
    try {
      const s = await persist();
      if (!s.body.name || !s.body.assetId || !s.body.selling)
        throw new Error("请先填写商品名称、图片和核心卖点");
      if (kind === "image" && !s.body.confirmed)
        throw new Error("请先确认文案与分页");
      const q = await api("/quotes", "POST", { kind });
      if (
        !confirm(
          `${q.mode === "mock" ? "模拟任务" : "模型调用"}将预留 ${money(q.reserve)}，调用 1 次。${target < 0 && kind === "image" ? "结果可应用到全部页面。" : ""} 是否继续？`,
        )
      )
        return;
      await api(
        "/jobs",
        "POST",
        {
          project_id: doc.id,
          kind,
          prompt:
            (kind === "text" && target >= 0
              ? `重点重写第 ${target + 1} 页的标题和文字，保留其他页原有内容。`
              : "") +
            (prompt ||
              `${b.brand?.tone || "自然简约"}，${b.name}，${b.selling}，主色${b.color}`),
          page: target,
        },
        { "Idempotency-Key": crypto.randomUUID() },
      );
      notify("已加入生成队列，离开页面也会继续");
    } catch (e) {
      notify((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  function apply(j: Job) {
    if (j.kind === "text" && j.result.draft) {
      const d = j.result.draft;
      update({
        ...b,
        title: j.input.page < 0 ? d.title : b.title,
        caption: j.input.page < 0 ? d.caption : b.caption,
        tags: j.input.page < 0 ? d.tags : b.tags,
        confirmed: false,
        pages: b.pages.map((p, i) =>
          j.input.page < 0 || j.input.page === i ? { ...p, ...d.pages[i] } : p,
        ),
        appliedJobs: [...(b.appliedJobs || []), j.id],
      });
    } else if (j.result.assetId) {
      update({
        ...b,
        pages: b.pages.map((p, i) =>
          j.input.page < 0 || j.input.page === i
            ? { ...p, backgroundId: j.result.assetId! }
            : p,
        ),
        appliedJobs: [...(b.appliedJobs || []), j.id],
      });
    }
    notify("已应用到当前草稿，可继续编辑");
  }
  async function exportImages(all: boolean) {
    setBusy(true);
    try {
      await persist();
      if (all) {
        const z = new JSZip();
        for (let i = 0; i < b.pages.length; i++)
          z.file(`${i + 1}.png`, await png(b, b.pages[i], i));
        z.file("文案.txt", b.title + "\n\n" + b.caption + "\n\n" + b.tags);
        download(
          await z.generateAsync({ type: "blob" }),
          "owlet-" + doc.id.slice(0, 8) + ".zip",
        );
      } else download(await png(b, page, index), `owlet-${index + 1}.png`);
    } catch (e) {
      notify((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="editor">
      <div className="editor-top">
        <button
          onClick={async () => {
            try {
              if (dirty.current) await persist();
              back();
            } catch (e) {
              notify((e as Error).message);
            }
          }}
        >
          <ArrowLeft size={17} />
          返回
        </button>
        <input
          aria-label="作品名称"
          className="project-name"
          value={b.name}
          onChange={(e) => set("name", e.target.value)}
        />
        <span className="saved">
          <Check size={14} />
          {status}
        </span>
        <button
          onClick={async () => {
            try {
              setVersions(await api("/documents/" + doc.id + "/versions"));
            } catch (e) {
              notify((e as Error).message);
            }
          }}
        >
          <History size={16} />
          版本
        </button>
        <button
          className="primary"
          disabled={busy}
          onClick={() => exportImages(true)}
        >
          <Download size={16} />
          导出图文
        </button>
      </div>
      <div className="editor-layout">
        <div className="edit-controls desktop-edit">
          <h3>创作资料</h3>
          <button
            className="wide"
            onClick={() => {
              const brand = sources.find((d) => d.kind === "brand")?.body;
              const product = sources.find(
                (d) => d.id === b.productSourceId,
              )?.body;
              if (!brand && !product) {
                notify("没有可应用的公共资料");
                return;
              }
              if (
                confirm(
                  "应用最新品牌与商品资料？已有文案会保留，请重新确认分页。",
                )
              )
                update({
                  ...b,
                  ...(product
                    ? {
                        name: product.name,
                        selling: product.selling,
                        audience: product.audience,
                        price: product.price,
                        note: product.note,
                        assetId: product.assetId,
                        product,
                        pages: b.pages.map((p) => ({
                          ...p,
                          assetId: product.assetId,
                        })),
                      }
                    : {}),
                  ...(brand ? { brand, color: brand.color } : {}),
                  confirmed: false,
                });
            }}
          >
            应用最新资料
          </button>
          <Field
            label="商品名称"
            value={b.name}
            onChange={(v) => set("name", v)}
          />
          <label className="upload-small">
            <ImagePlus size={16} />
            上传 / 替换商品图片
            <input
              type="file"
              accept="image/png,image/jpeg,image/webp"
              onChange={async (e) => {
                const f = e.target.files?.[0];
                if (f)
                  try {
                    const aid = await upload(f);
                    update({
                      ...b,
                      assetId: aid,
                      pages: b.pages.map((p) => ({ ...p, assetId: aid })),
                    });
                  } catch (e) {
                    notify((e as Error).message);
                  }
              }}
            />
          </label>
          <Field
            label="核心卖点"
            multiline
            value={b.selling}
            onChange={(v) => set("selling", v)}
          />
          <Field
            label="本次生成要求（可选）"
            multiline
            value={prompt}
            onChange={setPrompt}
          />
          <button
            className="primary wide"
            disabled={busy}
            onClick={() => generate("text", -1)}
          >
            <Sparkles size={16} />
            生成 / 重写文案
          </button>
          <div className="divider" />
          <h3>整篇文案</h3>
          <Field
            label="标题"
            value={b.title}
            onChange={(v) => update({ ...b, title: v, confirmed: false })}
          />
          <Field
            label="正文"
            multiline
            value={b.caption}
            onChange={(v) => update({ ...b, caption: v, confirmed: false })}
          />
          <Field label="标签" value={b.tags} onChange={(v) => set("tags", v)} />
          <label className="check">
            <input
              type="checkbox"
              checked={!!b.confirmed}
              onChange={(e) => set("confirmed", e.target.checked)}
            />
            我已确认文案与分页
          </label>
          <button
            className="wide"
            disabled={busy || !b.confirmed}
            onClick={() => generate("image", -1)}
          >
            <Sparkles size={16} />
            生成一张共享背景
          </button>
        </div>
        <div className="canvas-area">
          <div className="canvas-heading">
            <span>{index === 0 ? "封面" : "内容页 " + index}</span>
            <span>3:4 · 商品原图保留</span>
          </div>
          <Preview body={b} page={page} index={index} />
          <div className="page-strip">
            {b.pages.map((p, i) => (
              <button
                key={i}
                className={i === index ? "selected" : ""}
                onClick={() => setIndex(i)}
              >
                <Preview body={b} page={p} index={i} />
                <span>{i === 0 ? "封面" : `第 ${i + 1} 页`}</span>
              </button>
            ))}
          </div>
          <div className="actions">
            <button disabled={busy} onClick={() => exportImages(false)}>
              <Download size={15} />
              下载这一页
            </button>
            <button
              onClick={() =>
                navigator.clipboard
                  .writeText(b.title + "\n\n" + b.caption + "\n\n" + b.tags)
                  .then(() => notify("文案已复制"))
                  .catch(() => notify("复制失败，请手动选择文案"))
              }
            >
              <Copy size={15} />
              复制文案
            </button>
          </div>
          <p className="mobile-hint">
            排版编辑请使用电脑，手机可查看、复制和下载。
          </p>
        </div>
        <div className="page-controls desktop-edit">
          <h3>页面样式</h3>
          <label className="field">
            <span>模板</span>
            <select
              value={b.template}
              onChange={(e) => set("template", e.target.value)}
            >
              <option value="simple">简约商品</option>
              <option value="promo">促销活动</option>
              <option value="life">生活方式</option>
            </select>
          </label>
          <div className="two">
            <Field
              label="品牌主色"
              type="color"
              value={b.color}
              onChange={(v) => set("color", v)}
            />
            <Field
              label="背景颜色"
              type="color"
              value={b.background}
              onChange={(v) => set("background", v)}
            />
          </div>
          <Field
            label="本页标题"
            value={page.title}
            onChange={(v) => setPage("title", v)}
          />
          <Field
            label="本页文字"
            multiline
            value={page.text}
            onChange={(v) => setPage("text", v)}
          />
          <button
            className="wide"
            disabled={busy || !!running}
            onClick={() => generate("text", index)}
          >
            <Sparkles size={16} />
            重写本页文案
          </button>
          <label className="field">
            <span>预设背景</span>
            <select
              value=""
              onChange={(e) =>
                update({
                  ...b,
                  background: e.target.value,
                  pages: b.pages.map((p, i) =>
                    i === index ? { ...p, backgroundId: "" } : p,
                  ),
                })
              }
            >
              <option value="" disabled>
                选择背景配色
              </option>
              <option value="#f4eee5">奶油米色</option>
              <option value="#e8eee6">鼠尾草绿</option>
              <option value="#fce9ed">柔雾粉色</option>
              <option value="#e9eef7">晴空蓝色</option>
              <option value="#ffffff">纯净白色</option>
            </select>
          </label>
          {[
            ["x", "水平位置", 0, 100],
            ["y", "垂直位置", 0, 100],
            ["scale", "图片大小", 20, 90],
            ["cropX", "裁剪水平位置", 0, 100],
            ["cropY", "裁剪垂直位置", 0, 100],
          ].map(([k, label, min, max]) => (
            <label className="slider" key={k}>
              <span>
                {label}
                <small>{page[k as keyof Page]}</small>
              </span>
              <input
                type="range"
                min={min}
                max={max}
                value={page[k as keyof Page]}
                onChange={(e) =>
                  setPage(k as keyof Page, Number(e.target.value))
                }
              />
            </label>
          ))}
          <button
            className="wide"
            disabled={busy || !b.confirmed}
            onClick={() => generate("image", index)}
          >
            <Sparkles size={16} />
            重画本页背景
          </button>
          {page.backgroundId && (
            <button
              className="text-button"
              onClick={() => setPage("backgroundId", "")}
            >
              恢复预设背景
            </button>
          )}
        </div>
      </div>
      {live && (
        <details className="stream">
          <summary>文案生成进度</summary>
          <pre>{live}</pre>
        </details>
      )}
      <div className="editor-jobs">
        <h3>本篇生成记录</h3>
        <JobList
          jobs={jobs}
          notify={notify}
          apply={apply}
          applied={b.appliedJobs || []}
        />
      </div>
      {versions && (
        <div className="modal-backdrop">
          <div className="modal">
            <h2>历史版本</h2>
            <p className="muted">选择后作为当前草稿保存，不覆盖历史。</p>
            {versions.map((v) => (
              <div className="version-row" key={v.revision}>
                <span>
                  版本 {v.revision} · {new Date(v.created_at).toLocaleString()}
                </span>
                <button
                  onClick={() => {
                    update(v.body);
                    setVersions(null);
                  }}
                >
                  使用此版本
                </button>
              </div>
            ))}
            <button onClick={() => setVersions(null)}>关闭</button>
          </div>
        </div>
      )}
    </section>
  );
}
function JobList({
  jobs,
  notify,
  apply,
  applied = [],
}: {
  jobs: Job[];
  notify: (s: string) => void;
  apply?: (j: Job) => void;
  applied?: string[];
}) {
  return (
    <div className="job-list">
      {!jobs.length && (
        <p className="empty">还没有生成记录。你可以先使用模板手动创作。</p>
      )}
      {jobs.map((j) => (
        <div className="job-row" key={j.id}>
          <div className={"job-icon " + j.state}>
            {j.kind === "text" ? (
              <Sparkles size={20} />
            ) : (
              <ImagePlus size={20} />
            )}
          </div>
          <div className="job-info">
            <strong>
              {j.kind === "text" ? "图文文案" : "背景图片"}{" "}
              <span className={"badge " + j.state}>{stateName[j.state]}</span>
            </strong>
            <small>
              {new Date(j.created_at).toLocaleString()} ·{" "}
              {j.result.mode === "mock" ? "模拟额度" : "费用"}{" "}
              {money(j.charged)} / 预留 {money(j.reserve)}
            </small>
            {j.error && <p>{j.error}</p>}
          </div>
          {j.state === "succeeded" && apply && (
            <button onClick={() => apply(j)}>
              {applied.includes(j.id) ? "再次应用" : "应用到草稿"}
            </button>
          )}
          {["running", "queued"].includes(j.state) && (
            <button
              onClick={() =>
                api("/jobs/" + j.id + "/cancel", "POST", {})
                  .then(() => notify("已请求取消，已发出的调用仍可能计费"))
                  .catch((e) => notify(e.message))
              }
            >
              取消
            </button>
          )}
          {j.state === "failed" && apply && (
            <button
              onClick={async () => {
                try {
                  if (!confirm("重新提交会重新预留额度。继续？")) return;
                  const input = await api<Job[]>("/jobs");
                  const old = input.find((x) => x.id === j.id);
                  if (old)
                    await api(
                      "/jobs",
                      "POST",
                      { ...old.input, project_id: j.project_id, kind: j.kind },
                      { "Idempotency-Key": crypto.randomUUID() },
                    );
                  notify("已重新排队");
                } catch (e) {
                  notify((e as Error).message);
                }
              }}
            >
              重试
            </button>
          )}
        </div>
      ))}
    </div>
  );
}
function Admin({ notify }: { notify: (s: string) => void }) {
  const [data, setData] = useState<any>(null),
    [jobs, setJobs] = useState<Job[]>([]),
    [invite, setInvite] = useState("");
  async function refresh() {
    setData(await api("/admin"));
    setJobs(await api("/jobs?all=true"));
  }
  useEffect(() => {
    refresh().catch((e) => notify(e.message));
  }, []);
  async function act(action: string, body: any) {
    try {
      const r = await api("/admin/" + action, "POST", body);
      if (r.invite) setInvite(r.invite);
      await refresh();
    } catch (e) {
      notify((e as Error).message);
    }
  }
  return (
    <section className="content">
      <div className="section-title">
        <h1>管理后台</h1>
        <div className="actions">
          <button onClick={() => act("pause", { paused: !data?.paused })}>
            {data?.paused ? "恢复生成" : "暂停全站生成"}
          </button>
          <button className="primary" onClick={() => act("invite", {})}>
            <Plus size={16} />
            生成邀请码
          </button>
        </div>
      </div>
      {invite && (
        <div className="panel">
          <p>邀请码仅展示本次，请复制后发送给受邀用户：</p>
          <code>{invite}</code>
          <button onClick={() => navigator.clipboard.writeText(invite)}>
            复制
          </button>
        </div>
      )}
      <h3>用户</h3>
      <div className="panel">
        {data?.users?.map((u: any) => (
          <div className="version-row" key={u.id}>
            <span>
              {u.username} ·{" "}
              {u.admin ? "管理员" : u.disabled ? "已禁用" : "正常"}
            </span>
            <div className="actions">
              <button
                onClick={() => act("user", { id: u.id, disabled: !u.disabled })}
              >
                {u.disabled ? "启用" : "禁用"}
              </button>
              <button
                onClick={() => {
                  const password = prompt("输入 12–72 字节的新密码");
                  if (password) act("password", { id: u.id, password });
                }}
              >
                重置密码
              </button>
            </div>
          </div>
        ))}
      </div>
      <h3>失败与待核对任务</h3>
      <JobList
        jobs={jobs.filter((j) =>
          ["failed", "outcome_unknown"].includes(j.state),
        )}
        notify={notify}
      />
      {jobs
        .filter((j) => j.state === "outcome_unknown")
        .map((j) => (
          <div className="panel" key={j.id}>
            <code>{j.id}</code>
            <button
              onClick={() => {
                const amount = prompt(
                  "核对上游账单后输入实际费用（元）；此操作会结算预留额度",
                );
                if (
                  amount !== null &&
                  amount.trim() !== "" &&
                  Number.isFinite(Number(amount))
                )
                  act("reconcile", {
                    id: j.id,
                    amount: Math.round(Number(amount) * 1e6),
                  });
              }}
            >
              按上游账单结算
            </button>
          </div>
        ))}
      <h3>费用流水</h3>
      <div className="panel">
        {data?.ledger?.map((l: any) => (
          <div className="version-row" key={l.job_id}>
            <span>
              {l.username} · {l.note}
            </span>
            <strong>{money(l.amount)}</strong>
          </div>
        ))}
        {!data?.ledger?.length && <p className="muted">暂无流水</p>}
      </div>
    </section>
  );
}
