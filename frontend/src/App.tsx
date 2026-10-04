import { useCallback, useEffect, useRef, useState } from "react";
import { Call, Events } from "@wailsio/runtime";

type Rect = { x: number; y: number; w: number; h: number };
type Spot = { id: string; key: string; roi: Rect };
type Metrics = { spot: Spot; white: number; blue: number; score: number };
type Status = {
  running: boolean;
  spots: Metrics[] | null;
  best: number;
  ambiguous: boolean;
  presses: number;
  lastKey: string;
  elapsedMs: number;
  error?: string;
};
type Config = {
  pollMs: number;
  whiteThreshold: number;
  clearThreshold: number;
  margin: number;
  refractoryMs: number;
  retryMs: number;
};
type Display = { index: number; x: number; y: number; w: number; h: number; primary: boolean };
type Diagnostics = {
  capturePath: string;
  activeTitle: string;
  displays: Display[] | null;
  configPath: string;
  error?: string;
};
type Preview = { png: string; w: number; h: number };

const emptyStatus: Status = {
  running: false,
  spots: null,
  best: -1,
  ambiguous: false,
  presses: 0,
  lastKey: "",
  elapsedMs: 0,
};

const call = <T,>(method: string, ...args: unknown[]) =>
  Call.ByName(`main.Service.${method}`, ...args) as Promise<T>;

