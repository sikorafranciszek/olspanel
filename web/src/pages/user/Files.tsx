import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import CodeMirror from "@uiw/react-codemirror";
import { php } from "@codemirror/lang-php";
import { html } from "@codemirror/lang-html";
import { css } from "@codemirror/lang-css";
import { javascript } from "@codemirror/lang-javascript";
import { Archive, ChevronRight, Copy, Download, File, FileArchive, FileText, Folder, FolderPlus, Home, Pencil, RefreshCw, Shield, Trash2, Upload } from "lucide-react";
import { downloadUrl, get, post, put, uploadFiles, errorMessage } from "../../lib/api";
import type { FileEntry } from "../../lib/types";
import { useToast } from "../../components/toast";
import { Button, Confirm, Empty, Field, Input, Modal, PageHeader, Spinner, Table, Td, Th, cx, formatBytes, formatDate } from "../../components/ui";

const textExt = new Set(["php", "html", "htm", "css", "js", "ts", "json", "txt", "md", "xml", "yml", "yaml", "ini", "conf", "htaccess", "env", "sh", "sql", "csv", "log", "svg", "tpl", "twig"]);
const ext = (n: string) => (n.startsWith(".") && !n.slice(1).includes(".") ? n.slice(1) : n.split(".").pop() ?? "").toLowerCase();
function langFor(name: string) {
  switch (ext(name)) {
    case "php": return [php()];
    case "html": case "htm": case "tpl": case "twig": return [html()];
    case "css": return [css()];
    case "js": case "ts": case "json": return [javascript({ typescript: ext(name) === "ts" })];
    default: return [];
  }
}

