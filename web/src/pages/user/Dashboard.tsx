import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { get, errorMessage } from "../../lib/api";
import type { Domain, Usage } from "../../lib/types";
import { useAuth } from "../../lib/auth";
import { Badge, Card, ErrorBox, PageHeader, Progress, Spinner, formatMB, phpLabel } from "../../components/ui";

function Limit({ label, used, max }: { label: string; used: number; max: number }) {
  return (
    <div>
      <div className="mb-1 flex justify-between text-sm"><span>{label}</span><span className="text-slate-500">{used} / {max}</span></div>
      <Progress value={used} max={max} />
    </div>
  );
}

export default function UserDashboard() {
  const { user } = useAuth();
  const usage = useQuery({ queryKey: ["usage"], queryFn: () => get<Usage>("/usage") });
  const domains = useQuery({ queryKey: ["domains"], queryFn: () => get<Domain[]>("/domains") });
  if (usage.isLoading) return <Spinner />;
  if (usage.error || !usage.data) return <ErrorBox>{errorMessage(usage.error)}</ErrorBox>;
  const u = usage.data;
  const p = u.package;
  return (
    <>
      <PageHeader title={`Witaj, ${user?.username}`} subtitle={p ? `Pakiet: ${p.name}` : "Brak pakietu"} />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="Wykorzystanie zasobów">
          {p ? (
            <div className="space-y-4">
              <div>
                <div className="mb-1 flex justify-between text-sm"><span>Miejsce na dysku</span><span className="text-slate-500">{formatMB(u.disk_used_mb)} / {formatMB(p.disk_mb)}</span></div>
                <Progress value={u.disk_used_mb} max={p.disk_mb} />
              </div>
              <Limit label="Domeny" used={u.domains} max={p.max_domains} />
              <Limit label="Subdomeny" used={u.subdomains} max={p.max_subdomains} />
              <Limit label="Bazy danych" used={u.databases} max={p.max_databases} />
              <Limit label="Konta FTP" used={u.ftp} max={p.max_ftp} />
              <Limit label="Zadania cron" used={u.cron} max={p.max_cron} />
              <p className="text-xs text-slate-500">Dostępne wersje PHP: {p.php_versions.map(phpLabel).join(", ")}</p>
            </div>
          ) : (
            <p className="text-sm text-slate-500">Administrator nie przypisał jeszcze pakietu do Twojego konta.</p>
          )}
        </Card>
        <Card title="Twoje domeny" actions={<Link to="/domains" className="text-sm text-blue-600 hover:underline">Zarządzaj</Link>}>
          {domains.data?.length ? (
            <ul className="divide-y divide-slate-100 text-sm">
              {domains.data.map((d) => (
                <li key={d.id} className="flex items-center justify-between py-2">
                  <a href={`http${d.ssl_status === "active" ? "s" : ""}://${d.name}`} target="_blank" rel="noreferrer" className="font-medium text-slate-900 hover:text-blue-700">{d.name}</a>
                  <span className="flex gap-2">
                    {d.type !== "alias" && <Badge>{phpLabel(d.php_version)}</Badge>}
                    {d.ssl_status === "active" && <Badge tone="green">SSL</Badge>}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-slate-500">Nie masz jeszcze żadnych domen. <Link to="/domains" className="text-blue-600 hover:underline">Dodaj pierwszą</Link>.</p>
          )}
        </Card>
      </div>
      <Card title="Szybki start" className="mt-6">
        <ol className="list-decimal space-y-1 pl-5 text-sm text-slate-700">
          <li>Dodaj domenę w zakładce <Link to="/domains" className="text-blue-600 hover:underline">Domeny</Link> i skieruj jej rekord A na adres IP serwera.</li>
          <li>Wgraj pliki strony do katalogu <code className="rounded bg-slate-100 px-1">domains/&lt;domena&gt;/public_html</code> przez <Link to="/files" className="text-blue-600 hover:underline">menedżer plików</Link> lub <Link to="/ftp" className="text-blue-600 hover:underline">FTP</Link>.</li>
          <li>Utwórz <Link to="/databases" className="text-blue-600 hover:underline">bazę danych</Link>, jeśli strona jej potrzebuje.</li>
          <li>Gdy DNS wskazuje na serwer, włącz darmowy certyfikat SSL przy domenie.</li>
        </ol>
      </Card>
    </>
  );
}