export default function App() {
  const [diag, setDiag] = useState<Diagnostics | null>(null);
  const [spots, setSpots] = useState<Spot[]>([]);
  const [cfg, setCfg] = useState<Config | null>(null);
  const [status, setStatus] = useState<Status>(emptyStatus);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [busy, setBusy] = useState(false);
  const [note, setNote] = useState("");
  const offRef = useRef<(() => void) | null>(null);

  const reload = useCallback(async () => {
    const [d, s, c] = await Promise.all([
      call<Diagnostics>("Diagnostics"),
      call<Spot[]>("GetSpots"),
      call<Config>("GetConfig"),
    ]);
    setDiag(d);
    setSpots(s);
    setCfg(c);
  }, []);

  useEffect(() => {
    reload().catch((e) => setNote(String(e)));
    offRef.current = Events.On("status", (ev: any) => setStatus(ev.data as Status));
    return () => offRef.current?.();
  }, [reload]);

  const saveSpots = async (next: Spot[]) => {
    setSpots(next);
    await call("SetSpots", next);
  };

  const saveCfg = async (next: Config) => {
    setCfg(next);
    await call("SetConfig", next);
  };

  const patchSpot = (i: number, patch: Partial<Spot>) => {
    const next = spots.map((s, j) => (j === i ? { ...s, ...patch } : s));
    saveSpots(next).catch((e) => setNote(String(e)));
  };
  const patchRoi = (i: number, patch: Partial<Rect>) => {
    const next = spots.map((s, j) => (j === i ? { ...s, roi: { ...s.roi, ...patch } } : s));
    saveSpots(next).catch((e) => setNote(String(e)));
  };

  const start = async () => {
    setBusy(true);
    try {
      await call("Start");
      setNote("");
    } catch (e) {
      setNote(String(e));
    } finally {
      setBusy(false);
    }
  };
  const stop = () => call("Stop");

  const setupKwin = async () => {
    setBusy(true);
    try {
      const path = await call<string>("SetupKWin");
      setNote(`Authorized KWin capture via ${path}`);
      await reload();
    } catch (e) {
      setNote(String(e));
    } finally {
      setBusy(false);
    }
  };

  const capture = async () => {
    setBusy(true);
    try {
      setPreview(await call<Preview>("CapturePreview"));
    } catch (e) {
      setNote(String(e));
    } finally {
      setBusy(false);
    }
  };

  const scoreFor = (id: string): Metrics | undefined =>
    status.spots?.find((m) => m.spot.id === id);

  const backendDown = !!diag?.error;

  return (
    <div className="app">
      <header>
        <h1>WAMI Auto Fisher</h1>
        <div className="sub">
          {diag ? (
            <>
              capture: <code>{diag.capturePath || "—"}</code>
              {diag.activeTitle && <> · focused: <code>{diag.activeTitle}</code></>}
            </>
          ) : (
            "loading…"
          )}
        </div>
      </header>

      {backendDown && <div className="banner warn">Capture backend unavailable: {diag?.error}</div>}
      {note && <div className="banner">{note}</div>}

      <div className="grid">
        <section className="panel">
          <div className="row between">
            <h2>Prompt spots</h2>
            <button onClick={capture} disabled={busy || backendDown}>
              Capture screenshot
            </button>
          </div>

          <table className="spots">
            <thead>
              <tr>
                <th>key</th>
                <th>x</th>
                <th>y</th>
                <th>w</th>
                <th>h</th>
                <th>white</th>
                <th>blue</th>
              </tr>
            </thead>
            <tbody>
              {spots.map((s, i) => {
                const m = scoreFor(s.id);
                const best = status.best >= 0 && status.spots?.[status.best]?.spot.id === s.id;
                return (
                  <tr key={s.id} className={best ? "best" : ""}>
                    <td>
                      <select value={s.key} onChange={(e) => patchSpot(i, { key: e.target.value })}>
                        {["a", "w", "s", "d"].map((k) => (
                          <option key={k} value={k}>
                            {k}
                          </option>
                        ))}
                      </select>
                    </td>
                    {(["x", "y", "w", "h"] as const).map((f) => (
                      <td key={f}>
                        <input
                          type="number"
                          value={s.roi[f]}
                          onChange={(e) => patchRoi(i, { [f]: Number(e.target.value) } as Partial<Rect>)}
                        />
                      </td>
                    ))}
                    <td className="num">{m ? m.white.toFixed(2) : "—"}</td>
                    <td className="num">{m ? m.blue.toFixed(2) : "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>

          <details>
            <summary>Calibration preview</summary>
            {preview ? (
              <div className="preview">
                <img src={`data:image/png;base64,${preview.png}`} alt="target monitor" />
                {spots.map((s) => (
                  <div
                    key={s.id}
                    className="roi"
                    style={{
                      left: `${(s.roi.x / preview.w) * 100}%`,
                      top: `${(s.roi.y / preview.h) * 100}%`,
                      width: `${(s.roi.w / preview.w) * 100}%`,
                      height: `${(s.roi.h / preview.h) * 100}%`,
                    }}
                  >
                    <span>{s.key}</span>
                  </div>
                ))}
              </div>
            ) : (
              <p className="hint">
                Capture a screenshot, then adjust x/y/w/h so each box frames a prompt.
              </p>
            )}
          </details>
        </section>

        <section className="panel">
          <h2>Run</h2>
          <div className="row">
            <button className="primary" onClick={start} disabled={busy || status.running || backendDown}>
              ▶ Start
            </button>
            <button className="danger" onClick={stop} disabled={!status.running}>
              ⏹ Stop
            </button>
          </div>
          <div className="telemetry">
            <div>
              <span className="k">state</span>
              <span className={status.running ? "ok" : ""}>{status.running ? "running" : "idle"}</span>
            </div>
            <div>
              <span className="k">presses</span>
              <span>{status.presses}</span>
            </div>
            <div>
              <span className="k">last key</span>
              <span>{status.lastKey || "—"}</span>
            </div>
            <div>
              <span className="k">elapsed</span>
              <span>{(status.elapsedMs / 1000).toFixed(1)}s</span>
            </div>
          </div>
          {status.error && <div className="banner warn">{status.error}</div>}

          <h2>Detection</h2>
          {cfg && (
            <div className="fields">
              <label>
                poll (ms)
                <input
                  type="number"
                  value={cfg.pollMs}
                  onChange={(e) => saveCfg({ ...cfg, pollMs: Number(e.target.value) })}
                />
              </label>
              <label>
                white threshold
                <input
                  type="number"
                  step="0.05"
                  value={cfg.whiteThreshold}
                  onChange={(e) => saveCfg({ ...cfg, whiteThreshold: Number(e.target.value) })}
                />
              </label>
              <label>
                clear threshold
                <input
                  type="number"
                  step="0.05"
                  value={cfg.clearThreshold}
                  onChange={(e) => saveCfg({ ...cfg, clearThreshold: Number(e.target.value) })}
                />
              </label>
              <label>
                margin
                <input
                  type="number"
                  step="0.05"
                  value={cfg.margin}
                  onChange={(e) => saveCfg({ ...cfg, margin: Number(e.target.value) })}
                />
              </label>
            </div>
          )}

          <h2>Setup</h2>
          <button onClick={setupKwin} disabled={busy}>
            Authorize fast KWin capture (KDE Wayland)
          </button>
          <p className="hint">
            Fast region capture on KDE requires this once per binary path. Without it the app
            falls back to the slow portal.
          </p>
        </section>
      </div>
    </div>
  );
}
