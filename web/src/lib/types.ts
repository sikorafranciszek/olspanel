export type Role = "admin" | "user";

export interface User {
  id: number;
  username: string;
  role: Role;
  email: string;
  package_id: number;
  package_name: string;
  uid: number;
  gid: number;
  home: string;
  disk_used_mb: number;
  suspended: boolean;
  created_at: string;
  domain_count: number;
}

export interface Package {
  id: number;
  name: string;
  disk_mb: number;
  max_domains: number;
  max_subdomains: number;
  max_databases: number;
  max_ftp: number;
  max_cron: number;
  php_versions: string[];
  user_count: number;
  created_at: string;
}

export type DomainType = "domain" | "subdomain" | "alias";

export interface Domain {
  id: number;
  user_id: number;
  username: string;
  name: string;
  parent_id: number | null;
  type: DomainType;
  php_version: string;
  ssl_status: "none" | "active" | "error";
  ssl_expires_at: string | null;
  force_https: boolean;
  created_at: string;
}

export interface DBUser {
  id: number;
  user_id: number;
  database_id: number;
  username: string;
  created_at: string;
}

export interface Database {
  id: number;
  user_id: number;
  name: string;
  users: DBUser[];
  created_at: string;
}

export interface FTPAccount {
  id: number;
  user_id: number;
  login: string;
  home_subdir: string;
  created_at: string;
}

export interface CronJob {
  id: number;
  user_id: number;
  schedule: string;
  command: string;
  enabled: boolean;
  created_at: string;
}

export interface FileEntry {
  name: string;
  path: string;
  dir: boolean;
  size: number;
  mode: string;
  mtime: string;
  symlink: boolean;
}

export interface PHPVersion {
  id: string;
  label: string;
  bin: string;
}

export interface Usage {
  package: Package | null;
  disk_used_mb: number;
  domains: number;
  subdomains: number;
  databases: number;
  ftp: number;
  cron: number;
}

export interface Me {
  user: User | null;
  impersonating: boolean;
  csrf: string;
  version: string;
}

export interface ServiceStatus {
  unit: string;
  label: string;
  active: boolean;
}

export interface ServerInfo {
  hostname: string;
  version: string;
  go: string;
  os: string;
  uptime_s: number;
  users: number;
  domains: number;
  packages: number;
  services: ServiceStatus[];
  php: PHPVersion[];
  ols_ok: boolean;
  resources: {
    load_avg: number[];
    cpus: number;
    mem_total_mb: number;
    mem_used_mb: number;
    disk_total_gb: number;
    disk_used_gb: number;
  };
}

export interface AuditEntry {
  id: number;
  actor_id: number;
  actor: string;
  impersonator_id: number;
  action: string;
  target: string;
  detail: string;
  created_at: string;
}
