import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { del, get, post, put, errorMessage } from "../../lib/api";
import type { PHPVersion, Package } from "../../lib/types";
import { useToast } from "../../components/toast";
import { Button, Confirm, Empty, Field, Input, Modal, PageHeader, Spinner, Table, Td, Th, formatMB, phpLabel } from "../../components/ui";

type Form = Omit<Package, "id" | "user_count" | "created_at">;
const empty: Form = { name: "", disk_mb: 5120, max_domains: 3, max_subdomains: 10, max_databases: 3, max_ftp: 3, max_cron: 5, php_versions: [] };

export default function AdminPackages() {
  const qc = useQueryClient();
  const toast = useToast();
  const list = useQuery({ queryKey: ["admin", "packages"], queryFn: () => get<Package[]>("/admin/packages") });
  const php = useQuery({ queryKey: ["php"], queryFn: () => get<PHPVersion[]>("/php/versions") });
  const [modal, setModal] = useState<{ id?: number; form: Form } | null>(null);
  const [remove, setRemove] = useState<Package | null>(null);
  const invalidate = () => qc.invalidateQueries({ queryKey: ["admin", "packages"] });

  const save = useMutation({
    mutationFn: ({ id, form }: { id?: number; form: Form }) => (id ? put<Package>(`/admin/packages/${id}`, form) : post<Package>("/admin/packages", form)),
    onSuccess: () => {
      toast.success("Pakiet zapisany");
      setModal(null);
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });
  const removeM = useMutation({
    mutationFn: (id: number) => del(`/admin/packages/${id}`),
    onSuccess: () => {
      toast.success("Pakiet usunięty");
      setRemove(null);
      invalidate();
    },
    onError: (e) => toast.error(errorMessage(e)),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (modal) save.mutate(modal);
  };
  const f = modal?.form ?? empty;
  const setF = (patch: Partial<Form>) => modal && setModal({ ...modal, form: { ...modal.form, ...patch } });
  const togglePHP = (id: string) => setF({ php_versions: f.php_versions.includes(id) ? f.php_versions.filter((x) => x !== id) : [...f.php_versions, id] });
  const num = (key: keyof Form) => ({ type: "number", min: 0, value: f[key] as number, onChange: (e: React.ChangeEvent<HTMLInputElement>) => setF({ [key]: Number(e.target.value) } as Partial<Form>) });

  return (
    <>
      <PageHeader title="Pakiety" subtitle="Limity zasobów przypisywane użytkownikom" actions={<Button onClick={() => setModal({ form: { ...empty, php_versions: php.data?.map((p) => p.id) ?? [] } })}><Plus className="h-4 w-4" /> Nowy pakiet</Button>} />
      {list.isLoading ? (
        <Spinner />
      ) : (
        <Table head={<><Th>Nazwa</Th><Th>Dysk</Th><Th>Domeny</Th><Th>Subdomeny</Th><Th>Bazy</Th><Th>FTP</Th><Th>Cron</Th><Th>PHP</Th><Th>Użytk.</Th><Th className="text-right">Akcje</Th></>} empty={list.data?.length === 0 && <Empty>Brak pakietów</Empty>}>
          {list.data?.map((p) => (
            <tr key={p.id} className="hover:bg-slate-50">
              <Td className="font-medium text-slate-900">{p.name}</Td>
              <Td>{formatMB(p.disk_mb)}</Td>
              <Td>{p.max_domains}</Td>
              <Td>{p.max_subdomains}</Td>
              <Td>{p.max_databases}</Td>
              <Td>{p.max_ftp}</Td>
              <Td>{p.max_cron}</Td>
              <Td className="text-xs">{p.php_versions.map(phpLabel).join(", ")}</Td>
              <Td>{p.user_count}</Td>
              <Td>
                <div className="flex justify-end gap-1">
                  <Button size="sm" variant="ghost" onClick={() => setModal({ id: p.id, form: { name: p.name, disk_mb: p.disk_mb, max_domains: p.max_domains, max_subdomains: p.max_subdomains, max_databases: p.max_databases, max_ftp: p.max_ftp, max_cron: p.max_cron, php_versions: p.php_versions } })}><Pencil className="h-4 w-4" /></Button>
                  <Button size="sm" variant="ghost" onClick={() => setRemove(p)} disabled={p.user_count > 0} title={p.user_count > 0 ? "Pakiet jest używany" : "Usuń"}><Trash2 className="h-4 w-4 text-red-600" /></Button>
                </div>
              </Td>
            </tr>
          ))}
        </Table>
      )}

      <Modal open={!!modal} onClose={() => setModal(null)} title={modal?.id ? "Edycja pakietu" : "Nowy pakiet"} footer={<><Button variant="secondary" onClick={() => setModal(null)}>Anuluj</Button><Button type="submit" form="pkg-form" loading={save.isPending}>Zapisz</Button></>}>
        <form id="pkg-form" onSubmit={submit} className="space-y-4">
          <Field label="Nazwa"><Input value={f.name} onChange={(e) => setF({ name: e.target.value })} required maxLength={40} autoFocus /></Field>
          <div className="grid grid-cols-2 gap-4">
            <Field label="Dysk (MB)"><Input {...num("disk_mb")} min={1} /></Field>
            <Field label="Domeny"><Input {...num("max_domains")} /></Field>
            <Field label="Subdomeny"><Input {...num("max_subdomains")} /></Field>
            <Field label="Bazy danych"><Input {...num("max_databases")} /></Field>
            <Field label="Konta FTP"><Input {...num("max_ftp")} /></Field>
            <Field label="Zadania cron"><Input {...num("max_cron")} /></Field>
          </div>
          <div>
            <span className="mb-1 block text-sm font-medium text-slate-700">Dostępne wersje PHP</span>
            <div className="flex flex-wrap gap-3">
              {(php.data ?? []).map((v) => (
                <label key={v.id} className="flex items-center gap-1.5 text-sm">
                  <input type="checkbox" checked={f.php_versions.includes(v.id)} onChange={() => togglePHP(v.id)} className="rounded border-slate-300" /> {v.label}
                </label>
              ))}
              {php.data?.length === 0 && <span className="text-xs text-slate-500">Brak zainstalowanych wersji PHP</span>}
            </div>
          </div>
        </form>
      </Modal>
      <Confirm open={!!remove} onClose={() => setRemove(null)} onConfirm={() => remove && removeM.mutate(remove.id)} loading={removeM.isPending} title="Usuń pakiet" text={<>Czy na pewno usunąć pakiet <b>{remove?.name}</b>?</>} />
    </>
  );
}
