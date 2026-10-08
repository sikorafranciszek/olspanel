import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RefreshCw } from "lucide-react";
import { get, post, errorMessage } from "../../lib/api";
import type { ServerInfo } from "../../lib/types";
import { Badge, Button, Card, PageHeader, Progress, Spinner, Stat, ErrorBox } from "../../components/ui";
import { useToast } from "../../components/toast";

function uptime(s: number) {
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  return d > 0 ? `${d} d ${h} h` : `${h} h ${m} min`;
}

export default function AdminDashboard() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["admin", "server"], queryFn: () => get<ServerInfo>("/admin/server"), refetchInterval: 15_000 });
  const restart = useMutation({
    mutationFn: (unit: string) => post(`/admin/services/${unit}/restart`),
    onSuccess: () => {
      toast.success("Usługa została zrestartowana");
      qc.invalidateQueries({ queryKey: ["admin", "server"] });
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const apply = useMutation({
    mutationFn: () => post("/admin/ols/apply"),
    onSuccess: () => toast.success("Konfiguracja OpenLiteSpeed została odświeżona"),
    onError: (e) => toast.error(errorMessage(e)),
  });

  if (q.isLoading) return <Spinner />;
  if (q.error || !q.data) return <ErrorBox>{errorMessage(q.error)}</ErrorBox>;
  const s = q.data;
  const r = s.resources;

  return (
    <>
      <PageHeader title="Pulpit" subtitle={`${s.hostname} · olspanel ${s.version}`} actions={<Button variant="secondary" onClick={() => apply.mutate()} loading={apply.isPending}><RefreshCw className="h-4 w-4" /> Odśwież konfigurację OLS</Button>} />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat label="Użytkownicy" value={s.users} />
        <Stat label="Domeny" value={s.domains} />
        <Stat label="Pakiety" value={s.packages} />
        <Stat label="Czas działania panelu" value={uptime(s.uptime_s)} sub={s.os} />
      </div>
      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <Card title="Zasoby serwera">
          <div className="space-y-4 text-sm">
            <div>
              <div className="mb-1 flex justify-between"><span>Obciążenie (1 / 5 / 15 min)</span><span className="text-slate-500">{r.load_avg.map((x) => x.toFixed(2)).join(" / ")} · {r.cpus} CPU</span></div>
              <Progress value={r.load_avg[0]} max={Math.max(1, r.cpus)} />
            </div>
            <div>
              <div className="mb-1 flex justify-between"><span>Pamięć RAM</span><span className="text-slate-500">{r.mem_used_mb} / {r.mem_total_mb} MB</span></div>
              <Progress value={r.mem_used_mb} max={r.mem_total_mb} />
            </div>
            <div>
              <div className="mb-1 flex justify-between"><span>Dysk (/home)</span><span className="text-slate-500">{r.disk_used_gb.toFixed(1)} / {r.disk_total_gb.toFixed(1)} GB</span></div>
              <Progress value={r.disk_used_gb} max={r.disk_total_gb} />
            </div>
          </div>
        </Card>
        <Card title="Usługi">
          <ul className="divide-y divide-slate-100 text-sm">
            {(s.services ?? []).map((svc) => (
              <li key={svc.unit} className="flex items-center justify-between py-2">
                <span className="flex items-center gap-2">
                  {svc.label}
                  {svc.unit === "lsws" && !s.ols_ok && <Badge tone="red">nie odpowiada na :80</Badge>}
                </span>
                <span className="flex items-center gap-2">
                  <Badge tone={svc.active ? "green" : "red"}>{svc.active ? "działa" : "zatrzymana"}</Badge>
                  <Button size="sm" variant="secondary" onClick={() => restart.mutate(svc.unit)} loading={restart.isPending && restart.variables === svc.unit}>Restart</Button>
                </span>
              </li>
            ))}
          </ul>
          <div className="mt-4 text-xs text-slate-500">
            Zainstalowane PHP: {s.php?.length ? s.php.map((p) => p.label).join(", ") : "brak"}
          </div>
        </Card>
      </div>
    </>
  );
}
