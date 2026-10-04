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
  idleTimeoutMs: number;
};
type Display = { index: number; x: number; y: number; w: number; h: number; primary: boolean };
type Diagnostics = {
  capturePath: string;
  activeTitle: string;
  displays: Display[] | null;
  configPath: string;
  platform: string;
  version: string;
  error?: string;
};

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

const roiText = (r: Rect) => `${r.x},${r.y} ${r.w}×${r.h}`;

export default function App() {
  const [diag, setDiag] = useState<Diagnostics | null>(null);
  const [spots, setSpots] = useState<Spot[]>([]);
  const [cfg, setCfg] = useState<Config | null>(null);
  const [status, setStatus] = useState<Status>(emptyStatus);
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

  const saveCfg = async (next: Config) => {
    setCfg(next);
    await call("SetConfig", next);
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

  const scoreFor = (id: string): Metrics | undefined => status.spots?.find((m) => m.spot.id === id);
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
              {" · "}
              <code>v{diag.version}</code>
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
          <h2>Prompt spots (fixed)</h2>
          <p className="hint">
            Coordinates are fixed; the app does not calibrate. The game window must be at the same
            position used when the coordinates were recorded.
          </p>
          <table className="spots">
            <thead>
              <tr>
                <th>key</th>
                <th>roi</th>
                <th>white</th>
                <th>blue</th>
              </tr>
            </thead>
            <tbody>
              {spots.map((s) => {
                const m = scoreFor(s.id);
                const best = status.best >= 0 && status.spots?.[status.best]?.spot.id === s.id;
                return (
                  <tr key={s.id} className={best ? "best" : ""}>
                    <td className="key">{s.key}</td>
                    <td className="mono">{roiText(s.roi)}</td>
                    <td className="num">{m ? m.white.toFixed(2) : "—"}</td>
                    <td className="num">{m ? m.blue.toFixed(2) : "—"}</td>
                  </tr>
                );
              })}
            </tbody>
          </table>
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
              <label>
                stop if idle (s)
                <input
                  type="number"
                  value={cfg.idleTimeoutMs / 1000}
                  onChange={(e) => saveCfg({ ...cfg, idleTimeoutMs: Number(e.target.value) * 1000 })}
                />
              </label>
            </div>
          )}

          {diag?.platform === "linux" && (
            <>
              <h2>Setup</h2>
              <button onClick={setupKwin} disabled={busy}>
                Authorize fast KWin capture (KDE Wayland)
              </button>
              <p className="hint">
                Fast region capture on KDE requires this once per binary path. Without it the app
                falls back to the slow portal.
              </p>
            </>
          )}
        </section>
      </div>
    </div>
  );
}
