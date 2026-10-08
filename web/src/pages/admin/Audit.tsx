import { useQuery } from "@tanstack/react-query";
import { get } from "../../lib/api";
import type { AuditEntry } from "../../lib/types";
import { Empty, PageHeader, Spinner, Table, Td, Th, formatDate } from "../../components/ui";

export default function AdminAudit() {
  const q = useQuery({ queryKey: ["admin", "audit"], queryFn: () => get<AuditEntry[]>("/admin/audit"), refetchInterval: 30_000 });
  return (
    <>
      <PageHeader title="Dziennik zdarzeń" subtitle="Ostatnie 200 operacji wykonanych w panelu" />
      {q.isLoading ? (
        <Spinner />
      ) : (
        <Table head={<><Th>Czas</Th><Th>Kto</Th><Th>Akcja</Th><Th>Obiekt</Th><Th>Szczegóły</Th></>} empty={q.data?.length === 0 && <Empty>Brak wpisów</Empty>}>
          {q.data?.map((e) => (
            <tr key={e.id}>
              <Td className="whitespace-nowrap text-slate-500">{formatDate(e.created_at)}</Td>
              <Td>{e.actor || "–"}{e.impersonator_id ? <span className="ml-1 text-xs text-amber-700">(admin)</span> : null}</Td>
              <Td className="font-mono text-xs">{e.action}</Td>
              <Td>{e.target}</Td>
              <Td className="text-slate-500">{e.detail}</Td>
            </tr>
          ))}
        </Table>
      )}
    </>
  );
}
