import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { del, get, post, put, errorMessage } from "../../lib/api";
import type { CronJob } from "../../lib/types";
import { useToast } from "../../components/toast";
import { Badge, Button, Card, Confirm, Empty, Field, Input, Modal, PageHeader, Select, Spinner, Table, Td, Th } from "../../components/ui";

const presets = [
  { label: "Co 5 minut", value: "*/5 * * * *" },
  { label: "Co godzinę", value: "0 * * * *" },
  { label: "Codziennie o 3:00", value: "0 3 * * *" },
  { label: "Co tydzień (niedziela 4:00)", value: "0 4 * * 0" },
  { label: "Co miesiąc (1. dnia 5:00)", value: "0 5 1 * *" },
];

type Form = { schedule: string; command: string; enabled: boolean };
const empty: Form = { schedule: "0 3 * * *", command: "", enabled: true };

export default function UserCron() {
  const qc = useQueryClient();
  const toast = useToast();
  const list = useQuery({ queryKey: ["cron"], queryFn: () => get<CronJob[]>("/cron") });
  const [modal, setModal] = useState<{ id?: number; form: Form } | null>(null);
  const [remove, setRemove] = useState<CronJob | null>(null);
  const invalidate = () => { qc.invalidateQueries({ queryKey: ["cron"] }); qc.invalidateQueries({ queryKey: ["usage"] }); };
  const onErr = (e: unknown) => toast.error(errorMessage(e));
  const save = useMutation({ mutationFn: ({ id, form }: { id?: number; form: Form }) => (id ? put<CronJob>(`/cron/${id}`, form) : post<CronJob>("/cron", form)), onSuccess: () => { toast.success("Zadanie zapisane"); setModal(null); invalidate(); }, onError: onErr });
  const toggle = useMutation({ mutationFn: (j: CronJob) => put<CronJob>(`/cron/${j.id}`, { schedule: j.schedule, command: j.command, enabled: !j.enabled }), onSuccess: invalidate, onError: onErr });
  const removeM = useMutation({ mutationFn: (id: number) => del(`/cron/${id}`), onSuccess: () => { toast.success("Zadanie usunięte"); setRemove(null); invalidate(); }, onError: onErr });
  const f = modal?.form ?? empty;
  const setF = (p: Partial<Form>) => modal && setModal({ ...modal, form: { ...modal.form, ...p } });
  const submit = (e: FormEvent) => { e.preventDefault(); if (modal) save.mutate(modal); };

  return (
    <>
      <PageHeader title="Zadania cron" subtitle="Polecenia uruchamiane cyklicznie na Twoim koncie" actions={<Button onClick={() => setModal({ form: empty })}><Plus className="h-4 w-4" /> Nowe zadanie</Button>} />
      <Card className="mb-4"><p className="text-sm text-slate-600">Polecenia działają jako Twój użytkownik systemowy. Do skryptów PHP użyj pełnej ścieżki, np. <code className="rounded bg-slate-100 px-1">/usr/local/lsws/lsphp83/bin/php /home/{"<użytkownik>"}/domains/example.com/public_html/cron.php</code>. Wyjście poleceń nie jest wysyłane e-mailem – przekieruj je do pliku, jeśli chcesz je zachować.</p></Card>
      {list.isLoading ? <Spinner /> : (
        <Table head={<><Th>Harmonogram</Th><Th>Polecenie</Th><Th>Status</Th><Th className="text-right">Akcje</Th></>} empty={list.data?.length === 0 && <Empty>Brak zadań cron</Empty>}>
          {list.data?.map((j) => (
            <tr key={j.id} className="hover:bg-slate-50">
              <Td className="font-mono text-xs">{j.schedule}</Td>
              <Td className="max-w-md truncate font-mono text-xs" title={j.command}>{j.command}</Td>
              <Td><button onClick={() => toggle.mutate(j)} title="Przełącz">{j.enabled ? <Badge tone="green">włączone</Badge> : <Badge>wyłączone</Badge>}</button></Td>
              <Td><div className="flex justify-end gap-1"><Button size="sm" variant="ghost" onClick={() => setModal({ id: j.id, form: { schedule: j.schedule, command: j.command, enabled: j.enabled } })}><Pencil className="h-4 w-4" /></Button><Button size="sm" variant="ghost" onClick={() => setRemove(j)}><Trash2 className="h-4 w-4 text-red-600" /></Button></div></Td>
            </tr>
          ))}
        </Table>
      )}
      <Modal open={!!modal} onClose={() => setModal(null)} title={modal?.id ? "Edycja zadania" : "Nowe zadanie cron"} footer={<><Button variant="secondary" onClick={() => setModal(null)}>Anuluj</Button><Button type="submit" form="cron-form" loading={save.isPending}>Zapisz</Button></>}>
        <form id="cron-form" onSubmit={submit} className="space-y-4">
          <Field label="Szablon harmonogramu">
            <Select value={presets.some((p) => p.value === f.schedule) ? f.schedule : ""} onChange={(e) => e.target.value && setF({ schedule: e.target.value })}>
              <option value="">– własny –</option>
              {presets.map((p) => <option key={p.value} value={p.value}>{p.label}</option>)}
            </Select>
          </Field>
          <Field label="Harmonogram (minuta godzina dzień miesiąc dzień_tygodnia)"><Input value={f.schedule} onChange={(e) => setF({ schedule: e.target.value })} className="font-mono" required /></Field>
          <Field label="Polecenie"><Input value={f.command} onChange={(e) => setF({ command: e.target.value })} className="font-mono" required placeholder="/usr/local/lsws/lsphp83/bin/php /home/.../cron.php" /></Field>
          <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={f.enabled} onChange={(e) => setF({ enabled: e.target.checked })} className="rounded border-slate-300" /> Włączone</label>
        </form>
      </Modal>
      <Confirm open={!!remove} onClose={() => setRemove(null)} onConfirm={() => remove && removeM.mutate(remove.id)} loading={removeM.isPending} title="Usuń zadanie" text="Zadanie cron zostanie usunięte." />
    </>
  );
}
