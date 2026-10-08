import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { Activity, Boxes, Clock, Database, FolderOpen, Globe, KeyRound, LayoutDashboard, LogOut, Menu, ScrollText, Server, Settings, Upload, UserCircle, Users, X } from "lucide-react";
import { useState } from "react";
import { useAuth } from "../lib/auth";
import { post } from "../lib/api";
import type { Me } from "../lib/types";
import { cx } from "./ui";

const adminNav = [
  { to: "/admin", label: "Pulpit", icon: LayoutDashboard, end: true },
  { to: "/admin/users", label: "Użytkownicy", icon: Users },
  { to: "/admin/packages", label: "Pakiety", icon: Boxes },
  { to: "/admin/domains", label: "Domeny", icon: Globe },
  { to: "/admin/settings", label: "Ustawienia", icon: Settings },
  { to: "/admin/audit", label: "Dziennik", icon: ScrollText },
];

const userNav = [
  { to: "/", label: "Pulpit", icon: LayoutDashboard, end: true },
  { to: "/domains", label: "Domeny", icon: Globe },
  { to: "/databases", label: "Bazy danych", icon: Database },
  { to: "/files", label: "Pliki", icon: FolderOpen },
  { to: "/ftp", label: "Konta FTP", icon: Upload },
  { to: "/cron", label: "Cron", icon: Clock },
  { to: "/account", label: "Moje konto", icon: KeyRound },
];

export default function Layout({ admin }: { admin: boolean }) {
  const { user, impersonating, version, logout, applyMe } = useAuth();
  const nav = admin ? adminNav : userNav;
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);

  const stopImpersonation = async () => {
    const me = await post<Me>("/auth/stop-impersonation");
    applyMe(me);
    navigate("/admin/users");
  };

  return (
    <div className="flex min-h-full">
      <aside className={cx("fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-slate-200 bg-white transition-transform lg:static lg:translate-x-0", open ? "translate-x-0" : "-translate-x-full")}>
        <div className="flex h-14 items-center justify-between border-b border-slate-100 px-4">
          <div className="flex items-center gap-2 font-semibold text-slate-900">
            <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-blue-600 text-sm font-bold text-white">O</span>
            olspanel
          </div>
          <button className="rounded p-1 text-slate-500 lg:hidden" onClick={() => setOpen(false)} aria-label="Zamknij menu">
            <X className="h-5 w-5" />
          </button>
        </div>
        <nav className="flex-1 space-y-0.5 overflow-y-auto p-3">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.end}
              onClick={() => setOpen(false)}
              className={({ isActive }) => cx("flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium", isActive ? "bg-blue-50 text-blue-700" : "text-slate-700 hover:bg-slate-100")}
            >
              <n.icon className="h-4 w-4" />
              {n.label}
            </NavLink>
          ))}
          {admin && (
            <a href="/phpmyadmin/" target="_blank" rel="noreferrer" className="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm font-medium text-slate-700 hover:bg-slate-100">
              <Database className="h-4 w-4" /> phpMyAdmin
            </a>
          )}
        </nav>
        <div className="border-t border-slate-100 p-3 text-xs text-slate-500">
          <div className="flex items-center gap-2 px-2 py-1">
            <UserCircle className="h-4 w-4" />
            <span className="truncate font-medium text-slate-700">{user?.username}</span>
            <span className="ml-auto rounded bg-slate-100 px-1.5 py-0.5">{admin ? "admin" : user?.package_name || "użytkownik"}</span>
          </div>
          <div className="px-2 pt-1 text-[11px] text-slate-400">olspanel {version}</div>
        </div>
      </aside>
      {open && <div className="fixed inset-0 z-30 bg-slate-900/30 lg:hidden" onClick={() => setOpen(false)} />}

      <div className="flex min-w-0 flex-1 flex-col">
        {impersonating && (
          <div className="flex items-center justify-between gap-3 bg-amber-100 px-4 py-2 text-sm text-amber-900">
            <span className="flex items-center gap-2">
              <Activity className="h-4 w-4" /> Przeglądasz konto <b>{user?.username}</b> jako administrator.
            </span>
            <button onClick={stopImpersonation} className="rounded-md bg-amber-600 px-3 py-1 text-xs font-medium text-white hover:bg-amber-700">
              Wróć do panelu admina
            </button>
          </div>
        )}
        <header className="flex h-14 items-center justify-between border-b border-slate-200 bg-white px-4 lg:px-6">
          <button className="rounded p-1 text-slate-600 lg:hidden" onClick={() => setOpen(true)} aria-label="Otwórz menu">
            <Menu className="h-5 w-5" />
          </button>
          <div className="hidden items-center gap-2 text-sm text-slate-500 lg:flex">
            <Server className="h-4 w-4" /> {admin ? "Panel administratora" : "Panel użytkownika"}
          </div>
          <button onClick={() => logout()} className="flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-sm text-slate-700 hover:bg-slate-100">
            <LogOut className="h-4 w-4" /> Wyloguj
          </button>
        </header>
        <main className="flex-1 p-4 lg:p-8">
          <div className="mx-auto max-w-6xl">
            <Outlet />
          </div>
        </main>
      </div>
    </div>
  );
}
