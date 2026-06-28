// ============================================================
// Device Schema
// ============================================================

// ── Enums ────────────────────────────────────────────────────

export type DeviceType = 'handheld' | 'desktop' | 'laptop' | 'other';

export type DeviceOS = 'steamos' | 'windows' | 'macos' | 'linux';

export type VolumeType = 'internal' | 'sd_card' | 'external_hdd' | 'external_ssd';

// ── Sub-types ────────────────────────────────────────────────

export interface StorageVolume {
  id: string;           // UUID — referenced by InstallSource.volume_id
  label: string;        // Human-readable name e.g. 'Internal SSD', 'Stora'
  type: VolumeType;
  mount_path: string;   // e.g. '/home/deck', '/run/media/deck/Stora', 'D:\'
  capacity_gb: number | null;
}

// ── Core Schema ──────────────────────────────────────────────

export interface Device {
  id: string;                   // UUID
  name: string;                 // e.g. 'Praxis', 'Ergaster'
  type: DeviceType;
  os: DeviceOS;
  storage_volumes: StorageVolume[];
}

// ── Known Devices ─────────────────────────────────────────────

/*
  Praxis  — Steam Deck (handheld, SteamOS)
            Volumes: Internal SSD (/home/deck)
                     Stora SD card (/run/media/deck/Stora)

  Ergaster — Acemagic M1 i9-13900hx MiniPC (desktop, Windows)
             Used for games requiring keyboard + mouse
*/