export default function UserFiles() {
  const qc = useQueryClient();
  const toast = useToast();
  const [dir, setDir] = useState("domains");
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [editor, setEditor] = useState<{ path: string; content: string; dirty: boolean } | null>(null);
  const [mkdir, setMkdir] = useState<string | null>(null);
  const [rename, setRename] = useState<{ from: string; to: string } | null>(null);
  const [copyTo, setCopyTo] = useState<{ from: string; to: string } | null>(null);
  const [chmod, setChmod] = useState<{ path: string; mode: string } | null>(null);
  const [zipName, setZipName] = useState<string | null>(null);
  const [removeConfirm, setRemoveConfirm] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);

  const list = useQuery({ queryKey: ["files", dir], queryFn: () => get<FileEntry[]>(`/files?path=${encodeURIComponent(dir)}`) });
  const invalidate = () => qc.invalidateQueries({ queryKey: ["files", dir] });
  const onErr = (e: unknown) => toast.error(errorMessage(e));
  useEffect(() => setSelected(new Set()), [dir]);

  const crumbs = useMemo(() => dir.split("/").filter(Boolean), [dir]);
  const join = (a: string, b: string) => (a ? `${a}/${b}` : b);

  const mkdirM = useMutation({ mutationFn: (name: string) => post("/files/mkdir", { path: join(dir, name) }), onSuccess: () => { setMkdir(null); invalidate(); }, onError: onErr });
  const renameM = useMutation({ mutationFn: (r: { from: string; to: string }) => post("/files/rename", { from: r.from, to: join(dir, r.to) }), onSuccess: () => { setRename(null); invalidate(); }, onError: onErr });
  const copyM = useMutation({ mutationFn: (r: { from: string; to: string }) => post("/files/copy", { from: r.from, to: join(dir, r.to) }), onSuccess: () => { setCopyTo(null); invalidate(); }, onError: onErr });
  const chmodM = useMutation({ mutationFn: (r: { path: string; mode: string }) => post("/files/chmod", r), onSuccess: () => { setChmod(null); invalidate(); }, onError: onErr });
  const deleteM = useMutation({ mutationFn: (paths: string[]) => post("/files/delete", { paths }), onSuccess: () => { setRemoveConfirm(false); toast.success("Usunięto"); invalidate(); }, onError: onErr });
  const zipM = useMutation({ mutationFn: (name: string) => post("/files/compress", { paths: Array.from(selected), dest: join(dir, name) }), onSuccess: () => { setZipName(null); toast.success("Archiwum utworzone"); invalidate(); }, onError: onErr });
  const unzipM = useMutation({ mutationFn: (p: string) => post("/files/extract", { path: p, dest: dir }), onSuccess: () => { toast.success("Rozpakowano"); invalidate(); }, onError: onErr });
  const saveM = useMutation({ mutationFn: (e: { path: string; content: string }) => put("/files/content", e), onSuccess: () => { toast.success("Zapisano"); setEditor((s) => s && { ...s, dirty: false }); invalidate(); }, onError: onErr });
  const uploadM = useMutation({ mutationFn: (files: FileList | File[]) => uploadFiles(dir, files), onSuccess: (r) => { toast.success(`Wgrano plików: ${r.uploaded}`); invalidate(); }, onError: onErr });

  const openEntry = async (e: FileEntry) => {
    if (e.dir) { setDir(e.path); return; }
    if (!textExt.has(ext(e.name)) && e.size > 0 && e.size > 2 * 1024 * 1024) { window.open(downloadUrl(e.path)); return; }
    try {
      const r = await get<{ content: string }>(`/files/content?path=${encodeURIComponent(e.path)}`);
      setEditor({ path: e.path, content: r.content, dirty: false });
    } catch (err) { toast.error(errorMessage(err)); }
  };
  const toggle = (p: string) => setSelected((s) => { const n = new Set(s); n.has(p) ? n.delete(p) : n.add(p); return n; });
  const onDrop = useCallback((ev: React.DragEvent) => { ev.preventDefault(); setDragOver(false); if (ev.dataTransfer.files.length) uploadM.mutate(ev.dataTransfer.files); }, [uploadM]);
  const one = selected.size === 1 ? Array.from(selected)[0] : null;
  const oneEntry = one ? list.data?.find((e) => e.path === one) : undefined;

  return (
    <>
      <PageHeader title="Menedżer plików" subtitle="Pliki Twojego konta (katalog domowy)" actions={<><input ref={fileInput} type="file" multiple className="hidden" onChange={(e) => e.target.files && uploadM.mutate(e.target.files)} /><Button variant="secondary" onClick={() => setMkdir("")}><FolderPlus className="h-4 w-4" /> Nowy katalog</Button><Button onClick={() => fileInput.current?.click()} loading={uploadM.isPending}><Upload className="h-4 w-4" /> Wgraj pliki</Button></>} />
      <div className="mb-3 flex flex-wrap items-center gap-1 text-sm">
        <button onClick={() => setDir("")} className="flex items-center gap-1 rounded px-1.5 py-0.5 text-slate-600 hover:bg-slate-100"><Home className="h-4 w-4" /> /home</button>
        {crumbs.map((c, i) => (
          <span key={i} className="flex items-center gap-1">
            <ChevronRight className="h-3.5 w-3.5 text-slate-400" />
            <button onClick={() => setDir(crumbs.slice(0, i + 1).join("/"))} className={cx("rounded px-1.5 py-0.5 hover:bg-slate-100", i === crumbs.length - 1 ? "font-medium text-slate-900" : "text-slate-600")}>{c}</button>
          </span>
        ))}
        <Button size="sm" variant="ghost" className="ml-auto" onClick={() => invalidate()}><RefreshCw className="h-3.5 w-3.5" /></Button>
      </div>
      {selected.size > 0 && (
        <div className="mb-3 flex flex-wrap items-center gap-2 rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-sm">
          <span className="font-medium text-blue-900">Zaznaczono: {selected.size}</span>
          {oneEntry && !oneEntry.dir && <a href={downloadUrl(oneEntry.path)}><Button size="sm" variant="secondary"><Download className="h-3.5 w-3.5" /> Pobierz</Button></a>}
          {oneEntry && <Button size="sm" variant="secondary" onClick={() => setRename({ from: oneEntry.path, to: oneEntry.name })}><Pencil className="h-3.5 w-3.5" /> Zmień nazwę</Button>}
          {oneEntry && <Button size="sm" variant="secondary" onClick={() => setCopyTo({ from: oneEntry.path, to: `${oneEntry.name}-kopia` })}><Copy className="h-3.5 w-3.5" /> Kopiuj</Button>}
          {oneEntry && <Button size="sm" variant="secondary" onClick={() => setChmod({ path: oneEntry.path, mode: oneEntry.mode })}><Shield className="h-3.5 w-3.5" /> Uprawnienia</Button>}
          {oneEntry && ext(oneEntry.name) === "zip" && <Button size="sm" variant="secondary" onClick={() => unzipM.mutate(oneEntry.path)} loading={unzipM.isPending}><FileArchive className="h-3.5 w-3.5" /> Rozpakuj tutaj</Button>}
          <Button size="sm" variant="secondary" onClick={() => setZipName("archiwum.zip")}><Archive className="h-3.5 w-3.5" /> Spakuj do zip</Button>
          <Button size="sm" variant="danger" onClick={() => setRemoveConfirm(true)}><Trash2 className="h-3.5 w-3.5" /> Usuń</Button>
        </div>
      )}
      <div onDragOver={(e) => { e.preventDefault(); setDragOver(true); }} onDragLeave={() => setDragOver(false)} onDrop={onDrop} className={cx("rounded-lg transition", dragOver && "ring-2 ring-blue-500 ring-offset-2")}>
        {list.isLoading ? <Spinner /> : (
          <Table head={<><Th className="w-8"><input type="checkbox" checked={!!list.data?.length && selected.size === list.data.length} onChange={(e) => setSelected(e.target.checked ? new Set(list.data?.map((x) => x.path)) : new Set())} className="rounded border-slate-300" /></Th><Th>Nazwa</Th><Th>Rozmiar</Th><Th>Uprawnienia</Th><Th>Zmodyfikowano</Th></>} empty={list.data?.length === 0 && <Empty>Pusty katalog. Przeciągnij tu pliki, aby je wgrać.</Empty>}>
            {dir && (
              <tr className="hover:bg-slate-50"><Td /><Td colSpan={4}><button onClick={() => setDir(crumbs.slice(0, -1).join("/"))} className="flex items-center gap-2 text-slate-600 hover:text-blue-700"><Folder className="h-4 w-4 text-slate-400" /> ..</button></Td></tr>
            )}
            {list.data?.map((e) => (
              <tr key={e.path} className={cx("hover:bg-slate-50", selected.has(e.path) && "bg-blue-50/50")}>
                <Td><input type="checkbox" checked={selected.has(e.path)} onChange={() => toggle(e.path)} className="rounded border-slate-300" /></Td>
                <Td>
                  <button onClick={() => openEntry(e)} className="flex items-center gap-2 text-left font-medium text-slate-900 hover:text-blue-700">
                    {e.dir ? <Folder className="h-4 w-4 text-amber-500" /> : textExt.has(ext(e.name)) ? <FileText className="h-4 w-4 text-slate-400" /> : <File className="h-4 w-4 text-slate-400" />}
                    {e.name}{e.symlink && <span className="text-xs text-slate-400">→</span>}
                  </button>
                </Td>
                <Td className="text-slate-500">{e.dir ? "–" : formatBytes(e.size)}</Td>
                <Td className="font-mono text-xs text-slate-500">{e.mode}</Td>
                <Td className="text-slate-500">{formatDate(e.mtime)}</Td>
              </tr>
            ))}
          </Table>
        )}
      </div>

      <Modal open={!!editor} onClose={() => (!editor?.dirty || confirm("Odrzucić niezapisane zmiany?")) && setEditor(null)} title={editor?.path ?? ""} wide footer={<><Button variant="secondary" onClick={() => setEditor(null)}>Zamknij</Button><Button onClick={() => editor && saveM.mutate({ path: editor.path, content: editor.content })} loading={saveM.isPending} disabled={!editor?.dirty}>Zapisz</Button></>}>
        {editor && <CodeMirror value={editor.content} height="60vh" extensions={langFor(editor.path)} onChange={(v) => setEditor((s) => s && { ...s, content: v, dirty: true })} basicSetup={{ lineNumbers: true }} />}
      </Modal>
      <Modal open={mkdir !== null} onClose={() => setMkdir(null)} title="Nowy katalog" footer={<><Button variant="secondary" onClick={() => setMkdir(null)}>Anuluj</Button><Button onClick={() => mkdir && mkdirM.mutate(mkdir)} loading={mkdirM.isPending} disabled={!mkdir}>Utwórz</Button></>}>
        <Field label="Nazwa"><Input value={mkdir ?? ""} onChange={(e) => setMkdir(e.target.value)} autoFocus onKeyDown={(e) => e.key === "Enter" && mkdir && mkdirM.mutate(mkdir)} /></Field>
      </Modal>
      <Modal open={!!rename} onClose={() => setRename(null)} title="Zmień nazwę" footer={<><Button variant="secondary" onClick={() => setRename(null)}>Anuluj</Button><Button onClick={() => rename && renameM.mutate(rename)} loading={renameM.isPending}>Zapisz</Button></>}>
        <Field label="Nowa nazwa"><Input value={rename?.to ?? ""} onChange={(e) => rename && setRename({ ...rename, to: e.target.value })} autoFocus /></Field>
      </Modal>
      <Modal open={!!copyTo} onClose={() => setCopyTo(null)} title="Kopiuj" footer={<><Button variant="secondary" onClick={() => setCopyTo(null)}>Anuluj</Button><Button onClick={() => copyTo && copyM.mutate(copyTo)} loading={copyM.isPending}>Kopiuj</Button></>}>
        <Field label="Nazwa kopii (w bieżącym katalogu)"><Input value={copyTo?.to ?? ""} onChange={(e) => copyTo && setCopyTo({ ...copyTo, to: e.target.value })} autoFocus /></Field>
      </Modal>
      <Modal open={!!chmod} onClose={() => setChmod(null)} title="Uprawnienia" footer={<><Button variant="secondary" onClick={() => setChmod(null)}>Anuluj</Button><Button onClick={() => chmod && chmodM.mutate(chmod)} loading={chmodM.isPending}>Zapisz</Button></>}>
        <Field label="Tryb (ósemkowo)" hint="np. 0644 dla plików, 0755 dla katalogów"><Input value={chmod?.mode ?? ""} onChange={(e) => chmod && setChmod({ ...chmod, mode: e.target.value })} pattern="0?[0-7]{3}" autoFocus /></Field>
      </Modal>
      <Modal open={zipName !== null} onClose={() => setZipName(null)} title="Spakuj do zip" footer={<><Button variant="secondary" onClick={() => setZipName(null)}>Anuluj</Button><Button onClick={() => zipName && zipM.mutate(zipName)} loading={zipM.isPending}>Spakuj</Button></>}>
        <Field label="Nazwa archiwum"><Input value={zipName ?? ""} onChange={(e) => setZipName(e.target.value)} autoFocus /></Field>
      </Modal>
      <Confirm open={removeConfirm} onClose={() => setRemoveConfirm(false)} onConfirm={() => deleteM.mutate(Array.from(selected))} loading={deleteM.isPending} title="Usuń" text={<>Usunąć zaznaczone elementy ({selected.size})? Katalogi zostaną usunięte razem z zawartością.</>} />
    </>
  );
}
