import { useEffect, useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, put, errorMessage } from "../../lib/api";
import type { PHPVersion } from "../../lib/types";
import { useToast } from "../../components/toast";
import { Button, Card, Field, Input, PageHeader, Select, Spinner } from "../../components/ui";

type Settings = Record<string, string>;

export default function AdminSettings() {
  const qc = useQueryClient();
  const toast = useToast();
  const q = useQuery({ queryKey: ["admin", "settings"], queryFn: () => get<Settings>("/admin/settings") });
  const php = useQuery({ queryKey: ["php"], queryFn: () => get<PHPVersion[]>("/php/versions") });
  const [form, setForm] = useState<Settings>({});
  useEffect(() => {
    if (q.data) setForm({ panel_hostname: "", acme_email: "", acme_staging: "0", default_php: "", ftp_passive_ip: "", ...q.data });
  }, [q.data]);
  const save = useMutation({
    mutationFn: (s: Settings) => put<Settings>("/admin/settings", s),
    onSuccess: () => {
      toast.success("Ustawienia zapisane");
      qc.invalidateQueries({ queryKey: ["admin", "settings"] });
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(form);
  };
  if (q.isLoading) return <Spinner />;
  return (
    <>
      <PageHeader title="Ustawienia" subtitle="Konfiguracja panelu i certyfikatów" />
      <form onSubmit={submit} className="max-w-xl space-y-6">
        <Card title="Panel">
          <div className="space-y-4">
            <Field label="Nazwa hosta panelu" hint="np. panel.example.com – używana dla certyfikatu panelu i FTP">
              <Input value={form.panel_hostname ?? ""} onChange={(e) => setForm({ ...form, panel_hostname: e.target.value })} />
            </Field>
            <Field label="Domyślna wersja PHP dla nowych domen">
              <Select value={form.default_php ?? ""} onChange={(e) => setForm({ ...form, default_php: e.target.value })}>
                <option value="">– najnowsza z pakietu –</option>
                {php.data?.map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
              </Select>
            </Field>
            <Field label="Publiczny adres IP dla pasywnego FTP" hint="wymagane tylko gdy serwer jest za NAT">
              <Input value={form.ftp_passive_ip ?? ""} onChange={(e) => setForm({ ...form, ftp_passive_ip: e.target.value })} placeholder="np. 203.0.113.10" />
            </Field>
          </div>
        </Card>
        <Card title="Let's Encrypt">
          <div className="space-y-4">
            <Field label="Adres e-mail konta ACME" hint="wymagany do wystawiania certyfikatów; Let's Encrypt wysyła na niego ostrzeżenia o wygaśnięciu">
              <Input type="email" value={form.acme_email ?? ""} onChange={(e) => setForm({ ...form, acme_email: e.target.value })} />
            </Field>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={form.acme_staging === "1"} onChange={(e) => setForm({ ...form, acme_staging: e.target.checked ? "1" : "0" })} className="rounded border-slate-300" />
              Używaj środowiska testowego (staging) Let's Encrypt – certyfikaty nie będą zaufane
            </label>
          </div>
        </Card>
        <Button type="submit" loading={save.isPending}>Zapisz ustawienia</Button>
      </form>
    </>
  );
}
